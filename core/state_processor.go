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
	"github.com/google/go-cmp/cmp"
	"google.golang.org/protobuf/proto"
)

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
		if config := p.config; config.IsByzantium(header.Number) {
			statedb.Finalise(true)
		} else {
			statedb.Finalise(config.IsEIP158(header.Number))
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
		receipt, err := applyTransaction(msg, p.config, gp, statedb, blockNumber, blockHash, tx, usedGas, vmenv)
		if err != nil {
			return nil, nil, 0, fmt.Errorf("could not apply tx %d [%v]: %w", i, tx.Hash().Hex(), err)
		}

		// record-replay: save tx substate into DBs, merge block hashes to env
		{
			// SetTxContext method will reset statedb.Research*
			substate := &research.Substate{}
			statedb.SaveSubstate(substate)

			// reset blockContext.ResearchBlockHashes manually
			blockContext.SaveSubstate(substate)
			blockContext.ResearchBlockHashes = nil

			// nothing to reset
			msg.SaveSubstate(substate)

			// convert *types.Receipt to *research.Receipt for SaveSubstae method
			rr := research.NewResearchReceipt(receipt)
			rr.SaveSubstate(substate)

			research.PutSubstate(block.NumberU64(), i, substate)

			// TODO: check substate works for faithful replay
			go func(block uint64, tx int, substate *research.Substate) {
				err := checkFaithfulReplay(block, tx, substate)
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

// checkFaithfulReplay checks faithful transaction replay with the given substate
func checkFaithfulReplay(block uint64, tx int, substate *research.Substate) error {
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
	db := rawdb.NewMemoryDatabase()
	statedb, _ := state.New(common.Hash{}, state.NewDatabase(db), nil)
	statedb.LoadSubstate(substate)

	var (
		gaspool   = new(GasPool)
		blockHash = common.Hash{0x01}
		txHash    = common.Hash{0x02}
		txIndex   = tx
	)

	blockCtx := vm.BlockContext{
		CanTransfer: CanTransfer,
		Transfer:    Transfer,
	}
	blockCtx.LoadSubstate(substate)
	gaspool.AddGas(blockCtx.GasLimit)

	msg := &Message{}
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
	result, err := ApplyMessage(evm, msg, gaspool)
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

	replaySubstate := &research.Substate{}
	statedb.SaveSubstate(replaySubstate)
	blockCtx.SaveSubstate(replaySubstate)
	msg.SaveSubstate(replaySubstate)
	rr.SaveSubstate(replaySubstate)

	eqAlloc := proto.Equal(substate.OutputAlloc, replaySubstate.OutputAlloc)
	eqResult := proto.Equal(substate.Result, replaySubstate.Result)

	if !(eqAlloc && eqResult) {
		fmt.Printf("block %v, tx %v, inconsistent output report BEGIN\n", block, tx)
		x := substate.HashedCopy()
		y := replaySubstate.HashedCopy()
		d := cmp.Diff(x, y)
		fmt.Printf("+record -replay\n%s\n", d)
		fmt.Printf("block %v, tx %v, inconsistent output report END\n", block, tx)
		return fmt.Errorf("not faithful replay - inconsistent output")
	}

	return nil
}
