// stage1-substate: research/substate.go

package research

import (
	"bytes"
	"fmt"
	"math/big"
	"path/filepath"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/rlp"

	"github.com/syndtr/goleveldb/leveldb"
	leveldb_errors "github.com/syndtr/goleveldb/leveldb/errors"
	leveldb_opt "github.com/syndtr/goleveldb/leveldb/opt"
)

var substateDir = filepath.Join("stage1-substate")
var substateDB, codeDB *leveldb.DB

func OpenSubstateDB() {
	fmt.Println("stage1-substate: OpenSubstateDB")

	var err error
	var opt leveldb_opt.Options
	var path string

	// increase BlockCacheCapacity to 1GiB
	opt.BlockCacheCapacity = 1 * leveldb_opt.GiB
	// decrease OpenFilesCacheCapacity to avoid "Too many file opened" error
	opt.OpenFilesCacheCapacity = 50

	dbNameMap := map[string]*leveldb.DB{
		"substate": nil,
		"code":     nil,
	}

	for name := range dbNameMap {
		var db *leveldb.DB
		path = filepath.Join(substateDir, name)
		db, err = leveldb.OpenFile(path, &opt)
		if _, corrupted := err.(*leveldb_errors.ErrCorrupted); corrupted {
			db, err = leveldb.RecoverFile(path, &opt)
		}
		if err != nil {
			panic(fmt.Errorf("error opening substate leveldb %s: %v", path, err))
		}

		fmt.Printf("stage1-substate: successfully opened %s leveldb\n", name)

		dbNameMap[name] = db
	}

	substateDB = dbNameMap["substate"]
	codeDB = dbNameMap["code"]
}

func OpenSubstateDBReadOnly() {
	fmt.Println("stage1-substate: OpenSubstateDB")

	var err error
	var opt leveldb_opt.Options
	var path string

	// increase BlockCacheCapacity to 1GiB
	opt.BlockCacheCapacity = 1 * leveldb_opt.GiB
	// decrease OpenFilesCacheCapacity to avoid "Too many file opened" error
	opt.OpenFilesCacheCapacity = 50
	// set ReadOnly flag to true
	opt.ReadOnly = true

	dbNameMap := map[string]*leveldb.DB{
		"substate": nil,
		"code":     nil,
	}

	for name := range dbNameMap {
		var db *leveldb.DB
		path = filepath.Join(substateDir, name)
		db, err = leveldb.OpenFile(path, &opt)
		if _, corrupted := err.(*leveldb_errors.ErrCorrupted); corrupted {
			db, err = leveldb.RecoverFile(path, &opt)
		}
		if err != nil {
			panic(fmt.Errorf("error opening substate leveldb %s: %v", path, err))
		}

		fmt.Printf("stage1-substate: successfully opened %s leveldb\n", name)

		dbNameMap[name] = db
	}

	substateDB = dbNameMap["substate"]
	codeDB = dbNameMap["code"]
}

func CloseSubstateDB() {
	defer fmt.Println("stage1-substate: CloseSubstateDB")

	dbNameMap := map[string]*leveldb.DB{
		"substate": substateDB,
		"code":     codeDB,
	}

	for name, db := range dbNameMap {
		db.Close()
		fmt.Printf("stage1-substate: successfully closed %s leveldb\n", name)
	}
}

func GetCode(codeHash common.Hash) []byte {
	code, err := codeDB.Get(codeHash.Bytes(), nil)
	if err != nil {
		panic(fmt.Errorf("stage1-substate: error getting code %s: %v", codeHash.Hex(), err))
	}
	return code
}

func PutCode(code []byte) {
	codeHash := crypto.Keccak256Hash(code)
	err := codeDB.Put(codeHash.Bytes(), code, nil)
	if err != nil {
		panic(fmt.Errorf("stage1-substate: error putting code %s: %v", codeHash.Hex(), err))
	}
}

// SubstateAccount is modification of GenesisAccount in core/genesis.go
type SubstateAccount struct {
	Nonce   uint64
	Balance *big.Int
	Storage map[common.Hash]common.Hash
	Code    []byte
}

func NewSubstateAccount(nonce uint64, balance *big.Int, code []byte) *SubstateAccount {
	return &SubstateAccount{
		Nonce:   nonce,
		Balance: new(big.Int).Set(balance),
		Storage: make(map[common.Hash]common.Hash),
		Code:    code,
	}
}

