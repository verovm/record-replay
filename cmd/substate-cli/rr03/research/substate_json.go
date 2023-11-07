package research

import (
	"encoding/json"
	"math/big"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/common/math"
	"github.com/ethereum/go-ethereum/core/types"

	"github.com/ethereum/go-ethereum/common"
)

// SubstateAccountJSON is modification of core.GenesisAccount
type SubstateAccountJSON struct {
	Code    hexutil.Bytes               `json:"code,omitempty"`
	Storage map[common.Hash]common.Hash `json:"storage,omitempty"`
	Balance *math.HexOrDecimal256       `json:"balance" gencodec:"required"`
	Nonce   math.HexOrDecimal64         `json:"nonce,omitempty"`
}

func NewSubstateAccountJSON(sa *SubstateAccount) *SubstateAccountJSON {
	return &SubstateAccountJSON{
		Nonce:   math.HexOrDecimal64(sa.Nonce),
		Balance: (*math.HexOrDecimal256)(sa.Balance),
		Storage: sa.Storage,
		Code:    sa.Code,
	}
}

func (sa *SubstateAccount) SetJSON(saJSON *SubstateAccountJSON) {
	sa.Nonce = uint64(saJSON.Nonce)
	sa.Balance = (*big.Int)(saJSON.Balance)
	sa.Storage = make(map[common.Hash]common.Hash)
	if saJSON.Storage != nil {
		sa.Storage = saJSON.Storage
	}
	sa.Code = saJSON.Code
}

func (sa SubstateAccount) MarshalJSON() ([]byte, error) {
	return json.Marshal(NewSubstateAccountJSON(&sa))
}

func (sa *SubstateAccount) UnmarshalJSON(b []byte) error {
	var err error
	var saJSON SubstateAccountJSON

	err = json.Unmarshal(b, &saJSON)
	if err != nil {
		return err
	}

	sa.SetJSON(&saJSON)

	return nil
}

type SubstateAllocJSON map[common.Address]*SubstateAccountJSON

func NewSubstateAllocJSON(alloc SubstateAlloc) SubstateAllocJSON {
	allocJSON := make(SubstateAllocJSON)
	for addr, account := range alloc {
		allocJSON[addr] = NewSubstateAccountJSON(account)
	}

	return allocJSON
}

func (alloc *SubstateAlloc) SetJSON(allocJSON SubstateAllocJSON) {
	*alloc = make(SubstateAlloc)
	for addr, saJSON := range allocJSON {
		var sa SubstateAccount

		sa.Nonce = uint64(saJSON.Nonce)
		sa.Balance = (*big.Int)(saJSON.Balance)
		sa.Storage = make(map[common.Hash]common.Hash)
		if saJSON.Storage != nil {
			sa.Storage = saJSON.Storage
		}
		sa.Code = saJSON.Code

		(*alloc)[addr] = &sa
	}
}

func (alloc SubstateAlloc) MarshalJSON() ([]byte, error) {
	return json.Marshal(NewSubstateAllocJSON(alloc))
}

func (alloc *SubstateAlloc) UnmarshalJSON(b []byte) error {
	var err error
	var allocJSON SubstateAllocJSON

	err = json.Unmarshal(b, &allocJSON)
	if err != nil {
		return err
	}

	alloc.SetJSON(allocJSON)

	return nil
}

// SubstateEnvJSON is modification of t8ntool.stEnv
type SubstateEnvJSON struct {
	Coinbase    common.Address                      `json:"coinbase" gencodec:"required"`
	Difficulty  *math.HexOrDecimal256               `json:"difficulty" gencodec:"required"`
	GasLimit    math.HexOrDecimal64                 `json:"gasLimit" gencodec:"required"`
	Number      math.HexOrDecimal64                 `json:"number" gencodec:"required"`
	Timestamp   math.HexOrDecimal64                 `json:"timestamp" gencodec:"required"`
	BlockHashes map[math.HexOrDecimal64]common.Hash `json:"blockHashes,omitempty"`

	BaseFee *math.HexOrDecimal256 `json:"baseFee,omitempty"`
}

