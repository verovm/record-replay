package replay

import (
	"bytes"
	"errors"
	"fmt"
	"math/big"
	"sort"
	"sync"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/params"
	"github.com/ethereum/go-ethereum/research"
	"github.com/ethereum/go-ethereum/tests"
	cli "github.com/urfave/cli/v2"
	"google.golang.org/protobuf/proto"
)

// record-replay: replay-fork command
var ReplayForkCommand = &cli.Command{
	Action: replayForkAction,
	Name:   "replay-fork",
	Usage:  "replay transactions with the given hard fork and compare results",
	Flags: []cli.Flag{
		research.WorkersFlag,
		research.SkipTransferTxsFlag,
		research.SkipCallTxsFlag,
		research.SkipCreateTxsFlag,
		HardForkFlag,
		research.SubstateDirFlag,
		research.BlockSegmentFlag,
	},
	Description: `
substate-cli replay executes transactions in the given block segment
with the given hard fork config and report output comparison results.`,
	Category: "replay",
}

var HardForkName = map[int64]string{
	1:          "Frontier",
	1_150_000:  "Homestead",
	2_463_000:  "Tangerine Whistle",
	2_675_000:  "Spurious Dragon",
	4_370_000:  "Byzantium",
	7_280_000:  "Constantinople + Petersburg",
	9_069_000:  "Istanbul",
	12_244_000: "Berlin",
	12_965_000: "London",
}

func hardForkFlagDefault() int64 {
	var v int64 = 0
	for num64 := range HardForkName {
		if num64 > v {
			v = num64
		}
	}
	if v <= 0 {
		panic(fmt.Errorf("substate-cli replay-fork: corrupted --hard-fork default value: %v", v))
	}
	return v
}

var HardForkFlag = &cli.Int64Flag{
	Name: "hard-fork",
	Usage: func() string {
		s := ""
		s += "Hard-fork block number, won't change block number in Env for NUMBER instruction"

		hardForkNums := make([]int64, 0, len(HardForkName))
		for num64 := range HardForkName {
			hardForkNums = append(hardForkNums, num64)
		}
		sort.Slice(hardForkNums, func(i, j int) bool { return hardForkNums[i] < hardForkNums[j] })
		for _, num64 := range hardForkNums {
			s += fmt.Sprintf("\n\t  %v: %s", num64, HardForkName[num64])
		}
		return s
	}(),
	Value: hardForkFlagDefault(),
}

var ReplayForkChainConfig *params.ChainConfig = &params.ChainConfig{}

type ReplayForkStat struct {
	Count  int64
	ErrStr string
}

var ReplayForkStatChan chan *ReplayForkStat = make(chan *ReplayForkStat, 1_000_000)
var ReplayForkStatMap map[string]*ReplayForkStat = make(map[string]*ReplayForkStat)

var (
	ErrReplayForkOutOfGas     = errors.New("out of gas in replay-fork")
	ErrReplayForkInvalidAlloc = errors.New("invalid alloc in replay-fork")
	ErrReplayForkMoreGas      = errors.New("more gas in replay-fork")
	ErrReplayForkLessGas      = errors.New("less gas in replay-fork")
	ErrReplayForkMisc         = errors.New("misc in replay-fork")
)

