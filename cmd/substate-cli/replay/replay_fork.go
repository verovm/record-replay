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
	"github.com/ethereum/go-ethereum/core/rawdb"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/core/vm"
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
	15_537_394: "Paris (The Merge)",
	17_034_870: "Shanghai",
	19_426_587: "Cancun",
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
		s += "Hard-fork block number, won't change block number in BlockEnv for NUMBER instruction"

		hardForkNums := make([]int64, 0, len(HardForkName))
		for num64 := range HardForkName {
			hardForkNums = append(hardForkNums, num64)
		}
		sort.Slice(hardForkNums, func(i, j int) bool { return hardForkNums[i] < hardForkNums[j] })
		for _, num64 := range hardForkNums {
			s += fmt.Sprintf(", %v: %s", num64, HardForkName[num64])
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

const (
	ReplayForkResult_Equal        = "equal output in replay-fork"
	ReplayForkResult_OutOfGas     = "out of gas in replay-fork"
	ReplayForkResult_InvalidAlloc = "invalid alloc in replay-fork"
	ReplayForkResult_MoreGas      = "more gas in replay-fork"
	ReplayForkResult_LessGas      = "less gas in replay-fork"
	ReplayForkResult_Misc         = "misc in replay-fork"
)

func replayForkTask(block uint64, tx int, substate *research.Substate, taskPool *research.SubstateTaskPool) error {
	// replay-fork
	var stat *ReplayForkStat
	defer func() {
		if stat != nil {
			ReplayForkStatChan <- stat
		}
	}()

	// InputAlloc
	statedb, _ := state.New(types.EmptyRootHash, state.NewDatabase(rawdb.NewMemoryDatabase()), nil)
	statedb.LoadSubstate(substate)

	// BlockEnv
	blockContext := &vm.BlockContext{
		CanTransfer: core.CanTransfer,
		Transfer:    core.Transfer,
	}
	blockContext.LoadSubstate(substate)
	blockNumber := blockContext.BlockNumber

	// Prevent segfault in opRandom function
	if blockContext.Random == nil {
		blockContext.Random = &common.Hash{}
	}

	// TxMessage
	txMessage := &core.Message{}
	txMessage.LoadSubstate(substate)

	// replay-fork hard fork configuration
	chainConfig := ReplayForkChainConfig
	if chainConfig.IsLondon(blockNumber) && blockContext.BaseFee == nil {
		// If blockCtx.BaseFee is nil, assume blockCtx.BaseFee is zero
		blockContext.BaseFee = new(big.Int)
	}

	vmConfig := vm.Config{}

	evm := vm.NewEVM(*blockContext, vm.TxContext{}, statedb, chainConfig, vmConfig)

	statedb.SetTxContext(common.Hash{}, tx)

	txContext := core.NewEVMTxContext(txMessage)
	evm.Reset(txContext, statedb)

	gaspool := new(core.GasPool).AddGas(blockContext.GasLimit)

	result, err := core.ApplyMessage(evm, txMessage, gaspool)
	if err != nil {
		erruw := errors.Unwrap(err)
		if erruw == nil {
			erruw = err
		}
		stat = &ReplayForkStat{
			Count:  1,
			ErrStr: fmt.Sprintf("runtime error: %v", erruw),
		}
		return nil
	}

	if chainConfig.IsByzantium(blockNumber) {
		statedb.Finalise(true)
	} else {
		// No need for root hash, call  Finalise instead of IntermediateRoot
		statedb.Finalise(chainConfig.IsEIP158(blockNumber))
	}

	replaySubstate := &research.Substate{}
	statedb.SaveSubstate(replaySubstate)

	blockContext = &evm.Context
	blockContext.SaveSubstate(replaySubstate)

	txMessage.SaveSubstate(replaySubstate)

	rr := &research.ResearchReceipt{}
	if result.Failed() {
		rr.Status = types.ReceiptStatusFailed
	} else {
		rr.Status = types.ReceiptStatusSuccessful
	}
	rr.Logs = statedb.GetLogs(common.Hash{}, blockContext.BlockNumber.Uint64(), common.Hash{})
	rr.Bloom = types.CreateBloom(types.Receipts{&types.Receipt{Logs: rr.Logs}})
	rr.GasUsed = result.UsedGas
	rr.SaveSubstate(replaySubstate)

	eqAlloc := proto.Equal(substate.InputAlloc, replaySubstate.InputAlloc) &&
		proto.Equal(substate.OutputAlloc, replaySubstate.OutputAlloc)
	eqResult := proto.Equal(substate.Result, replaySubstate.Result)

	if eqAlloc && eqResult {
		// same transaction output state and result
		stat = &ReplayForkStat{
			Count:  1,
			ErrStr: ReplayForkResult_Equal,
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
	msgErr := result.Err

	if *outputResult.Status == types.ReceiptStatusSuccessful &&
		*evmResult.Status == types.ReceiptStatusFailed {
		// if output was successful but evm failed, return runtime error
		stat = &ReplayForkStat{
			Count:  1,
			ErrStr: fmt.Sprintf("tx error: %v", msgErr),
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
				ErrStr: ReplayForkResult_OutOfGas,
			}
			return nil
		}
		for addr := range outputAlloc {
			account1 := outputAlloc[addr]
			account2 := evmAlloc[addr]
			if account2 == nil {
				stat = &ReplayForkStat{
					Count:  1,
					ErrStr: ReplayForkResult_InvalidAlloc,
				}
				return nil
			}

			// check nonce
			if account1.Nonce != account2.Nonce {
				stat = &ReplayForkStat{
					Count:  1,
					ErrStr: ReplayForkResult_InvalidAlloc,
				}
				return nil
			}

			// check code
			if !bytes.Equal(account1.GetCode(), account2.GetCode()) {
				stat = &ReplayForkStat{
					Count:  1,
					ErrStr: ReplayForkResult_InvalidAlloc,
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
					ErrStr: ReplayForkResult_InvalidAlloc,
				}
				return nil
			}
			for k, v1 := range storage1 {
				if v2, exist := storage2[k]; !exist || v1 != v2 {
					stat = &ReplayForkStat{
						Count:  1,
						ErrStr: ReplayForkResult_InvalidAlloc,
					}
					return nil
				}
			}
		}

		// more gas
		if *evmResult.GasUsed > *outputResult.GasUsed {
			stat = &ReplayForkStat{
				Count:  1,
				ErrStr: ReplayForkResult_MoreGas,
			}
			return nil
		}

		// less gas
		if *evmResult.GasUsed < *outputResult.GasUsed {
			stat = &ReplayForkStat{
				Count:  1,
				ErrStr: ReplayForkResult_LessGas,
			}
			return nil
		}

		// misc: logs, ...
		stat = &ReplayForkStat{
			Count:  1,
			ErrStr: ReplayForkResult_Misc,
		}
		return nil

	}

	// misc (logs, ...)
	stat = &ReplayForkStat{
		Count:  1,
		ErrStr: ReplayForkResult_Misc,
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
	case 15_537_394:
		*ReplayForkChainConfig = *tests.Forks["Merge"]
	case 17_034_870:
		*ReplayForkChainConfig = *tests.Forks["Shanghai"]
	case 19_426_587:
		*ReplayForkChainConfig = *tests.Forks["Cancun"]

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
		return fmt.Errorf("substate-cli replay-fork: error parsing block segment: %w", err)
	}

	err = taskPool.ExecuteSegment(segment)
	if err == nil {
		close(ReplayForkStatChan)
	}

	statWg.Wait()
	var totalCount int64 = 0
	errstrSlice := make([]string, 0, len(ReplayForkStatMap))
	for errstr, stat := range ReplayForkStatMap {
		errstrSlice = append(errstrSlice, errstr)
		totalCount += stat.Count
	}
	sort.Slice(errstrSlice, func(i, j int) bool {
		return ReplayForkStatMap[errstrSlice[i]].Count < ReplayForkStatMap[errstrSlice[j]].Count
	})
	fmt.Printf("substate-cli replay-fork: %12s %7s %s\n", "Count", "Ratio", "Result")
	for _, errstr := range errstrSlice {
		stat := ReplayForkStatMap[errstr]
		count := stat.Count
		percentStr := fmt.Sprintf("%.02f%%", float64(count)/float64(totalCount)*100)
		fmt.Printf("substate-cli replay-fork: %12v %7s %s\n", count, percentStr, errstr)
	}

	return err
}