func (x *SubstateAccount) Equal(y *SubstateAccount) bool {
	if x == y {
		return true
	}

	if (x == nil || y == nil) && x != y {
		return false
	}

	equal := (x.Nonce == y.Nonce &&
		x.Balance.Cmp(y.Balance) == 0 &&
		bytes.Equal(x.Code, y.Code) &&
		len(x.Storage) == len(y.Storage))
	if !equal {
		return false
	}

	for k, xv := range x.Storage {
		yv, exist := y.Storage[k]
		if !(exist && xv == yv) {
			return false
		}
	}

	return true
}

func (sa *SubstateAccount) Copy() *SubstateAccount {
	saCopy := NewSubstateAccount(sa.Nonce, sa.Balance, sa.Code)

	for key, value := range sa.Storage {
		saCopy.Storage[key] = value
	}

	return saCopy
}

func (sa *SubstateAccount) CodeHash() common.Hash {
	return crypto.Keccak256Hash(sa.Code)
}

type SubstateAlloc map[common.Address]*SubstateAccount

func (x SubstateAlloc) Equal(y SubstateAlloc) bool {
	if len(x) != len(y) {
		return false
	}

	for k, xv := range x {
		yv, exist := y[k]
		if !(exist && xv.Equal(yv)) {
			return false
		}
	}

	return true
}

type SubstateEnv struct {
	Coinbase    common.Address
	Difficulty  *big.Int
	GasLimit    uint64
	Number      uint64
	Timestamp   uint64
	BlockHashes map[uint64]common.Hash
}

func NewSubstateEnv(b *types.Block, blockHashes map[uint64]common.Hash) *SubstateEnv {
	var env = &SubstateEnv{}

	env.Coinbase = b.Coinbase()
	env.Difficulty = new(big.Int).Set(b.Difficulty())
	env.GasLimit = b.GasLimit()
	env.Number = b.NumberU64()
	env.Timestamp = b.Time()
	env.BlockHashes = make(map[uint64]common.Hash)
	for num64, bhash := range blockHashes {
		env.BlockHashes[num64] = bhash
	}

	return env
}

func (x *SubstateEnv) Equal(y *SubstateEnv) bool {
	if x == y {
		return true
	}

	if (x == nil || y == nil) && x != y {
		return false
	}

	equal := (x.Coinbase == y.Coinbase &&
		x.Difficulty.Cmp(y.Difficulty) == 0 &&
		x.GasLimit == y.GasLimit &&
		x.Number == y.Number &&
		x.Timestamp == y.Timestamp &&
		len(x.BlockHashes) == len(y.BlockHashes))
	if !equal {
		return false
	}

	for k, xv := range x.BlockHashes {
		yv, exist := y.BlockHashes[k]
		if !(exist && xv == yv) {
			return false
		}
	}

	return true
}

type SubstateMessage struct {
	Nonce      uint64
	CheckNonce bool
	GasPrice   *big.Int
	Gas        uint64

	From  common.Address
	To    *common.Address // nil means contract creation
	Value *big.Int
	Data  []byte

	// for memoization
	dataHash *common.Hash
}

func NewSubstateMessage(msg *types.Message) *SubstateMessage {
	var smsg = &SubstateMessage{}

	smsg.Nonce = msg.Nonce()
	smsg.CheckNonce = msg.CheckNonce()
	smsg.GasPrice = msg.GasPrice()
	smsg.Gas = msg.Gas()

	smsg.From = msg.From()
	smsg.To = msg.To()
	smsg.Value = msg.Value()
	smsg.Data = msg.Data()

	return smsg
}

func (x *SubstateMessage) Equal(y *SubstateMessage) bool {
	if x == y {
		return true
	}

	if (x == nil || y == nil) && x != y {
		return false
	}

	equal := (x.Nonce == y.Nonce &&
		x.CheckNonce == y.CheckNonce &&
		x.GasPrice.Cmp(y.GasPrice) == 0 &&
		x.Gas == y.Gas &&
		x.From == y.From &&
		(x.To == y.To || (x.To != nil && y.To != nil && *x.To == *y.To)) &&
		x.Value.Cmp(y.Value) == 0 &&
		bytes.Equal(x.Data, y.Data))
	if !equal {
		return false
	}

	return true
}

func (msg *SubstateMessage) DataHash() common.Hash {
	if msg.dataHash == nil {
		dataHash := crypto.Keccak256Hash(msg.Data)
		msg.dataHash = &dataHash
	}
	return *msg.dataHash
}