func replayForkTask(block uint64, tx int, substate *research.Substate, taskPool *research.SubstateTaskPool) error {
	var stat *ReplayForkStat
	defer func() {
		if stat != nil {
			ReplayForkStatChan <- stat
		}
	}()
	var (
		vmConfig    vm.Config
		chainConfig *params.ChainConfig
		getTracerFn func(txIndex int, txHash common.Hash) (tracer vm.EVMLogger, err error)
	)

	vmConfig = vm.Config{}

	chainConfig = &params.ChainConfig{}
	*chainConfig = *params.MainnetChainConfig
	// disable DAOForkSupport, otherwise account states will be overwritten
	chainConfig.DAOForkSupport = false

	getTracerFn = func(txIndex int, txHash common.Hash) (tracer vm.EVMLogger, err error) {
		return nil, nil
	}

	// Apply Message
	var (
		statedb   = MakeOffTheChainStateDB(substate)
		gaspool   = new(core.GasPool)
		blockHash = common.Hash{0x01}
		txHash    = common.Hash{0x02}
		txIndex   = tx
	)

	blockCtx := vm.BlockContext{
		CanTransfer: core.CanTransfer,
		Transfer:    core.Transfer,
	}
	blockCtx.LoadSubstate(substate)
	gaspool.AddGas(blockCtx.GasLimit)

	msg := &core.Message{}
	msg.LoadSubstate(substate)

	tracer, err := getTracerFn(txIndex, txHash)
	if err != nil {
		return err
	}
	vmConfig.Tracer = tracer

	txCtx := vm.TxContext{
		GasPrice: msg.GasPrice,
		Origin:   msg.From,
	}

	// replay-fork hard fork configuration
	chainConfig = ReplayForkChainConfig
	if chainConfig.IsLondon(blockCtx.BlockNumber) && blockCtx.BaseFee == nil {
		// If blockCtx.BaseFee is nil, assume blockCtx.BaseFee is zero
		blockCtx.BaseFee = new(big.Int)
	}

	statedb.SetTxContext(txHash, tx)

	evm := vm.NewEVM(blockCtx, txCtx, statedb, chainConfig, vmConfig)
	result, msgErr := core.ApplyMessage(evm, msg, gaspool)

	if chainConfig.IsByzantium(blockCtx.BlockNumber) {
		statedb.Finalise(true)
	} else {
		statedb.IntermediateRoot(chainConfig.IsEIP158(blockCtx.BlockNumber))
	}

	rr := &research.ResearchReceipt{}
	if result.Failed() {
		rr.Status = types.ReceiptStatusFailed
	} else {
		rr.Status = types.ReceiptStatusSuccessful
	}
	rr.GasUsed = result.UsedGas

	if msg.To == nil {
		contractAddr := crypto.CreateAddress(evm.TxContext.Origin, msg.Nonce)
		rr.ContractAddress = &contractAddr
	}
	rr.Logs = statedb.GetLogs(txHash, blockCtx.BlockNumber.Uint64(), blockHash)
	rr.Bloom = types.CreateBloom(types.Receipts{&types.Receipt{Logs: rr.Logs}})

	replaySubstate := &research.Substate{}
	statedb.SaveSubstate(replaySubstate)
	blockCtx.SaveSubstate(replaySubstate)
	msg.SaveSubstate(replaySubstate)
	rr.SaveSubstate(replaySubstate)

	// TODO: compare substate and
	eqAlloc := proto.Equal(substate.OutputAlloc, replaySubstate.OutputAlloc)
	eqResult := proto.Equal(substate.Result, replaySubstate.Result)

	if eqAlloc && eqResult {
		stat = &ReplayForkStat{
			Count:  1,
			ErrStr: fmt.Sprintf("%v", ErrReplayForkMisc),
		}
		return nil
	}

	outputAlloc := make(map[common.Address]*research.Substate_Account)
	for _, entry := range substate.OutputAlloc.Alloc {
		outputAlloc[*research.BytesToAddress(entry.Address)] = entry.Account
	}
	evmAlloc := make(map[common.Address]*research.Substate_Account)
	for _, entry := range replaySubstate.OutputAlloc.Alloc {
		evmAlloc[*research.BytesToAddress(entry.Address)] = entry.Account
	}

	outputResult := substate.Result
	evmResult := replaySubstate.Result

	if *outputResult.Status == types.ReceiptStatusSuccessful &&
		*evmResult.Status == types.ReceiptStatusFailed {
		// if output was successful but evm failed, return runtime error
		stat = &ReplayForkStat{
			Count:  1,
			ErrStr: fmt.Sprintf("%v", msgErr),
		}
		return nil
	}

	if *outputResult.Status == types.ReceiptStatusSuccessful &&
		*evmResult.Status == types.ReceiptStatusSuccessful {
		// when both output and evm were successful, check alloc and gas usage

		// check account states
		if len(outputAlloc) != len(evmAlloc) {
			stat = &ReplayForkStat{
				Count:  1,
				ErrStr: fmt.Sprintf("%v", ErrReplayForkInvalidAlloc),
			}
			return nil
		}
		for addr := range outputAlloc {
			account1 := outputAlloc[addr]
			account2 := evmAlloc[addr]
			if account2 == nil {
				stat = &ReplayForkStat{
					Count:  1,
					ErrStr: fmt.Sprintf("%v", ErrReplayForkInvalidAlloc),
				}
				return nil
			}

			// check nonce
			if account1.Nonce != account2.Nonce {
				stat = &ReplayForkStat{
					Count:  1,
					ErrStr: fmt.Sprintf("%v", ErrReplayForkInvalidAlloc),
				}
				return nil
			}

			// check code
			if !bytes.Equal(account1.GetCode(), account2.GetCode()) {
				stat = &ReplayForkStat{
					Count:  1,
					ErrStr: fmt.Sprintf("%v", ErrReplayForkInvalidAlloc),
				}
				return nil
			}

			// check storage
			storage1 := make(map[common.Hash]common.Hash)
			for _, entry := range account1.Storage {
				storage1[*research.BytesToHash(entry.Key)] = *research.BytesToHash(entry.Value)
			}
			storage2 := make(map[common.Hash]common.Hash)
			for _, entry := range account2.Storage {
				storage2[*research.BytesToHash(entry.Key)] = *research.BytesToHash(entry.Value)
			}
			if len(storage1) != len(storage2) {
				stat = &ReplayForkStat{
					Count:  1,
					ErrStr: fmt.Sprintf("%v", ErrReplayForkInvalidAlloc),
				}
				return nil
			}
			for k, v1 := range storage1 {
				if v2, exist := storage2[k]; !exist || v1 != v2 {
					stat = &ReplayForkStat{
						Count:  1,
						ErrStr: fmt.Sprintf("%v", ErrReplayForkInvalidAlloc),
					}
					return nil
				}
			}
		}

		// more gas
		if *evmResult.GasUsed > *outputResult.GasUsed {
			stat = &ReplayForkStat{
				Count:  1,
				ErrStr: fmt.Sprintf("%v", ErrReplayForkMoreGas),
			}
			return nil
		}

		// less gas
		if *evmResult.GasUsed < *outputResult.GasUsed {
			stat = &ReplayForkStat{
				Count:  1,
				ErrStr: fmt.Sprintf("%v", ErrReplayForkLessGas),
			}
			return nil
		}

		// misc: logs, ...
		stat = &ReplayForkStat{
			Count:  1,
			ErrStr: fmt.Sprintf("%v", ErrReplayForkMisc),
		}
		return nil

	}

	// misc (logs, ...)
	stat = &ReplayForkStat{
		Count:  1,
		ErrStr: fmt.Sprintf("%v", ErrReplayForkMisc),
	}
	return nil
}