func NewSubstateEnvJSON(env *SubstateEnv) *SubstateEnvJSON {
	var envJSON SubstateEnvJSON

	envJSON.Coinbase = env.Coinbase
	envJSON.Difficulty = (*math.HexOrDecimal256)(env.Difficulty)
	envJSON.GasLimit = math.HexOrDecimal64(env.GasLimit)
	envJSON.Number = math.HexOrDecimal64(env.Number)
	envJSON.Timestamp = math.HexOrDecimal64(env.Timestamp)
	envJSON.BlockHashes = make(map[math.HexOrDecimal64]common.Hash)
	if env.BlockHashes != nil {
		for num64, bhash := range env.BlockHashes {
			envJSON.BlockHashes[math.HexOrDecimal64(num64)] = bhash
		}
	}

	envJSON.BaseFee = (*math.HexOrDecimal256)(env.BaseFee)

	return &envJSON
}

func (env *SubstateEnv) SetJSON(envJSON *SubstateEnvJSON) {
	env.Coinbase = envJSON.Coinbase
	env.Difficulty = (*big.Int)(envJSON.Difficulty)
	env.GasLimit = uint64(envJSON.GasLimit)
	env.Number = uint64(envJSON.Number)
	env.Timestamp = uint64(envJSON.Timestamp)
	env.BlockHashes = make(map[uint64]common.Hash)
	if envJSON.BlockHashes != nil {
		for num64, bhash := range envJSON.BlockHashes {
			env.BlockHashes[uint64(num64)] = bhash
		}
	}

	env.BaseFee = (*big.Int)(envJSON.BaseFee)
	if env.BaseFee.Cmp(big.NewInt(0)) == 0 {
		env.BaseFee = nil
	}
}

func (env SubstateEnv) MarshalJSON() ([]byte, error) {
	return json.Marshal(NewSubstateEnvJSON(&env))
}

func (env *SubstateEnv) UnmarshalJSON(b []byte) error {
	var err error
	var envJSON SubstateEnvJSON

	err = json.Unmarshal(b, &envJSON)
	if err != nil {
		return err
	}

	env.SetJSON(&envJSON)

	return nil
}

// SubstateMessageJSON is modification of types.msgdata
type SubstateMessageJSON struct {
	Nonce      math.HexOrDecimal64   `json:"nonce" gencodec:"required"`
	CheckNonce bool                  `json:"checkNonce" gencodec:"required"`
	GasPrice   *math.HexOrDecimal256 `json:"gasPrice" gencodec:"required"`
	Gas        math.HexOrDecimal64   `json:"gas" gencodec:"required"`

	From  common.Address        `json:"from"`
	To    *common.Address       `json:"to"` // nil means contract creation
	Value *math.HexOrDecimal256 `json:"value" gencodec:"required"`
	Data  hexutil.Bytes         `json:"input" gencodec:"required"`

	AccessList types.AccessList `json:"accessList,omitempty"`

	GasFeeCap *math.HexOrDecimal256 `json:"gasFeeCap,omitempty"`
	GasTipCap *math.HexOrDecimal256 `json:"gasTipCap,omitempty"`
}

func NewSubstateMessageJSON(msg *SubstateMessage) *SubstateMessageJSON {
	var msgJSON SubstateMessageJSON

	msgJSON.Nonce = math.HexOrDecimal64(msg.Nonce)
	msgJSON.CheckNonce = msg.CheckNonce
	msgJSON.GasPrice = (*math.HexOrDecimal256)(msg.GasPrice)
	msgJSON.Gas = math.HexOrDecimal64(msg.Gas)

	msgJSON.From = msg.From
	msgJSON.To = msg.To
	msgJSON.Value = (*math.HexOrDecimal256)(msg.Value)
	msgJSON.Data = hexutil.Bytes(msg.Data)

	msgJSON.AccessList = msg.AccessList

	msgJSON.GasFeeCap = (*math.HexOrDecimal256)(msg.GasFeeCap)
	msgJSON.GasTipCap = (*math.HexOrDecimal256)(msg.GasTipCap)

	return &msgJSON
}