func (msg *SubstateMessage) AsMessage() types.Message {
	return types.NewMessage(
		msg.From, msg.To, msg.Nonce, msg.Value,
		msg.Gas, msg.GasPrice, msg.Data, msg.CheckNonce)
}

// modification of types.Receipt
type SubstateResult struct {
	Status uint64
	Bloom  types.Bloom
	Logs   []*types.Log

	ContractAddress common.Address
	GasUsed         uint64
}

func NewSubstateResult(receipt *types.Receipt) *SubstateResult {
	var sr = &SubstateResult{}

	sr.Status = receipt.Status
	sr.Bloom = receipt.Bloom
	sr.Logs = receipt.Logs

	sr.ContractAddress = receipt.ContractAddress
	sr.GasUsed = receipt.GasUsed

	return sr
}

func (x *SubstateResult) Equal(y *SubstateResult) bool {
	if x == y {
		return true
	}

	if (x == nil || y == nil) && x != y {
		return false
	}

	equal := (x.Status == y.Status &&
		x.Bloom == y.Bloom &&
		len(x.Logs) == len(y.Logs) &&
		x.ContractAddress == y.ContractAddress &&
		x.GasUsed == y.GasUsed)
	if !equal {
		return false
	}

	for i, xl := range x.Logs {
		yl := y.Logs[i]

		equal := (xl.Address == yl.Address &&
			len(xl.Topics) == len(yl.Topics) &&
			bytes.Equal(xl.Data, yl.Data))
		if !equal {
			return false
		}

		for i, xt := range xl.Topics {
			yt := yl.Topics[i]
			if xt != yt {
				return false
			}
		}
	}

	return true
}

type Substate struct {
	InputAlloc  SubstateAlloc
	OutputAlloc SubstateAlloc
	Env         *SubstateEnv
	Message     *SubstateMessage
	Result      *SubstateResult
}

func NewSubstate(inputAlloc SubstateAlloc, outputAlloc SubstateAlloc, env *SubstateEnv, message *SubstateMessage, result *SubstateResult) *Substate {
	return &Substate{
		InputAlloc:  inputAlloc,
		OutputAlloc: outputAlloc,
		Env:         env,
		Message:     message,
		Result:      result,
	}
}

func (x *Substate) Equal(y *Substate) bool {
	if x == y {
		return true
	}

	if (x == nil || y == nil) && x != y {
		return false
	}

	equal := (x.InputAlloc.Equal(y.InputAlloc) &&
		x.OutputAlloc.Equal(y.OutputAlloc) &&
		x.Env.Equal(y.Env) &&
		x.Message.Equal(y.Message) &&
		x.Result.Equal(y.Result))
	return equal
}

func HasSubstate(block uint64, tx int) bool {
	key := []byte(fmt.Sprintf("%v_%v", block, tx))
	has, _ := substateDB.Has(key, nil)
	return has
}

func GetSubstate(block uint64, tx int) *Substate {
	var err error

	key := []byte(fmt.Sprintf("%v_%v", block, tx))
	defer func() {
		if err != nil {
			panic(fmt.Errorf("stage1-substate: error getting substate %s from substate DB", key))
		}
	}()

	value, err := substateDB.Get(key, nil)
	if err != nil {
		return nil
	}

	substate := Substate{}

	substate.InputAlloc = make(SubstateAlloc)
	substate.OutputAlloc = make(SubstateAlloc)
	substate.Env = &SubstateEnv{}
	substate.Message = &SubstateMessage{}
	substate.Result = &SubstateResult{}

	err = rlp.DecodeBytes(value, &substate)
	if err != nil {
		return nil
	}

	return &substate
}

func PutSubstate(block uint64, tx int, substate *Substate) {
	var err error

	// put deployed/creation code
	for _, account := range substate.InputAlloc {
		PutCode(account.Code)
	}
	for _, account := range substate.OutputAlloc {
		PutCode(account.Code)
	}
	if msg := substate.Message; msg.To == nil {
		PutCode(msg.Data)
	}

	key := []byte(fmt.Sprintf("%v_%v", block, tx))
	defer func() {
		if err != nil {
			panic(fmt.Errorf("stage1-substate: error putting substate %s into substate DB", key))
		}
	}()

	value, err := rlp.EncodeToBytes(substate)
	if err != nil {
		return
	}

	err = substateDB.Put(key, value, nil)
	if err != nil {
		return
	}
}

func ExportSubstate(dirpath string, first, last uint64) error {
	return fmt.Errorf("export-substate is not impelemented yet")
}
