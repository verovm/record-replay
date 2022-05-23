package replay

import (
	"bytes"
	"errors"
	"fmt"
	"math/big"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/params"
	"github.com/ethereum/go-ethereum/research"
	"github.com/ethereum/go-ethereum/tests"
	cli "gopkg.in/urfave/cli.v1"
)

// record-replay: replay-fork command
var ReplayForkCommand = cli.Command{
	Action:    replayForkAction,
	Name:      "replay-fork",
	Usage:     "executes and check output consistency of all transactions in the range with the given hard-fork",
	ArgsUsage: "<blockNumFirst> <blockNumLast>",
	Flags: []cli.Flag{
		research.WorkersFlag,
		research.SkipTransferTxsFlag,
		research.SkipCallTxsFlag,
		research.SkipCreateTxsFlag,
		HardForkFlag,
		research.SubstateDirFlag,
	},
	Description: `
The replay-fork command requires two arguments:
<blockNumFirst> <blockNumLast>

<blockNumFirst> and <blockNumLast> are the first and
last block of the inclusive range of blocks to replay transactions.

--hard-fork parameter is recommended for this command.`,
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

var HardForkFlag = cli.Int64Flag{
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
	inputAlloc := substate.InputAlloc
	inputEnv := substate.Env
	inputMessage := substate.Message

	outputAlloc := substate.OutputAlloc
	outputResult := substate.Result

	var (
		vmConfig    vm.Config
		getTracerFn func(txIndex int, txHash common.Hash) (tracer vm.EVMLogger, err error)
	)

	vmConfig = vm.Config{}

	getTracerFn = func(txIndex int, txHash common.Hash) (tracer vm.EVMLogger, err error) {
		return nil, nil
	}

	// getHash returns zero for block hash that does not exist
	getHash := func(num uint64) common.Hash {
		if inputEnv.BlockHashes == nil {
			return common.Hash{}
		}
		h := inputEnv.BlockHashes[num]
		return h
	}

	// Apply Message
	var (
		statedb   = MakeOffTheChainStateDB(inputAlloc)
		gaspool   = new(core.GasPool)
		txHash    = common.Hash{0x01}
		blockHash = common.Hash{0x02}
		txIndex   = tx
	)

	gaspool.AddGas(inputEnv.GasLimit)
	blockCtx := vm.BlockContext{
		CanTransfer: core.CanTransfer,
		Transfer:    core.Transfer,
		Coinbase:    inputEnv.Coinbase,
		BlockNumber: new(big.Int).SetUint64(inputEnv.Number),
		Time:        new(big.Int).SetUint64(inputEnv.Timestamp),
		Difficulty:  inputEnv.Difficulty,
		GasLimit:    inputEnv.GasLimit,
		GetHash:     getHash,
	}
	// If currentBaseFee is defined, add it to the vmContext.
	if inputEnv.BaseFee != nil {
		blockCtx.BaseFee = new(big.Int).Set(inputEnv.BaseFee)
	}

	msg := inputMessage.AsMessage()

	tracer, err := getTracerFn(txIndex, txHash)
	if err != nil {
		return err
	}
	vmConfig.Tracer = tracer
	vmConfig.Debug = (tracer != nil)
	statedb.Prepare(txHash, txIndex)

	txCtx := vm.TxContext{
		GasPrice: msg.GasPrice(),
		Origin:   msg.From(),
	}

	chainConfig := ReplayForkChainConfig
	if chainConfig.IsLondon(blockCtx.BlockNumber) && blockCtx.BaseFee == nil {
		// If blockCtx.BaseFee is nil, assume blockCtx.BaseFee is zero
		blockCtx.BaseFee = new(big.Int)
	}
	evm := vm.NewEVM(blockCtx, txCtx, statedb, chainConfig, vmConfig)
	snapshot := statedb.Snapshot()
	msgResult, err := core.ApplyMessage(evm, msg, gaspool)

	if err != nil {
		statedb.RevertToSnapshot(snapshot)
		stat = &ReplayForkStat{
			Count:  1,
			ErrStr: strings.Split(err.Error(), ":")[0],
		}
		return nil
	}

	if chainConfig.IsByzantium(blockCtx.BlockNumber) {
		statedb.Finalise(true)
	} else {
		statedb.IntermediateRoot(chainConfig.IsEIP158(blockCtx.BlockNumber))
	}

	evmResult := &research.SubstateResult{}
	if msgResult.Failed() {
		evmResult.Status = types.ReceiptStatusFailed
	} else {
		evmResult.Status = types.ReceiptStatusSuccessful
	}
	evmResult.Logs = statedb.GetLogs(txHash, blockHash)
	evmResult.Bloom = types.BytesToBloom(types.LogsBloom(evmResult.Logs))
	if to := msg.To(); to == nil {
		evmResult.ContractAddress = crypto.CreateAddress(evm.TxContext.Origin, msg.Nonce())
	}
	evmResult.GasUsed = msgResult.UsedGas

	evmAlloc := statedb.ResearchPostAlloc

	if r, a := outputResult.Equal(evmResult), outputAlloc.Equal(evmAlloc); !(r && a) {
		if outputResult.Status == types.ReceiptStatusSuccessful &&
			evmResult.Status == types.ReceiptStatusSuccessful {
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
				if !bytes.Equal(account1.Code, account2.Code) {
					stat = &ReplayForkStat{
						Count:  1,
						ErrStr: fmt.Sprintf("%v", ErrReplayForkInvalidAlloc),
					}
					return nil
				}

				// check storage
				storage1 := account1.Storage
				storage2 := account2.Storage
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
			if evmResult.GasUsed > outputResult.GasUsed {
				stat = &ReplayForkStat{
					Count:  1,
					ErrStr: fmt.Sprintf("%v", ErrReplayForkMoreGas),
				}
				return nil
			}

			// less gas
			if evmResult.GasUsed < outputResult.GasUsed {
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

		} else if outputResult.Status == types.ReceiptStatusSuccessful &&
			evmResult.Status == types.ReceiptStatusFailed {
			// if output was successful but evm failed, return runtime error
			stat = &ReplayForkStat{
				Count:  1,
				ErrStr: fmt.Sprintf("%v", msgResult.Err),
			}
			return nil
		} else {
			// misc (logs, ...)
			stat = &ReplayForkStat{
				Count:  1,
				ErrStr: fmt.Sprintf("%v", ErrReplayForkMisc),
			}
			return nil
		}
	}

	return nil
}

// record-replay: func replayForkAction for replay-fork command
func replayForkAction(ctx *cli.Context) error {
	var err error

	if len(ctx.Args()) != 2 {
		return fmt.Errorf("substate-cli replay-fork command requires exactly 2 arguments")
	}

	first, ferr := strconv.ParseInt(strings.ReplaceAll(ctx.Args().Get(0), "_", ""), 10, 64)
	last, lerr := strconv.ParseInt(strings.ReplaceAll(ctx.Args().Get(1), "_", ""), 10, 64)
	if ferr != nil || lerr != nil {
		return fmt.Errorf("substate-cli replay-fork: error in parsing parameters: block number not an integer")
	}
	if first < 0 || last < 0 {
		return fmt.Errorf("substate-cli replay-fork: error: block number must be greater than 0")
	}
	if first > last {
		return fmt.Errorf("substate-cli replay-fork: error: first block has larger number than last block")
	}

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

	taskPool := research.NewSubstateTaskPool("substate-cli replay-fork", replayForkTask, uint64(first), uint64(last), ctx)
	err = taskPool.Execute()
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
