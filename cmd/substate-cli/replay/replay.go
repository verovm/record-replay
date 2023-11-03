package replay

import (
	"fmt"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/params"
	"github.com/ethereum/go-ethereum/research"
	cli "github.com/urfave/cli/v2"
)

// record-replay: substate-cli replay command
var ReplayCommand = &cli.Command{
	Action: replayAction,
	Name:   "replay",
	Usage:  "replay transactions and check output consistency",
	Flags: []cli.Flag{
		research.WorkersFlag,
		research.SkipTransferTxsFlag,
		research.SkipCallTxsFlag,
		research.SkipCreateTxsFlag,
		research.SubstateDirFlag,
		research.BlockSegmentFlag,
	},
	Description: `
substate-cli replay executes transactions in the given block segment
and check output consistency for faithful replaying.`,
	Category: "replay",
}

// replayTask replays a transaction substate
func replayTask(block uint64, tx int, substate *research.Substate, taskPool *research.SubstateTaskPool) error {
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

	statedb.SetTxContext(txHash, tx)

	evm := vm.NewEVM(blockCtx, txCtx, statedb, chainConfig, vmConfig)
	result, err := core.ApplyMessage(evm, msg, gaspool)
	if err != nil {
		return err
	}

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
		rr.ContractAddress = crypto.CreateAddress(evm.TxContext.Origin, msg.Nonce)
	}
	rr.Logs = statedb.GetLogs(txHash, blockCtx.BlockNumber.Uint64(), blockHash)
	rr.Bloom = types.CreateBloom(types.Receipts{&types.Receipt{Logs: rr.Logs}})

	outSubstate := &research.Substate{}
	statedb.SaveSubstate(outSubstate)
	blockCtx.SaveSubstate(outSubstate)
	msg.SaveSubstate(outSubstate)
	rr.SaveSubstate(outSubstate)

	// TODO: compare substate and

	/*
		r := outputResult.Equal(evmResult)
		a := outputAlloc.Equal(evmAlloc)
		if !(r && a) {
			fmt.Println()
			fmt.Printf("block %v, tx %v, inconsistent output report BEGIN\n", block, tx)
			var jbytes []byte
			if !r {
				fmt.Printf("inconsistent result\n")
				jbytes, _ = json.MarshalIndent(outputResult, "", " ")
				fmt.Printf("==== outputResult:\n%s\n", jbytes)
				// Clear log fields which are not saved in DB
				rlpBytes, _ := rlp.EncodeToBytes(evmResult.Logs)
				_ = rlp.DecodeBytes(rlpBytes, &evmResult.Logs)
				jbytes, _ = json.MarshalIndent(evmResult, "", " ")
				fmt.Printf("==== evmResult:\n%s\n", jbytes)
				fmt.Println()
			}
			if !a {
				fmt.Printf("inconsistent output\n")
				addrs := make(map[common.Address]struct{})
				for k, _ := range outputAlloc {
					addrs[k] = struct{}{}
				}
				for k, _ := range evmAlloc {
					addrs[k] = struct{}{}
				}
				for k, _ := range addrs {
					iv := inputAlloc[k]
					ov := outputAlloc[k]
					ev := evmAlloc[k]
					if ov.Equal(ev) {
						continue
					}
					kHex := k.Hex()
					ivCopy := iv.Copy()
					ovCopy := ov.Copy()
					evCopy := ev.Copy()
					ivCopy.Code = nil
					ovCopy.Code = nil
					evCopy.Code = nil
					fmt.Printf("account address: %s\n", kHex)
					fmt.Printf("==== inputAlloc ====\n")
					jbytes, _ = json.MarshalIndent(ivCopy, "", " ")
					fmt.Printf("%s\nCodeHash: %s\n", jbytes, iv.CodeHash())
					fmt.Printf("==== outputAlloc ====\n")
					jbytes, _ = json.MarshalIndent(ovCopy, "", " ")
					fmt.Printf("%s\nCodeHash: %s\n", jbytes, ov.CodeHash())
					fmt.Printf("==== evmAlloc ====\n")
					jbytes, _ = json.MarshalIndent(evCopy, "", " ")
					fmt.Printf("%s\nCodeHash: %s\n", jbytes, ev.CodeHash())
					fmt.Println()
				}
			}

			// information to search the transaction traces
			fmt.Printf("message from %s\n", inputMessage.From.Hex())
			fmt.Printf("message to %s\n", inputMessage.To.Hex())
			fmt.Printf("result status: %v\n", outputResult.Status)
			if !r {
				fmt.Printf("inconsistent result\n")
			}
			if !a {
				fmt.Printf("inconsistent alloc\n")
			}
			fmt.Printf("block %v, tx %v, inconsistent output report END\n", block, tx)
			fmt.Println()

			return fmt.Errorf("inconsistent output")
		}
	*/

	return nil
}

// record-replay: func replayAction for replay command
func replayAction(ctx *cli.Context) error {
	var err error

	research.SetSubstateFlags(ctx)
	research.OpenSubstateDBReadOnly()
	defer research.CloseSubstateDB()

	taskPool := research.NewSubstateTaskPoolCli("substate-cli replay", replayTask, ctx)

	segment, err := research.ParseBlockSegment(ctx.String(research.BlockSegmentFlag.Name))
	if err != nil {
		return fmt.Errorf("substate-cli replay: error parsing block segment: %s", err)
	}

	err = taskPool.ExecuteSegment(segment)

	return err
}
