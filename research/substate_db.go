package research

import (
	"encoding/binary"
	"fmt"
	"io"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethdb"
	"google.golang.org/protobuf/proto"
)

const (
	Stage1SubstatePrefix = "1s" // stage1SubstatePrefix + block (64-bit) + tx (64-bit) -> substateRLP
	Stage1CodePrefix     = "1c" // stage1CodePrefix + codeHash (256-bit) -> code
)

func Stage1SubstateKey(block uint64, tx int) []byte {
	prefix := []byte(Stage1SubstatePrefix)

	blockTx := make([]byte, 16)
	binary.BigEndian.PutUint64(blockTx[0:8], block)
	binary.BigEndian.PutUint64(blockTx[8:16], uint64(tx))

	return append(prefix, blockTx...)
}

func DecodeStage1SubstateKey(key []byte) (block uint64, tx int, err error) {
	prefix := Stage1SubstatePrefix
	if len(key) != len(prefix)+8+8 {
		err = fmt.Errorf("invalid length of stage1 substate key: %v", len(key))
		return
	}
	if p := string(key[:len(prefix)]); p != prefix {
		err = fmt.Errorf("invalid prefix of stage1 substate key: %#x", p)
		return
	}
	blockTx := key[len(prefix):]
	block = binary.BigEndian.Uint64(blockTx[0:8])
	tx = int(binary.BigEndian.Uint64(blockTx[8:16]))
	return
}

func Stage1SubstateBlockPrefix(block uint64) []byte {
	prefix := []byte(Stage1SubstatePrefix)

	blockBytes := make([]byte, 8)
	binary.BigEndian.PutUint64(blockBytes[0:8], block)

	return append(prefix, blockBytes...)
}

func Stage1CodeKey(codeHash common.Hash) []byte {
	prefix := []byte(Stage1CodePrefix)
	return append(prefix, codeHash.Bytes()...)
}

func DecodeStage1CodeKey(key []byte) (codeHash common.Hash, err error) {
	prefix := Stage1CodePrefix
	if len(key) != len(prefix)+32 {
		err = fmt.Errorf("invalid length of stage1 code key: %v", len(key))
		return
	}
	if p := string(key[:2]); p != prefix {
		err = fmt.Errorf("invalid prefix of stage1 code key: %#x", p)
		return
	}
	codeHash = common.BytesToHash(key[len(prefix):])
	return
}

type BackendDatabase interface {
	ethdb.KeyValueReader
	ethdb.KeyValueWriter
	ethdb.Batcher
	ethdb.Iteratee
	ethdb.Stater
	ethdb.Compacter
	io.Closer
}

type SubstateDB struct {
	backend BackendDatabase
}

func NewSubstateDB(backend BackendDatabase) *SubstateDB {
	return &SubstateDB{backend: backend}
}

func (db *SubstateDB) Compact(start []byte, limit []byte) error {
	return db.backend.Compact(start, limit)
}

func (db *SubstateDB) Close() error {
	return db.backend.Close()
}

func CodeHash(code []byte) common.Hash {
	return crypto.Keccak256Hash(code)
}

var EmptyCodeHash = CodeHash(nil)

func (db *SubstateDB) HasCode(codeHash common.Hash) bool {
	if codeHash == EmptyCodeHash {
		return false
	}
	key := Stage1CodeKey(codeHash)
	has, err := db.backend.Has(key)
	if err != nil {
		panic(fmt.Errorf("record-replay: error checking bytecode for codeHash %s: %v", codeHash.Hex(), err))
	}
	return has
}

func (db *SubstateDB) GetCode(codeHash common.Hash) []byte {
	if codeHash == EmptyCodeHash {
		return nil
	}
	key := Stage1CodeKey(codeHash)
	code, err := db.backend.Get(key)
	if err != nil {
		panic(fmt.Errorf("record-replay: error getting code %s: %v", codeHash.Hex(), err))
	}
	return code
}

