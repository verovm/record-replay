package research

import (
	"math/big"
	"sort"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

type SubstateAccountRLP struct {
	Nonce    uint64
	Balance  *big.Int
	CodeHash common.Hash
	Storage  [][2]common.Hash
}

func NewSubstateAccountRLP(sa *SubstateAccount) *SubstateAccountRLP {
	var saRLP SubstateAccountRLP

	saRLP.Nonce = sa.Nonce
	saRLP.Balance = new(big.Int).Set(sa.Balance)
	saRLP.CodeHash = sa.CodeHash()
	sortedKeys := []common.Hash{}
	for key := range sa.Storage {
		sortedKeys = append(sortedKeys, key)
	}
	sort.Slice(sortedKeys, func(i, j int) bool {
		return sortedKeys[i].Big().Cmp(sortedKeys[j].Big()) < 0
	})
	for _, key := range sortedKeys {
		value := sa.Storage[key]
		saRLP.Storage = append(saRLP.Storage, [2]common.Hash{key, value})
	}

	return &saRLP
}

func (sa *SubstateAccount) SetRLP(saRLP *SubstateAccountRLP, db *SubstateDB) {
	sa.Balance = saRLP.Balance
	sa.Nonce = saRLP.Nonce
	sa.Code = db.GetCode(saRLP.CodeHash)
	sa.Storage = make(map[common.Hash]common.Hash)
	for i := range saRLP.Storage {
		sa.Storage[saRLP.Storage[i][0]] = saRLP.Storage[i][1]
	}
}

type SubstateAllocRLP struct {
	Addresses []common.Address
	Accounts  []*SubstateAccountRLP
}

func NewSubstateAllocRLP(alloc SubstateAlloc) SubstateAllocRLP {
	var allocRLP SubstateAllocRLP

	allocRLP.Addresses = []common.Address{}
	allocRLP.Accounts = []*SubstateAccountRLP{}
	for addr := range alloc {
		allocRLP.Addresses = append(allocRLP.Addresses, addr)
	}
	sort.Slice(allocRLP.Addresses, func(i, j int) bool {
		return allocRLP.Addresses[i].Hash().Big().Cmp(allocRLP.Addresses[j].Hash().Big()) < 0
	})

	for _, addr := range allocRLP.Addresses {
		account := alloc[addr]
		allocRLP.Accounts = append(allocRLP.Accounts, NewSubstateAccountRLP(account))
	}

	return allocRLP
}

func (alloc *SubstateAlloc) SetRLP(allocRLP SubstateAllocRLP, db *SubstateDB) {
	*alloc = make(SubstateAlloc)
	for i, addr := range allocRLP.Addresses {
		var sa SubstateAccount

		saRLP := allocRLP.Accounts[i]
		sa.Balance = saRLP.Balance
		sa.Nonce = saRLP.Nonce
		sa.Code = db.GetCode(saRLP.CodeHash)
		sa.Storage = make(map[common.Hash]common.Hash)
		for i := range saRLP.Storage {
			sa.Storage[saRLP.Storage[i][0]] = saRLP.Storage[i][1]
		}

		(*alloc)[addr] = &sa
	}
}

type legacySubstateEnvRLP struct {
	Coinbase    common.Address
	Difficulty  *big.Int
	GasLimit    uint64
	Number      uint64
	Timestamp   uint64
	BlockHashes [][2]common.Hash
}

func (envRLP *SubstateEnvRLP) setLegacyRLP(lenvRLP *legacySubstateEnvRLP) {
	envRLP.Coinbase = lenvRLP.Coinbase
	envRLP.Difficulty = lenvRLP.Difficulty
	envRLP.GasLimit = lenvRLP.GasLimit
	envRLP.Number = lenvRLP.Number
	envRLP.Timestamp = lenvRLP.Timestamp
	envRLP.BlockHashes = lenvRLP.BlockHashes
}

type SubstateEnvRLP struct {
	Coinbase    common.Address
	Difficulty  *big.Int
	GasLimit    uint64
	Number      uint64
	Timestamp   uint64
	BlockHashes [][2]common.Hash

	BaseFee *common.Hash `rlp:"nil"` // missing in substate DB from Geth <= v1.10.3
}

func NewSubstateEnvRLP(env *SubstateEnv) *SubstateEnvRLP {
	var envRLP SubstateEnvRLP

	envRLP.Coinbase = env.Coinbase
	envRLP.Difficulty = env.Difficulty
	envRLP.GasLimit = env.GasLimit
	envRLP.Number = env.Number
	envRLP.Timestamp = env.Timestamp

	sortedNum64 := []uint64{}
	for num64 := range env.BlockHashes {
		sortedNum64 = append(sortedNum64, num64)
	}
	for _, num64 := range sortedNum64 {
		num := common.BigToHash(new(big.Int).SetUint64(num64))
		bhash := env.BlockHashes[num64]
		pair := [2]common.Hash{num, bhash}
		envRLP.BlockHashes = append(envRLP.BlockHashes, pair)
	}

	envRLP.BaseFee = nil
	if env.BaseFee != nil {
		baseFeeHash := common.BigToHash(env.BaseFee)
		envRLP.BaseFee = &baseFeeHash
	}

	return &envRLP
}

func (env *SubstateEnv) SetRLP(envRLP *SubstateEnvRLP, db *SubstateDB) {
	env.Coinbase = envRLP.Coinbase
	env.Difficulty = envRLP.Difficulty
	env.GasLimit = envRLP.GasLimit
	env.Number = envRLP.Number
	env.Timestamp = envRLP.Timestamp
	env.BlockHashes = make(map[uint64]common.Hash)
	for i := range envRLP.BlockHashes {
		pair := envRLP.BlockHashes[i]
		num64 := pair[0].Big().Uint64()
		bhash := pair[1]
		env.BlockHashes[num64] = bhash
	}

	env.BaseFee = nil
	if envRLP.BaseFee != nil {
		env.BaseFee = envRLP.BaseFee.Big()
	}
}

type legacySubstateMessageRLP struct {
	Nonce      uint64
	CheckNonce bool
	GasPrice   *big.Int
	Gas        uint64

	From  common.Address
	To    *common.Address `rlp:"nil"` // nil means contract creation
	Value *big.Int
	Data  []byte

	InitCodeHash *common.Hash `rlp:"nil"` // NOT nil for contract creation
}

func (msgRLP *SubstateMessageRLP) setLegacyRLP(lmsgRLP *legacySubstateMessageRLP) {
	msgRLP.Nonce = lmsgRLP.Nonce
	msgRLP.CheckNonce = lmsgRLP.CheckNonce
	msgRLP.GasPrice = lmsgRLP.GasPrice
	msgRLP.Gas = lmsgRLP.Gas

	msgRLP.From = lmsgRLP.From
	msgRLP.To = lmsgRLP.To
	msgRLP.Value = new(big.Int).Set(lmsgRLP.Value)
	msgRLP.Data = lmsgRLP.Data

	msgRLP.InitCodeHash = lmsgRLP.InitCodeHash

	msgRLP.AccessList = nil

	// Same behavior as LegacyTx.gasFeeCap() and LegacyTx.gasTipCap()
	msgRLP.GasFeeCap = lmsgRLP.GasPrice
	msgRLP.GasTipCap = lmsgRLP.GasPrice
}

type berlinSubstateMessageRLP struct {
	Nonce      uint64
	CheckNonce bool
	GasPrice   *big.Int
	Gas        uint64

	From  common.Address
	To    *common.Address `rlp:"nil"` // nil means contract creation
	Value *big.Int
	Data  []byte

	InitCodeHash *common.Hash `rlp:"nil"` // NOT nil for contract creation

	AccessList types.AccessList // missing in substate DB from Geth v1.9.x
}

func (msgRLP *SubstateMessageRLP) setBerlinRLP(bmsgRLP *berlinSubstateMessageRLP) {
	msgRLP.Nonce = bmsgRLP.Nonce
	msgRLP.CheckNonce = bmsgRLP.CheckNonce
	msgRLP.GasPrice = bmsgRLP.GasPrice
	msgRLP.Gas = bmsgRLP.Gas

	msgRLP.From = bmsgRLP.From
	msgRLP.To = bmsgRLP.To
	msgRLP.Value = new(big.Int).Set(bmsgRLP.Value)
	msgRLP.Data = bmsgRLP.Data

	msgRLP.InitCodeHash = bmsgRLP.InitCodeHash

	msgRLP.AccessList = nil

	// Same behavior as AccessListTx.gasFeeCap() and AccessListTx.gasTipCap()
	msgRLP.GasFeeCap = bmsgRLP.GasPrice
	msgRLP.GasTipCap = bmsgRLP.GasPrice
}

type SubstateMessageRLP struct {
	Nonce      uint64
	CheckNonce bool
	GasPrice   *big.Int
	Gas        uint64

	From  common.Address
	To    *common.Address `rlp:"nil"` // nil means contract creation
	Value *big.Int
	Data  []byte

	InitCodeHash *common.Hash `rlp:"nil"` // NOT nil for contract creation

	AccessList types.AccessList // missing in substate DB from Geth v1.9.x

	GasFeeCap *big.Int // missing in substate DB from Geth <= v1.10.3
	GasTipCap *big.Int // missing in substate DB from Geth <= v1.10.3
}

func NewSubstateMessageRLP(msg *SubstateMessage) *SubstateMessageRLP {
	var msgRLP SubstateMessageRLP

	msgRLP.Nonce = msg.Nonce
	msgRLP.CheckNonce = msg.CheckNonce
	msgRLP.GasPrice = msg.GasPrice
	msgRLP.Gas = msg.Gas

	msgRLP.From = msg.From
	msgRLP.To = msg.To
	msgRLP.Value = new(big.Int).Set(msg.Value)
	msgRLP.Data = msg.Data

	msgRLP.InitCodeHash = nil

	if msgRLP.To == nil {
		// put contract creation init code into codeDB
		dataHash := msg.DataHash()
		msgRLP.Data = nil
		msgRLP.InitCodeHash = &dataHash
	}

	msgRLP.AccessList = msg.AccessList

	msgRLP.GasFeeCap = msg.GasFeeCap
	msgRLP.GasTipCap = msg.GasTipCap

	return &msgRLP
}

func (msg *SubstateMessage) SetRLP(msgRLP *SubstateMessageRLP, db *SubstateDB) {
	msg.Nonce = msgRLP.Nonce
	msg.CheckNonce = msgRLP.CheckNonce
	msg.GasPrice = msgRLP.GasPrice
	msg.Gas = msgRLP.Gas

	msg.From = msgRLP.From
	msg.To = msgRLP.To
	msg.Value = msgRLP.Value
	msg.Data = msgRLP.Data

	if msgRLP.To == nil {
		msg.Data = db.GetCode(*msgRLP.InitCodeHash)
	}

	msg.AccessList = msgRLP.AccessList

	msg.GasFeeCap = msgRLP.GasFeeCap
	msg.GasTipCap = msgRLP.GasTipCap
}

type SubstateResultRLP struct {
	Status uint64
	Bloom  types.Bloom
	Logs   []*types.Log

	ContractAddress common.Address
	GasUsed         uint64
}

func NewSubstateResultRLP(result *SubstateResult) *SubstateResultRLP {
	var resultRLP SubstateResultRLP

	resultRLP.Status = result.Status
	resultRLP.Bloom = result.Bloom
	resultRLP.Logs = result.Logs

	resultRLP.ContractAddress = result.ContractAddress
	resultRLP.GasUsed = result.GasUsed

	return &resultRLP
}

func (result *SubstateResult) SetRLP(resultRLP *SubstateResultRLP, db *SubstateDB) {
	result.Status = resultRLP.Status
	result.Bloom = resultRLP.Bloom
	result.Logs = resultRLP.Logs

	result.ContractAddress = resultRLP.ContractAddress
	result.GasUsed = resultRLP.GasUsed
}

type legacySubstateRLP struct {
	InputAlloc  SubstateAllocRLP
	OutputAlloc SubstateAllocRLP
	Env         *legacySubstateEnvRLP
	Message     *legacySubstateMessageRLP
	Result      *SubstateResultRLP
}

func (substateRLP *SubstateRLP) setLegacyRLP(lsubstateRLP *legacySubstateRLP) {
	substateRLP.InputAlloc = lsubstateRLP.InputAlloc
	substateRLP.OutputAlloc = lsubstateRLP.OutputAlloc
	substateRLP.Env = &SubstateEnvRLP{}
	substateRLP.Env.setLegacyRLP(lsubstateRLP.Env)
	substateRLP.Message = &SubstateMessageRLP{}
	substateRLP.Message.setLegacyRLP(lsubstateRLP.Message)
	substateRLP.Result = lsubstateRLP.Result
}

type berlinSubstateRLP struct {
	InputAlloc  SubstateAllocRLP
	OutputAlloc SubstateAllocRLP
	Env         *legacySubstateEnvRLP
	Message     *berlinSubstateMessageRLP
	Result      *SubstateResultRLP
}

func (substateRLP *SubstateRLP) setBerlinRLP(bsubstateRLP *berlinSubstateRLP) {
	substateRLP.InputAlloc = bsubstateRLP.InputAlloc
	substateRLP.OutputAlloc = bsubstateRLP.OutputAlloc
	substateRLP.Env = &SubstateEnvRLP{}
	substateRLP.Env.setLegacyRLP(bsubstateRLP.Env)
	substateRLP.Message = &SubstateMessageRLP{}
	substateRLP.Message.setBerlinRLP(bsubstateRLP.Message)
	substateRLP.Result = bsubstateRLP.Result
}

type SubstateRLP struct {
	InputAlloc  SubstateAllocRLP
	OutputAlloc SubstateAllocRLP
	Env         *SubstateEnvRLP
	Message     *SubstateMessageRLP
	Result      *SubstateResultRLP
}

func NewSubstateRLP(substate *Substate) *SubstateRLP {
	var substateRLP SubstateRLP

	substateRLP.InputAlloc = NewSubstateAllocRLP(substate.InputAlloc)
	substateRLP.OutputAlloc = NewSubstateAllocRLP(substate.OutputAlloc)
	substateRLP.Env = NewSubstateEnvRLP(substate.Env)
	substateRLP.Message = NewSubstateMessageRLP(substate.Message)
	substateRLP.Result = NewSubstateResultRLP(substate.Result)

	return &substateRLP
}

func (substate *Substate) SetRLP(substateRLP *SubstateRLP, db *SubstateDB) {
	substate.InputAlloc = make(SubstateAlloc)
	substate.OutputAlloc = make(SubstateAlloc)
	substate.Env = &SubstateEnv{}
	substate.Message = &SubstateMessage{}
	substate.Result = &SubstateResult{}

	substate.InputAlloc.SetRLP(substateRLP.InputAlloc, db)
	substate.OutputAlloc.SetRLP(substateRLP.OutputAlloc, db)
	substate.Env.SetRLP(substateRLP.Env, db)
	substate.Message.SetRLP(substateRLP.Message, db)
	substate.Result.SetRLP(substateRLP.Result, db)
}
