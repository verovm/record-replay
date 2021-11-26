package research

import (
	"encoding/json"
	"fmt"
	"math/big"
	"runtime"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ethereum/go-ethereum/cmd/evm/internal/t8ntool"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/math"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/rawdb"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/params"
	"github.com/ethereum/go-ethereum/research"
	cli "gopkg.in/urfave/cli.v1"
)

// stage1-substate: func transitionSubstateTransaction
func ApplySubstate(ctx *cli.Context, block uint64, tx int, substate *research.Substate) error {
	inputAlloc := substate.InputAlloc
	inputEnv := substate.Env
	inputMessage := substate.Message

	outputAlloc := substate.OutputAlloc
	outputResult := substate.Result

	var (
		vmConfig    vm.Config
		chainConfig *params.ChainConfig
		pre         t8ntool.Prestate
		getTracerFn func(txIndex int) (tracer vm.EVMLogger, err error)
	)

	vmConfig = vm.Config{}

	chainConfig = &params.ChainConfig{}
	*chainConfig = *params.MainnetChainConfig
	// disable DAOForkSupport, otherwise account states will be overwritten
	chainConfig.DAOForkSupport = false

	// copy inputAlloc to Prestate.Pre
	pre.Pre = make(core.GenesisAlloc)
	for addr, account := range inputAlloc {
		ga := core.GenesisAccount{}
		ga.Nonce = account.Nonce
		ga.Balance = account.Balance
		ga.Storage = account.Storage
		ga.Code = account.Code
		pre.Pre[addr] = ga
	}
	// copy inputEnv to Prestate.Env
	pre.Env.Coinbase = inputEnv.Coinbase
	pre.Env.Difficulty = inputEnv.Difficulty
	pre.Env.GasLimit = inputEnv.GasLimit
	pre.Env.Number = inputEnv.Number
	pre.Env.Timestamp = inputEnv.Timestamp
	pre.Env.BlockHashes = make(map[math.HexOrDecimal64]common.Hash)
	for num64, bhash := range inputEnv.BlockHashes {
		pre.Env.BlockHashes[math.HexOrDecimal64(num64)] = bhash
	}

	getTracerFn = func(txIndex int) (tracer vm.EVMLogger, err error) {
		return nil, nil
	}

	var hashError error
	getHash := func(num uint64) common.Hash {
		if pre.Env.BlockHashes == nil {
			hashError = fmt.Errorf("getHash(%d) invoked, no blockhashes provided", num)
			return common.Hash{}
		}
		h, ok := pre.Env.BlockHashes[math.HexOrDecimal64(num)]
		if !ok {
			hashError = fmt.Errorf("getHash(%d) invoked, blockhash for that block not provided", num)
		}
		return h
	}

	// Apply Message
	var (
		statedb   = t8ntool.MakePreState(rawdb.NewMemoryDatabase(), pre.Pre)
		gaspool   = new(core.GasPool)
		blockHash = common.Hash{0x13, 0x37}
		txIndex   = tx
	)

	gaspool.AddGas(pre.Env.GasLimit)
	vmContext := vm.BlockContext{
		CanTransfer: core.CanTransfer,
		Transfer:    core.Transfer,
		Coinbase:    pre.Env.Coinbase,
		BlockNumber: new(big.Int).SetUint64(pre.Env.Number),
		Time:        new(big.Int).SetUint64(pre.Env.Timestamp),
		Difficulty:  pre.Env.Difficulty,
		GasLimit:    pre.Env.GasLimit,
		GetHash:     getHash,
		// GasPrice and Origin needs to be set per transaction
	}

	msg := inputMessage.AsMessage()

	tracer, err := getTracerFn(txIndex)
	if err != nil {
		return err
	}
	vmConfig.Tracer = tracer
	vmConfig.Debug = tracer != nil
	statedb.Prepare(common.Hash{}, txIndex)
	//vmContext.GasPrice = msg.GasPrice()
	//vmContext.Origin = msg.From()
	txContext := core.NewEVMTxContext(msg)
	snapshot := statedb.Snapshot()
	evm := vm.NewEVM(vmContext, txContext, statedb, chainConfig, vmConfig)

	msgResult, err := core.ApplyMessage(evm, msg, gaspool)
	if err != nil {
		statedb.RevertToSnapshot(snapshot)
		return err
	}

	if hashError != nil {
		return t8ntool.NewError(t8ntool.ErrorMissingBlockhash, hashError)
	}

	if chainConfig.IsByzantium(vmContext.BlockNumber) {
		statedb.Finalise(true)
	} else {
		statedb.IntermediateRoot(chainConfig.IsEIP158(vmContext.BlockNumber))
	}

	evmResult := &research.SubstateResult{}
	if msgResult.Failed() {
		evmResult.Status = types.ReceiptStatusFailed
	} else {
		evmResult.Status = types.ReceiptStatusSuccessful
	}
	evmResult.Logs = statedb.GetLogs(common.Hash{}, blockHash)
	evmResult.Bloom = types.BytesToBloom(types.LogsBloom(evmResult.Logs))
	// If the transaction created a contract, store the creation address in the receipt.
	if msg.To() == nil {
		evmResult.ContractAddress = crypto.CreateAddress(evm.TxContext.Origin, msg.Nonce())
	}
	evmResult.GasUsed = msgResult.UsedGas

	evmAlloc := statedb.ResearchPostAlloc

	if r, a := outputResult.Equal(evmResult), outputAlloc.Equal(evmAlloc); !(r && a) {
		fmt.Printf("%v_%v\n", block, tx)
		if !r {
			fmt.Printf("inconsistent output: result\n")
		}
		if !a {
			fmt.Printf("inconsistent output: alloc\n")
		}
		var jbytes []byte
		jbytes, _ = json.MarshalIndent(inputAlloc, "", " ")
		fmt.Printf("inputAlloc:\n%s\n", jbytes)
		jbytes, _ = json.MarshalIndent(inputEnv, "", " ")
		fmt.Printf("inputEnv:\n%s\n", jbytes)
		jbytes, _ = json.MarshalIndent(inputMessage, "", " ")
		fmt.Printf("inputMessage:\n%s\n", jbytes)
		jbytes, _ = json.MarshalIndent(outputAlloc, "", " ")
		fmt.Printf("outputAlloc:\n%s\n", jbytes)
		jbytes, _ = json.MarshalIndent(evmAlloc, "", " ")
		fmt.Printf("evmAlloc:\n%s\n", jbytes)
		jbytes, _ = json.MarshalIndent(outputResult, "", " ")
		fmt.Printf("outputResult:\n%s\n", jbytes)
		jbytes, _ = json.MarshalIndent(evmResult, "", " ")
		fmt.Printf("evmResult:\n%s\n", jbytes)
		return fmt.Errorf("inconsistent output")
	}

	return nil
}

