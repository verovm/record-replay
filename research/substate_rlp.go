// stage1-substate: research/substate.go

package research

import (
	"encoding/json"
	"io"
	"math/big"
	"sort"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/rlp"
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

func (sa *SubstateAccount) SetRLP(saRLP *SubstateAccountRLP) {
	sa.Balance = saRLP.Balance
	sa.Nonce = saRLP.Nonce
	sa.Code = GetCode(saRLP.CodeHash)
	sa.Storage = make(map[common.Hash]common.Hash)
	for i := range saRLP.Storage {
		sa.Storage[saRLP.Storage[i][0]] = saRLP.Storage[i][1]
	}
}

func (sa SubstateAccount) EncodeRLP(w io.Writer) error {
	return rlp.Encode(w, NewSubstateAccountRLP(&sa))
}

func (sa *SubstateAccount) DecodeRLP(s *rlp.Stream) error {
	var err error
	var saRLP SubstateAccountRLP

	err = s.Decode(&saRLP)
	if err != nil {
		return err
	}

	sa.SetRLP(&saRLP)

	return nil
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

func (alloc *SubstateAlloc) SetRLP(allocRLP SubstateAllocRLP) {
	*alloc = make(SubstateAlloc)
	for i, addr := range allocRLP.Addresses {
		var sa SubstateAccount

		saRLP := allocRLP.Accounts[i]
		sa.Balance = saRLP.Balance
		sa.Nonce = saRLP.Nonce
		sa.Code = GetCode(saRLP.CodeHash)
		sa.Storage = make(map[common.Hash]common.Hash)
		for i := range saRLP.Storage {
			sa.Storage[saRLP.Storage[i][0]] = saRLP.Storage[i][1]
		}

		(*alloc)[addr] = &sa
	}
}

func (alloc SubstateAlloc) EncodeRLP(w io.Writer) error {
	return rlp.Encode(w, NewSubstateAllocRLP(alloc))
}

func (alloc *SubstateAlloc) DecodeRLP(s *rlp.Stream) (err error) {
	var allocRLP SubstateAllocRLP

	err = s.Decode(&allocRLP)
	if err != nil {
		return err
	}

	alloc.SetRLP(allocRLP)

	return nil
}

type SubstateEnvRLP struct {
	Coinbase    common.Address
	Difficulty  *big.Int
	GasLimit    uint64
	Number      uint64
	Timestamp   uint64
	BlockHashes [][2]common.Hash
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

	return &envRLP
}

func (env *SubstateEnv) SetRLP(envRLP *SubstateEnvRLP) {
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
}

func (env SubstateEnv) EncodeRLP(w io.Writer) error {
	return rlp.Encode(w, NewSubstateEnvRLP(&env))
}

func (env *SubstateEnv) DecodeRLP(s *rlp.Stream) error {
	var err error
	var envRLP SubstateEnvRLP

	err = s.Decode(&envRLP)
	if err != nil {
		return err
	}

	env.SetRLP(&envRLP)

	return nil
}

type SubstateMessageRLP struct {
	Nonce      uint64
	CheckNonce bool
	Gas        uint64
	GasPrice   *big.Int
	GasFeeCap  *big.Int
	GasTipCap  *big.Int

	From  common.Address
	To    *common.Address `rlp:"nil"` // nil means contract creation
	Value *big.Int
	Data  []byte

	AccessList []byte

	InitCodeHash *common.Hash `rlp:"nil"` // NOT nil for contract creation
}

func NewSubstateMessageRLP(msg *SubstateMessage) *SubstateMessageRLP {
	var msgRLP SubstateMessageRLP

	msgRLP.Nonce = msg.Nonce
	msgRLP.Gas = msg.Gas
	msgRLP.GasPrice = msg.GasPrice
	msgRLP.GasFeeCap = msg.GasFeeCap
	msgRLP.GasTipCap = msg.GasTipCap

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

	msgRLP.AccessList, _ = json.Marshal(msg.AccessList)

	return &msgRLP
}

func (msg *SubstateMessage) SetRLP(msgRLP *SubstateMessageRLP) {
	msg.Nonce = msgRLP.Nonce
	msg.Gas = msgRLP.Gas
	msg.GasPrice = msgRLP.GasPrice
	msg.GasFeeCap = msgRLP.GasFeeCap
	msg.GasTipCap = msgRLP.GasTipCap

	msg.From = msgRLP.From
	msg.To = msgRLP.To
	msg.Value = msgRLP.Value
	msg.Data = msgRLP.Data

	if msgRLP.To == nil {
		msg.Data = GetCode(*msgRLP.InitCodeHash)
	}

	_ = json.Unmarshal(msgRLP.AccessList, &msg.AccessList)
}

func (msg SubstateMessage) EncodeRLP(w io.Writer) error {
	return rlp.Encode(w, NewSubstateMessageRLP(&msg))
}

func (msg *SubstateMessage) DecodeRLP(s *rlp.Stream) error {
	var err error
	var msgRLP SubstateMessageRLP

	err = s.Decode(&msgRLP)
	if err != nil {
		return err
	}

	msg.SetRLP(&msgRLP)

	return nil
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

func (result *SubstateResult) SetRLP(resultRLP *SubstateResultRLP) {
	result.Status = resultRLP.Status
	result.Bloom = resultRLP.Bloom
	result.Logs = resultRLP.Logs

	result.ContractAddress = resultRLP.ContractAddress
	result.GasUsed = resultRLP.GasUsed
}

func (result *SubstateResult) EncodeRLP(w io.Writer) error {
	return rlp.Encode(w, NewSubstateResultRLP(result))
}

func (result *SubstateResult) DecodeRLP(s *rlp.Stream) error {
	var err error
	var resultRLP SubstateResultRLP

	err = s.Decode(&resultRLP)
	if err != nil {
		return err
	}

	result.SetRLP(&resultRLP)

	return nil
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

func (substate *Substate) SetRLP(substateRLP *SubstateRLP) {
	substate.InputAlloc.SetRLP(substateRLP.InputAlloc)
	substate.OutputAlloc.SetRLP(substateRLP.OutputAlloc)
	substate.Env.SetRLP(substateRLP.Env)
	substate.Message.SetRLP(substateRLP.Message)
	substate.Result.SetRLP(substateRLP.Result)
}

func (substate *Substate) EncodeRLP(w io.Writer) error {
	return rlp.Encode(w, NewSubstateRLP(substate))
}

func (substate *Substate) DecodeRLP(s *rlp.Stream) error {
	var err error
	var substateRLP SubstateRLP

	err = s.Decode(&substateRLP)
	if err != nil {
		return err
	}

	substate.SetRLP(&substateRLP)

	return nil
}