func (db *SubstateDB) PutCode(code []byte) {
	if len(code) == 0 {
		return
	}
	codeHash := crypto.Keccak256Hash(code)
	key := Stage1CodeKey(codeHash)
	err := db.backend.Put(key, code)
	if err != nil {
		panic(fmt.Errorf("record-replay: error putting code %s: %v", codeHash.Hex(), err))
	}
}

func (db *SubstateDB) HasSubstate(block uint64, tx int) bool {
	key := Stage1SubstateKey(block, tx)
	has, _ := db.backend.Has(key)
	return has
}

func (db *SubstateDB) GetSubstate(block uint64, tx int) *Substate {
	var err error

	key := Stage1SubstateKey(block, tx)
	value, err := db.backend.Get(key)
	if err != nil {
		panic(fmt.Errorf("record-replay: error getting substate %v_%v from substate DB: %v,", block, tx, err))
	}

	hashedSubstate := &Substate{}
	err = proto.Unmarshal(value, hashedSubstate)
	if err != nil {
		panic(fmt.Errorf("record-replay: error decoding substate %v_%v: %v", block, tx, err))
	}

	hashes := hashedSubstate.HashKeys()
	hashMap := make(map[common.Hash][]byte)
	for codeHash, _ := range hashes {
		code := db.GetCode(codeHash)
		hashMap[codeHash] = code
	}

	substate := hashedSubstate.UnhashedCopy(hashMap)

	return substate
}

func (db *SubstateDB) GetBlockSubstates(block uint64) map[int]*Substate {
	var err error

	txSubstateMap := make(map[int]*Substate)

	prefix := Stage1SubstateBlockPrefix(block)

	iter := db.backend.NewIterator(prefix, nil)
	for iter.Next() {
		key := iter.Key()
		value := iter.Value()

		b, tx, err := DecodeStage1SubstateKey(key)
		if err != nil {
			panic(fmt.Errorf("record-replay: invalid substate key found for block %v: %v", block, err))
		}

		if block != b {
			panic(fmt.Errorf("record-replay: GetBlockSubstates(%v) iterated substates from block %v", block, b))
		}

		hashedSubstate := &Substate{}
		err = proto.Unmarshal(value, hashedSubstate)
		if err != nil {
			panic(fmt.Errorf("error decoding substate hashed substate %v_%v: %v", block, tx, err))
		}

		hashes := hashedSubstate.HashKeys()
		hashMap := make(map[common.Hash][]byte)
		for codeHash, _ := range hashes {
			code := db.GetCode(codeHash)
			hashMap[codeHash] = code
		}

		substate := hashedSubstate.UnhashedCopy(hashMap)

		txSubstateMap[tx] = substate
	}
	iter.Release()
	err = iter.Error()
	if err != nil {
		panic(err)
	}

	return txSubstateMap
}

func (db *SubstateDB) PutSubstate(block uint64, tx int, substate *Substate) {
	var err error

	// replace code to code hashes in accounts and messages
	hashMap := substate.HashMap()
	hashedSubstate := substate.HashedCopy()

	// put deployed/creation code
	for codeHash, code := range hashMap {
		if codeHash != EmptyCodeHash {
			db.PutCode(code)
		}
	}

	// marshal hashed substate and put it to DB
	key := Stage1SubstateKey(block, tx)
	defer func() {
		if err != nil {
			panic(fmt.Errorf("record-replay: error putting substate %v_%v into substate DB: %v", block, tx, err))
		}
	}()
	value, err := proto.Marshal(hashedSubstate)
	if err != nil {
		panic(err)
	}
	err = db.backend.Put(key, value)
	if err != nil {
		panic(err)
	}
}

func (db *SubstateDB) DeleteSubstate(block uint64, tx int) {
	key := Stage1SubstateKey(block, tx)
	err := db.backend.Delete(key)
	if err != nil {
		panic(err)
	}
}
