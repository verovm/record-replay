// Copyright 2015 The go-ethereum Authors
// This file is part of the go-ethereum library.
//
// The go-ethereum library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-ethereum library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-ethereum library. If not, see <http://www.gnu.org/licenses/>.

package core

import (
	"fmt"
	"math/big"
	"os"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus"
	"github.com/ethereum/go-ethereum/consensus/misc"
	"github.com/ethereum/go-ethereum/core/rawdb"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/params"
	"github.com/ethereum/go-ethereum/research"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

// record-replay: record substates when true
var RecordSubstateFlag = false

// StateProcessor is a basic Processor, which takes care of transitioning
// state from one point to another.
//
// StateProcessor implements Processor.
type StateProcessor struct {
	config *params.ChainConfig // Chain configuration options
	bc     *BlockChain         // Canonical block chain
	engine consensus.Engine    // Consensus engine used for block rewards
}

// NewStateProcessor initialises a new StateProcessor.
func NewStateProcessor(config *params.ChainConfig, bc *BlockChain, engine consensus.Engine) *StateProcessor {
	return &StateProcessor{
		config: config,
		bc:     bc,
		engine: engine,
	}
}

// Process processes the state changes according to the Ethereum rules by running
// the transaction messages using the statedb and applying any rewards to both
// the processor (coinbase) and any included uncles.
//
// Process returns the receipts and logs accumulated during the process and
// returns the amount of gas that was used in the process. If any of the
// transactions failed to execute due to insufficient gas it will return an error.
func (p *StateProcessor) Process(block *types.Block, statedb *state.StateDB, cfg vm.Config) (types.Receipts, []*types.Log, uint64, error) {
	var (
		receipts    types.Receipts
		usedGas     = new(uint64)
		header      = block.Header()
		blockHash   = block.Hash()
		blockNumber = block.Number()
		allLogs     []*types.Log
		gp          = new(GasPool).AddGas(block.GasLimit())
	)
	// Mutate the block and state according to any hard-fork specs
	if p.config.DAOForkSupport && p.config.DAOForkBlock != nil && p.config.DAOForkBlock.Cmp(block.Number()) == 0 {
		misc.ApplyDAOHardFork(statedb)

		// record-replay: Finalise all DAO accounts, don't save them in substate
		if RecordSubstateFlag {
			if config := p.config; config.IsByzantium(header.Number) {
				statedb.Finalise(true)
			} else {
				statedb.Finalise(config.IsEIP158(header.Number))
			}
		}

	}
	blockContext := NewEVMBlockContext(header, p.bc, nil)
	vmenv := vm.NewEVM(blockContext, vm.TxContext{}, statedb, p.config, cfg)
	// Iterate over and process the individual transactions
	for i, tx := range block.Transactions() {
		msg, err := TransactionToMessage(tx, types.MakeSigner(p.config, header.Number), header.BaseFee)
		if err != nil {
			return nil, nil, 0, fmt.Errorf("could not apply tx %d [%v]: %w", i, tx.Hash().Hex(), err)
		}
		statedb.SetTxContext(tx.Hash(), i)

		// SetTxContext method will reset statedb.Research*
		// reset blockContext.ResearchBlockHashes manually
		vmenv.Context.ResearchBlockHashes = nil

		receipt, err := applyTransaction(msg, p.config, gp, statedb, blockNumber, blockHash, tx, usedGas, vmenv)
		if err != nil {
			return nil, nil, 0, fmt.Errorf("could not apply tx %d [%v]: %w", i, tx.Hash().Hex(), err)
		}

		// record-replay: save tx substate into DBs, merge block hashes to env
		if RecordSubstateFlag {
			substate := &research.Substate{}
			statedb.SaveSubstate(substate)

			// load blockContext again from vmenv because it does not hold a pointer
			blockContext := &vmenv.Context
			blockContext.SaveSubstate(substate)

			// nothing to reset
			msg.SaveSubstate(substate)

			// convert *types.Receipt to *research.Receipt for SaveSubstae method
			rr := research.NewResearchReceipt(receipt)
			rr.SaveSubstate(substate)

			research.PutSubstate(block.NumberU64(), i, substate)

			// check substate works for faithful replay
			go func(block uint64, tx int, substate *research.Substate) {
				err := CheckFaithfulReplay(block, tx, substate)
				if err != nil {
					panic(err)
				}
			}(block.NumberU64(), i, substate)
		}

		receipts = append(receipts, receipt)
		allLogs = append(allLogs, receipt.Logs...)
	}
	// Fail if Shanghai not enabled and len(withdrawals) is non-zero.
	withdrawals := block.Withdrawals()
	if len(withdrawals) > 0 && !p.config.IsShanghai(block.Time()) {
		return nil, nil, 0, fmt.Errorf("withdrawals before shanghai")
	}
	// Finalize the block, applying any consensus engine specific extras (e.g. block rewards)
	p.engine.Finalize(p.bc, header, statedb, block.Transactions(), block.Uncles(), withdrawals)

	return receipts, allLogs, *usedGas, nil
}

func applyTransaction(msg *Message, config *params.ChainConfig, gp *GasPool, statedb *state.StateDB, blockNumber *big.Int, blockHash common.Hash, tx *types.Transaction, usedGas *uint64, evm *vm.EVM) (*types.Receipt, error) {
	// Create a new context to be used in the EVM environment.
	txContext := NewEVMTxContext(msg)
	evm.Reset(txContext, statedb)

	// Apply the transaction to the current state (included in the env).
	result, err := ApplyMessage(evm, msg, gp)
	if err != nil {
		return nil, err
	}

	// Update the state with pending changes.
	var root []byte
	if config.IsByzantium(blockNumber) {
		statedb.Finalise(true)
	} else {
		root = statedb.IntermediateRoot(config.IsEIP158(blockNumber)).Bytes()
	}
	*usedGas += result.UsedGas

	// Create a new receipt for the transaction, storing the intermediate root and gas used
	// by the tx.
	receipt := &types.Receipt{Type: tx.Type(), PostState: root, CumulativeGasUsed: *usedGas}
	if result.Failed() {
		receipt.Status = types.ReceiptStatusFailed
	} else {
		receipt.Status = types.ReceiptStatusSuccessful
	}
	receipt.TxHash = tx.Hash()
	receipt.GasUsed = result.UsedGas

	// If the transaction created a contract, store the creation address in the receipt.
	if msg.To == nil {
		receipt.ContractAddress = crypto.CreateAddress(evm.TxContext.Origin, tx.Nonce())
	}

	// Set the receipt logs and create the bloom filter.
	receipt.Logs = statedb.GetLogs(tx.Hash(), blockNumber.Uint64(), blockHash)
	receipt.Bloom = types.CreateBloom(types.Receipts{receipt})
	receipt.BlockHash = blockHash
	receipt.BlockNumber = blockNumber
	receipt.TransactionIndex = uint(statedb.TxIndex())
	return receipt, err
}

// ApplyTransaction attempts to apply a transaction to the given state database
// and uses the input parameters for its environment. It returns the receipt
// for the transaction, gas used and an error if the transaction failed,
// indicating the block was invalid.
func ApplyTransaction(config *params.ChainConfig, bc ChainContext, author *common.Address, gp *GasPool, statedb *state.StateDB, header *types.Header, tx *types.Transaction, usedGas *uint64, cfg vm.Config) (*types.Receipt, error) {
	msg, err := TransactionToMessage(tx, types.MakeSigner(config, header.Number), header.BaseFee)
	if err != nil {
		return nil, err
	}
	// Create a new context to be used in the EVM environment
	blockContext := NewEVMBlockContext(header, bc, author)
	vmenv := vm.NewEVM(blockContext, vm.TxContext{}, statedb, config, cfg)
	return applyTransaction(msg, config, gp, statedb, header.Number, header.Hash(), tx, usedGas, vmenv)
}

// CheckFaithfulReplay checks faithful transaction replay with the given substate
// and store json files of substates if execution results are different.
func CheckFaithfulReplay(block uint64, tx int, substate *research.Substate) error {
	// InputAlloc
	statedb, _ := state.New(common.Hash{}, state.NewDatabase(rawdb.NewMemoryDatabase()), nil)
	statedb.LoadSubstate(substate)

	// BlockEnv
	blockContext := vm.BlockContext{
		CanTransfer: CanTransfer,
		Transfer:    Transfer,
	}
	blockContext.LoadSubstate(substate)
	blockNumber := blockContext.BlockNumber

	// TxMessage
	txMessage := &Message{}
	txMessage.LoadSubstate(substate)

	chainConfig := &params.ChainConfig{}
	*chainConfig = *params.MainnetChainConfig
	// disable DAOForkSupport, otherwise account states will be overwritten
	chainConfig.DAOForkSupport = false

	vmConfig := vm.Config{}

	evm := vm.NewEVM(blockContext, vm.TxContext{}, statedb, chainConfig, vmConfig)

	statedb.SetTxContext(common.Hash{}, tx)

	txContext := NewEVMTxContext(txMessage)
	evm.Reset(txContext, statedb)

	gaspool := new(GasPool).AddGas(blockContext.GasLimit)

	result, err := ApplyMessage(evm, txMessage, gaspool)
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

	eqAlloc := proto.Equal(substate.OutputAlloc, replaySubstate.OutputAlloc)
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
