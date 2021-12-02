package replay

import (
	"errors"
	"fmt"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/rawdb"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/research"
	"github.com/ethereum/go-ethereum/trie"
)

// offTheChainDB is state.cachingDB clone without disk caches
type offTheChainDB struct {
	db *trie.Database
}

// OpenTrie opens the main account trie at a specific root hash.
func (db *offTheChainDB) OpenTrie(root common.Hash) (state.Trie, error) {
	tr, err := trie.NewSecure(root, db.db)
	if err != nil {
		return nil, err
	}
	return tr, nil
}

// OpenStorageTrie opens the storage trie of an account.
func (db *offTheChainDB) OpenStorageTrie(addrHash, root common.Hash) (state.Trie, error) {
	tr, err := trie.NewSecure(root, db.db)
	if err != nil {
		return nil, err
	}
	return tr, nil
}

// CopyTrie returns an independent copy of the given trie.
func (db *offTheChainDB) CopyTrie(t state.Trie) state.Trie {
	switch t := t.(type) {
	case *trie.SecureTrie:
		return t.Copy()
	default:
		panic(fmt.Errorf("unknown trie type %T", t))
	}
}

// ContractCode retrieves a particular contract's code.
func (db *offTheChainDB) ContractCode(addrHash, codeHash common.Hash) ([]byte, error) {
	code := rawdb.ReadCode(db.db.DiskDB(), codeHash)
	if len(code) > 0 {
		return code, nil
	}
	return nil, errors.New("not found")
}

// ContractCodeWithPrefix retrieves a particular contract's code. If the
// code can't be found in the cache, then check the existence with **new**
// db scheme.
func (db *offTheChainDB) ContractCodeWithPrefix(addrHash, codeHash common.Hash) ([]byte, error) {
	code := rawdb.ReadCodeWithPrefix(db.db.DiskDB(), codeHash)
	if len(code) > 0 {
		return code, nil
	}
	return nil, errors.New("not found")
}

// ContractCodeSize retrieves a particular contracts code's size.
func (db *offTheChainDB) ContractCodeSize(addrHash, codeHash common.Hash) (int, error) {
	code, err := db.ContractCode(addrHash, codeHash)
	return len(code), err
}

// TrieDB retrieves any intermediate trie-node caching layer.
func (db *offTheChainDB) TrieDB() *trie.Database {
	return db.db
}

// NewOffTheChainStateDB returns an empty in-memory *state.StateDB without disk caches
func NewOffTheChainStateDB() *state.StateDB {
	// backend in-memory key-value database
	kvdb := rawdb.NewMemoryDatabase()

	// zeroed trie.Config to disable Cache, Journal, Preimages, ...
	zerodConfig := &trie.Config{}
	tdb := trie.NewDatabaseWithConfig(kvdb, zerodConfig)

	sdb := &offTheChainDB{
		db: tdb,
	}

	statedb, err := state.New(common.Hash{}, sdb, nil)
	if err != nil {
		panic(fmt.Errorf("error calling state.New() in NewOffTheChainDB(): %v", err))
	}
	return statedb
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