// stage1-substate: func transitionSubstate for t8n-substate task
func transitionSubstateBlock(ctx *cli.Context, block int64) (int64, error) {
	var err error
	var numTx int64 = 0

	for block, tx := uint64(block), 0; ; tx++ {

		if !research.HasSubstate(block, tx) {
			break
		}

		substate := research.GetSubstate(block, tx)

		alloc := substate.InputAlloc
		msg := substate.Message

		to := msg.To
		if ctx.Bool(SkipTransferTxsFlag.Name) && to != nil {
			// skip regular transactions (ETH transfer)
			if account, exist := alloc[*to]; !exist || len(account.Code) == 0 {
				continue
			}
		}
		if ctx.Bool(SkipCallTxsFlag.Name) && to != nil {
			// skip CALL trasnactions with contract bytecode
			if account, exist := alloc[*to]; exist && len(account.Code) > 0 {
				continue
			}
		}
		if ctx.Bool(SkipCreateTxsFlag.Name) && to == nil {
			// skip CREATE transactions
			continue
		}

		err = ApplySubstate(ctx, block, tx, substate)
		if err != nil {
			return numTx, fmt.Errorf("stage1-substate: transitionSubstateTransaction %v_%v: %v", block, tx, err)
		}

		numTx++
	}

	return numTx, nil
}