func (msg *SubstateMessage) SetJSON(msgJSON *SubstateMessageJSON) {
	msg.Nonce = uint64(msgJSON.Nonce)
	msg.CheckNonce = msgJSON.CheckNonce
	msg.GasPrice = (*big.Int)(msgJSON.GasPrice)
	msg.Gas = uint64(msgJSON.Gas)

	msg.From = msgJSON.From
	msg.To = msgJSON.To
	msg.Value = (*big.Int)(msgJSON.Value)
	msg.Data = []byte(msgJSON.Data)

	msg.AccessList = msgJSON.AccessList

	msg.GasFeeCap = (*big.Int)(msgJSON.GasFeeCap)
	if msg.GasFeeCap.Cmp(big.NewInt(0)) == 0 {
		msg.GasFeeCap = msg.GasPrice
	}
	msg.GasTipCap = (*big.Int)(msgJSON.GasTipCap)
	if msg.GasTipCap.Cmp(big.NewInt(0)) == 0 {
		msg.GasTipCap = msg.GasPrice
	}
}

func (msg SubstateMessage) MarshalJSON() ([]byte, error) {
	return json.Marshal(NewSubstateMessageJSON(&msg))
}

func (msg *SubstateMessage) UnmarshalJSON(b []byte) error {
	var err error
	var msgJSON SubstateMessageJSON

	err = json.Unmarshal(b, &msgJSON)
	if err != nil {
		return err
	}

	msg.SetJSON(&msgJSON)

	return nil
}

type SubstateResultJSON struct {
	Status math.HexOrDecimal64 `json:"status"`
	Bloom  types.Bloom         `json:"logsBloom"`
	Logs   []*types.Log        `json:"logs"`

	ContractAddress common.Address      `json:"contractAddress"`
	GasUsed         math.HexOrDecimal64 `json:"gasUsed" gencodec:"required"`
}

func NewSubstateResultJSON(result *SubstateResult) *SubstateResultJSON {
	var resultJSON SubstateResultJSON

	resultJSON.Status = math.HexOrDecimal64(result.Status)
	resultJSON.Bloom = result.Bloom
	resultJSON.Logs = result.Logs

	resultJSON.ContractAddress = result.ContractAddress
	resultJSON.GasUsed = math.HexOrDecimal64(result.GasUsed)

	return &resultJSON
}

func (result *SubstateResult) SetJSON(resultJSON *SubstateResultJSON) {
	result.Status = uint64(resultJSON.Status)
	result.Bloom = resultJSON.Bloom
	result.Logs = resultJSON.Logs

	result.ContractAddress = resultJSON.ContractAddress
	result.GasUsed = uint64(resultJSON.GasUsed)
}

func (result SubstateResult) MarshalJSON() ([]byte, error) {
	return json.Marshal(NewSubstateResultJSON(&result))
}

func (result *SubstateResult) UnmarshalJSON(b []byte) error {
	var err error
	var resultJSON SubstateResultJSON

	err = json.Unmarshal(b, &resultJSON)
	if err != nil {
		return err
	}

	result.SetJSON(&resultJSON)

	return nil
}

type SubstateJSON struct {
	InputAlloc  SubstateAllocJSON    `json:"inputAlloc"`
	OutputAlloc SubstateAllocJSON    `json:"outputAlloc"`
	Env         *SubstateEnvJSON     `json:"env"`
	Message     *SubstateMessageJSON `json:"message"`
	Result      *SubstateResultJSON  `json:"result"`
}

func NewSubstateJSON(substate *Substate) *SubstateJSON {
	var substateJSON SubstateJSON

	substateJSON.InputAlloc = NewSubstateAllocJSON(substate.InputAlloc)
	substateJSON.OutputAlloc = NewSubstateAllocJSON(substate.OutputAlloc)
	substateJSON.Env = NewSubstateEnvJSON(substate.Env)
	substateJSON.Message = NewSubstateMessageJSON(substate.Message)
	substateJSON.Result = NewSubstateResultJSON(substate.Result)

	return &substateJSON
}

func (substate *Substate) SetJSON(substateJSON *SubstateJSON) {
	substate.InputAlloc.SetJSON(substateJSON.InputAlloc)
	substate.OutputAlloc.SetJSON(substateJSON.OutputAlloc)
	substate.Env.SetJSON(substateJSON.Env)
	substate.Message.SetJSON(substateJSON.Message)
	substate.Result.SetJSON(substateJSON.Result)
}

func (substate Substate) MarshalJSON() ([]byte, error) {
	return json.Marshal(NewSubstateJSON(&substate))
}

func (substate *Substate) UnmarshalJSON(b []byte) error {
	var err error
	var substateJSON SubstateJSON

	err = json.Unmarshal(b, &substateJSON)
	if err != nil {
		return err
	}

	substate.SetJSON(&substateJSON)

	return nil
}
