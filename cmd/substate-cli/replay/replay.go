package replay

import (
	"fmt"
	"os"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/rawdb"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/ethereum/go-ethereum/params"
	"github.com/ethereum/go-ethereum/research"
	cli "github.com/urfave/cli/v2"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
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
	// InputAlloc
	statedb, _ := state.New(types.EmptyRootHash, state.NewDatabase(rawdb.NewMemoryDatabase()), nil)
	statedb.LoadSubstate(substate)

	// BlockEnv
	blockContext := vm.BlockContext{
		CanTransfer: core.CanTransfer,
		Transfer:    core.Transfer,
	}
	blockContext.LoadSubstate(substate)
	blockNumber := blockContext.BlockNumber

	// TxMessage
	txMessage := &core.Message{}
	txMessage.LoadSubstate(substate)

	chainConfig := &params.ChainConfig{}
	*chainConfig = *params.MainnetChainConfig
	// disable DAOForkSupport, otherwise account states will be overwritten
	chainConfig.DAOForkSupport = false

	vmConfig := vm.Config{}

	evm := vm.NewEVM(blockContext, vm.TxContext{}, statedb, chainConfig, vmConfig)

	statedb.SetTxContext(common.Hash{}, tx)

	txContext := core.NewEVMTxContext(txMessage)
	evm.Reset(txContext, statedb)

	gaspool := new(core.GasPool).AddGas(blockContext.GasLimit)

	result, err := core.ApplyMessage(evm, txMessage, gaspool)
	if err != nil {
		return err
	}

	if chainConfig.IsByzantium(blockNumber) {
		statedb.Finalise(true)
	} else {
		// No need for root hash, call  Finalise instead of IntermediateRoot
		statedb.Finalise(chainConfig.IsEIP158(blockNumber))
	}

	rr := &research.ResearchReceipt{}
	if result.Failed() {
		rr.Status = types.ReceiptStatusFailed
	} else {
		rr.Status = types.ReceiptStatusSuccessful
	}
	rr.Logs = statedb.GetLogs(common.Hash{}, blockContext.BlockNumber.Uint64(), common.Hash{})
	rr.Bloom = types.CreateBloom(types.Receipts{&types.Receipt{Logs: rr.Logs}})

	rr.GasUsed = result.UsedGas

	replaySubstate := &research.Substate{}
	statedb.SaveSubstate(replaySubstate)
	blockContext.SaveSubstate(replaySubstate)
	txMessage.SaveSubstate(replaySubstate)
	rr.SaveSubstate(replaySubstate)

	eqAlloc := proto.Equal(substate.InputAlloc, replaySubstate.InputAlloc) &&
		proto.Equal(substate.OutputAlloc, replaySubstate.OutputAlloc)
	eqResult := proto.Equal(substate.Result, replaySubstate.Result)

	if !(eqAlloc && eqResult) {
		fmt.Printf("block %v, tx %v, inconsistent output\n", block, tx)
		jm := protojson.MarshalOptions{
			Indent: "  ",
		}

		var b []byte

		b, _ = jm.Marshal(substate)
		os.WriteFile(fmt.Sprintf("record_substate_%v_%v.json", block, tx), b, 0644)
		b, _ = jm.Marshal(substate.HashedCopy())
		os.WriteFile(fmt.Sprintf("record_substate_%v_%v_hashed.json", block, tx), b, 0644)

		b, _ = jm.Marshal(replaySubstate)
		os.WriteFile(fmt.Sprintf("replay_substate_%v_%v.json", block, tx), b, 0644)
		b, _ = jm.Marshal(replaySubstate.HashedCopy())
		os.WriteFile(fmt.Sprintf("replay_substate_%v_%v_hashed.json", block, tx), b, 0644)

		fmt.Printf("Saved record/replay_substate_*.json files (bytes in base64)\n")

		return fmt.Errorf("not faithful replay - inconsistent output")
	}

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
		return fmt.Errorf("substate-cli replay: error parsing block segment: %w", err)
	}

	err = taskPool.ExecuteSegment(segment)

	return err
}
