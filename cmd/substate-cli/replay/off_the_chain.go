package replay

import (
	"github.com/ethereum/go-ethereum/core/rawdb"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/research"
)

// NewOffTheChainStateDB returns an empty in-memory *state.StateDB without disk caches
func NewOffTheChainStateDB() *state.StateDB {
	db := rawdb.NewMemoryDatabase()
	state, _ := state.New(types.EmptyRootHash, state.NewDatabase(db), nil)
	return state
}

// MakeOffTheChainStateDB returns an in-memory *state.StateDB initialized with alloc
func MakeOffTheChainStateDB(substate *research.Substate) *state.StateDB {
	statedb := NewOffTheChainStateDB()
	statedb.LoadSubstate(substate)
	return statedb
}
