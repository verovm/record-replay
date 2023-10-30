package replay

import (
	"fmt"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/rawdb"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/research"
)

// NewOffTheChainStateDB returns an empty in-memory *state.StateDB without disk caches
func NewOffTheChainStateDB() *state.StateDB {
	db := rawdb.NewMemoryDatabase()
	state, _ := state.New(common.Hash{}, state.NewDatabase(db), nil)
	return state
}

// MakeOffTheChainStateDB returns an in-memory *state.StateDB initialized with alloc
func MakeOffTheChainStateDB(alloc research.SubstateAlloc) *state.StateDB {
	statedb := NewOffTheChainStateDB()
	for addr, a := range alloc {
		statedb.SetCode(addr, a.Code)
		statedb.SetNonce(addr, a.Nonce)
		statedb.SetBalance(addr, a.Balance)
		// DON'T USE SetStorage because it makes REVERT and dirtyStorage unavailble
		for k, v := range a.Storage {
			statedb.SetState(addr, k, v)
		}
	}
	// Commit and re-open to start with a clean state.
	_, err := statedb.Commit(false)
	if err != nil {
		panic(fmt.Errorf("error calling statedb.Commit() in MakeOffTheChainStateDB(): %v", err))
	}
	return statedb
}