// stage1-substate: func TransitionSubstate for t8n-substate command
func TransitionSubstate(ctx *cli.Context) error {
	if len(ctx.Args()) != 2 {
		return fmt.Errorf("stage1-substate: transition-substate (t8n-substate) command requires exactly 2 arguments")
	}

	start := time.Now()

	first, ferr := strconv.ParseInt(ctx.Args().Get(0), 10, 64)
	last, lerr := strconv.ParseInt(ctx.Args().Get(1), 10, 64)
	if ferr != nil || lerr != nil {
		return fmt.Errorf("stage1-substate: TransitionSubstate: error in parsing parameters: block number not an integer")
	}
	if first < 0 || last < 0 {
		return fmt.Errorf("stage1-substate: TransitionSubstate: error: block number must be greater than 0")
	}
	if first > last {
		return fmt.Errorf("stage1-substate: TransitionSubstate: error: first block has larger number than last block")
	}

	research.OpenSubstateDBReadOnly()
	defer research.CloseSubstateDB()

	var totalNumBlock, totalNumTx int64
	defer func() {
		duration := time.Since(start) + 1*time.Nanosecond
		sec := duration.Seconds()

		nb, nt := atomic.LoadInt64(&totalNumBlock), atomic.LoadInt64(&totalNumTx)
		blkPerSec := float64(nb) / sec
		txPerSec := float64(nt) / sec
		fmt.Printf("stage1-substate: TransitionSubstate: total #block = %v\n", nb)
		fmt.Printf("stage1-substate: TransitionSubstate: total #tx    = %v\n", nt)
		fmt.Printf("stage1-substate: TransitionSubstate: %.2f blk/s, %.2f tx/s\n", blkPerSec, txPerSec)
		fmt.Printf("stage1-substate: TransitionSubstate done in %v\n", duration.Round(1*time.Millisecond))
	}()

	numWorker := ctx.Int(WorkersFlag.Name)
	// numProcs + work producer (1) + main thread (1)
	numProcs := numWorker + 2
	if goMaxProcs := runtime.GOMAXPROCS(0); goMaxProcs < numProcs {
		runtime.GOMAXPROCS(numProcs)
	}

	fmt.Printf("stage1-substate: TransitionSubstate: #CPU = %v, #worker = %v, #thread = %v\n", runtime.NumCPU(), numWorker, runtime.GOMAXPROCS(0))

	workChan := make(chan int64, numWorker*10)
	doneChan := make(chan interface{}, numWorker*10)
	stopChan := make(chan struct{}, numWorker)
	wg := sync.WaitGroup{}
	defer func() {
		// stop all workers
		for i := 0; i < numWorker; i++ {
			stopChan <- struct{}{}
		}
		// stop work producer (1)
		stopChan <- struct{}{}

		wg.Wait()
		close(workChan)
		close(doneChan)
	}()
	for i := 0; i < numWorker; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			for {
				select {

				case block := <-workChan:
					nt, err := transitionSubstateBlock(ctx, block)
					atomic.AddInt64(&totalNumTx, nt)
					atomic.AddInt64(&totalNumBlock, 1)
					if err != nil {
						doneChan <- err
					} else {
						doneChan <- block
					}

				case <-stopChan:
					return

				}
			}
		}()
	}

	wg.Add(1)
	go func() {
		defer wg.Done()

		for block := first; block <= last; block++ {
			select {

			case workChan <- block:
				continue

			case <-stopChan:
				return

			}
		}
	}()

	var lastSec float64
	var lastNumBlock, lastNumTx int64
	for block := first; block <= last; block++ {
		data := <-doneChan
		switch t := data.(type) {

		case int64:
			block := data.(int64)
			if block%10000 == 0 {
				duration := time.Since(start) + 1*time.Nanosecond
				sec := duration.Seconds()

				nb, nt := atomic.LoadInt64(&totalNumBlock), atomic.LoadInt64(&totalNumTx)
				blkPerSec := float64(nb-lastNumBlock) / (sec - lastSec)
				txPerSec := float64(nt-lastNumTx) / (sec - lastSec)
				fmt.Printf("stage1-substate: elapsed time: %v, number = %v\n", duration.Round(1*time.Millisecond), block)
				fmt.Printf("stage1-substate: %.2f blk/s, %.2f tx/s\n", blkPerSec, txPerSec)

				lastSec, lastNumBlock, lastNumTx = sec, nb, nt
			}

		case error:
			err := data.(error)
			return err

		default:
			return fmt.Errorf("stage1-substate: TransitionSubstate: unknown type %T value from doneChan", t)

		}
	}

	return nil
}