// record-replay: func replayForkAction for replay-fork command
func replayForkAction(ctx *cli.Context) error {
	var err error

	hardFork := ctx.Int64(HardForkFlag.Name)
	if hardForkName, exist := HardForkName[hardFork]; !exist {
		return fmt.Errorf("substate-cli replay-fork: invalid hard-fork block number %v", hardFork)
	} else {
		fmt.Printf("substate-cli replay-fork: hard-fork: block %v (%s)\n", hardFork, hardForkName)
	}
	switch hardFork {
	case 1:
		*ReplayForkChainConfig = *tests.Forks["Frontier"]
	case 1_150_000:
		*ReplayForkChainConfig = *tests.Forks["Homestead"]
	case 2_463_000:
		*ReplayForkChainConfig = *tests.Forks["EIP150"] // Tangerine Whistle
	case 2_675_000:
		*ReplayForkChainConfig = *tests.Forks["EIP158"] // Spurious Dragon
	case 4_370_000:
		*ReplayForkChainConfig = *tests.Forks["Byzantium"]
	case 7_280_000:
		*ReplayForkChainConfig = *tests.Forks["ConstantinopleFix"]
	case 9_069_000:
		*ReplayForkChainConfig = *tests.Forks["Istanbul"]
	case 12_244_000:
		*ReplayForkChainConfig = *tests.Forks["Berlin"]
	case 12_965_000:
		*ReplayForkChainConfig = *tests.Forks["London"]
	}

	research.SetSubstateFlags(ctx)
	research.OpenSubstateDBReadOnly()
	defer research.CloseSubstateDB()

	statWg := &sync.WaitGroup{}
	statWg.Add(1)
	go func() {
		for stat := range ReplayForkStatChan {
			count := stat.Count
			errstr := stat.ErrStr

			if ReplayForkStatMap[errstr] == nil {
				ReplayForkStatMap[errstr] = &ReplayForkStat{
					Count:  0,
					ErrStr: errstr,
				}
			}

			ReplayForkStatMap[errstr].Count += count
		}
		statWg.Done()
	}()

	taskPool := research.NewSubstateTaskPoolCli("substate-cli replay-fork", replayForkTask, ctx)

	segment, err := research.ParseBlockSegment(ctx.String(research.BlockSegmentFlag.Name))
	if err != nil {
		return fmt.Errorf("substate-cli replay-fork: error parsing block segment: %s", err)
	}

	err = taskPool.ExecuteSegment(segment)
	if err == nil {
		close(ReplayForkStatChan)
	}

	statWg.Wait()
	errstrSlice := make([]string, 0, len(ReplayForkStatMap))
	for errstr := range ReplayForkStatMap {
		errstrSlice = append(errstrSlice, errstr)
	}
	for _, errstr := range errstrSlice {
		stat := ReplayForkStatMap[errstr]
		count := stat.Count
		fmt.Printf("substate-cli replay-fork: %12v %s\n", count, errstr)
	}

	return err
}
