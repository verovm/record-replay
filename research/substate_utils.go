package research

import (
	"bytes"
	"math/big"
	"sort"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/golang/protobuf/proto"
)

// HashToBytes in research package strictly returns nil if hash is nil
func HashToBytes(hash *common.Hash) []byte {
	if hash == nil {
		return nil
	}
	return hash.Bytes()
}

// BytesToHash in research package strictly returns nil if b is nil
func BytesToHash(b []byte) *common.Hash {
	if b == nil {
		return nil
	}
	hash := common.BytesToHash(b)
	return &hash
}

// AddressToBytes in research package strictly returns nil if addr is nil
func AddressToBytes(addr *common.Address) []byte {
	if addr == nil {
		return nil
	}
	return addr.Bytes()
}

// BytesToAddress in research package strictly returns nil if b is nil
func BytesToAddress(b []byte) *common.Address {
	if b == nil {
		return nil
	}
	addr := common.BytesToAddress(b)
	return &addr
}

// BigIntToBytes in research package strictly returns nil if x is nil
func BigIntToBytes(x *big.Int) []byte {
	if x == nil {
		return nil
	}
	return x.Bytes()
}

// BytesToBigInt in research package strictly returns nil if b is nil
func BytesToBigInt(b []byte) *big.Int {
	if b == nil {
		return nil
	}
	return new(big.Int).SetBytes(b)
}

// BloomToBytes in research package strictly returns nil if bloom is nil
func BloomToBytes(bloom *types.Bloom) []byte {
	if bloom == nil {
		return nil
	}
	return bloom.Bytes()
}

// BytesToBloom in research package strictly returns nil if b is nil
func BytesToBloom(b []byte) *types.Bloom {
	if b == nil {
		return nil
	}
	bloom := types.BytesToBloom(b)
	return &bloom
}

// (*Substate_Account).Copy returns a deep copy
func (x *Substate_Account) Copy() *Substate_Account {
	if x == nil {
		return nil
	}

	pb, err := proto.Marshal(x)
	if err != nil {
		panic(err)
	}

	y := &Substate_Account{}
	err = proto.Unmarshal(pb, y)
	if err != nil {
		panic(err)
	}

	return y
}

func EqualAccount(x, y *Substate_Account) bool {
	if x == y {
		return true
	}

	if (x == nil || y == nil) && x != y {
		return false
	}

	eq := x.Nonce == y.Nonce &&
		bytes.Equal(x.Balance, y.Balance) &&
		bytes.Equal(x.GetCode(), y.GetCode()) &&
		bytes.Equal(x.GetCodeHash(), y.GetCodeHash()) &&
		len(x.Storage) == len(y.Storage)
	if !eq {
		return false
	}

	if len(x.Storage) == 0 {
		return true
	}

	xs := append([]*Substate_Account_StorageEntry{}, x.Storage...)
	sort.Slice(xs, func(i, j int) bool {
		return bytes.Compare(xs[i].Key, xs[j].Key) < 0
	})

	ys := append([]*Substate_Account_StorageEntry{}, y.Storage...)
	sort.Slice(ys, func(i, j int) bool {
		return bytes.Compare(ys[i].Key, ys[j].Key) < 0
	})

	for i, xp := range xs {
		yp := ys[i]
		eq := bytes.Equal(xp.Key, yp.Key) && bytes.Equal(xp.Value, yp.Value)
		if !eq {
			return false
		}
	}

	return true
}

// EqualAlloc returns false when either x or y is nil and the other is 0-length slice
func EqualAlloc(x, y []*Substate_AllocEntry) bool {
	if x == nil && y == nil {
		return true
	}

	if (x == nil && y != nil) || (x != nil && y == nil) || len(x) != len(y) {
		return false
	}

	xalloc := append([]*Substate_AllocEntry{}, x...)
	sort.Slice(xalloc, func(i, j int) bool {
		return bytes.Compare(xalloc[i].Address, xalloc[j].Address) < 0
	})

	yalloc := append([]*Substate_AllocEntry{}, y...)
	sort.Slice(yalloc, func(i, j int) bool {
		return bytes.Compare(yalloc[i].Address, yalloc[j].Address) < 0
	})

	for i, xe := range xalloc {
		ye := yalloc[i]
		eq := bytes.Equal(xe.Address, ye.Address) && EqualAccount(xe.Account, ye.Account)
		if !eq {
			return false
		}
	}

	return true
}

// (*Substate).Copy returns a deep copy of the substate
func (x *Substate) Copy() *Substate {
	if x == nil {
		return nil
	}

	pb, err := proto.Marshal(x)
	if err != nil {
		panic(err)
	}

	y := &Substate{}
	err = proto.Unmarshal(pb, y)
	if err != nil {
		panic(err)
	}

	return y
}

// (*Substate).Hashes returns codeHash -> code from unhashed substate
func (x *Substate) HashMap() map[common.Hash][]byte {
	if x == nil {
		return nil
	}

	z := make(map[common.Hash][]byte)

	for _, entry := range x.InputAlloc {
		account := entry.Account
		if code := account.GetCode(); code != nil {
			codeHash := CodeHash(code)
			z[codeHash] = code
		}
	}

	for _, entry := range x.OutputAlloc {
		account := entry.Account
		if code := account.GetCode(); code != nil {
			codeHash := CodeHash(code)
			z[codeHash] = code
		}
	}

	if x.TxMessage.To == nil {
		if code := x.TxMessage.GetData(); code != nil {
			codeHash := CodeHash(code)
			z[codeHash] = code
		}
	}

	return z
}

// (*Substate).HashKeys returns a set of code hasehs from hashed substate
func (x *Substate) HashKeys() map[common.Hash]struct{} {
	if x == nil {
		return nil
	}

	z := make(map[common.Hash]struct{})

	for _, entry := range x.InputAlloc {
		account := entry.Account
		if codeHash := BytesToHash(account.GetCodeHash()); codeHash != nil {
			z[*codeHash] = struct{}{}
		}
	}

	for _, entry := range x.OutputAlloc {
		account := entry.Account
		if codeHash := BytesToHash(account.GetCodeHash()); codeHash != nil {
			z[*codeHash] = struct{}{}
		}
	}

	if codeHash := BytesToHash(x.TxMessage.GetInitCodeHash()); codeHash != nil {
		z[*codeHash] = struct{}{}
	}

	return z
}

// (*Substate).HashedCopy returns a copy of substate with code hashes in accounts and message.
func (x *Substate) HashedCopy() *Substate {
	y := x.Copy()

	if y == nil {
		return nil
	}

	for _, entry := range y.InputAlloc {
		account := entry.Account
		if code := account.GetCode(); code != nil {
			codeHash := CodeHash(code)
			account.Contract = &Substate_Account_CodeHash{
				CodeHash: HashToBytes(&codeHash),
			}
		}
	}

	for _, entry := range y.OutputAlloc {
		account := entry.Account
		if code := account.GetCode(); code != nil {
			codeHash := CodeHash(code)
			account.Contract = &Substate_Account_CodeHash{
				CodeHash: HashToBytes(&codeHash),
			}
		}
	}

	if y.TxMessage.To == nil {
		if code := y.TxMessage.GetData(); code != nil {
			codeHash := CodeHash(code)
			y.TxMessage.Input = &Substate_TxMessage_InitCodeHash{
				InitCodeHash: HashToBytes(&codeHash),
			}
		}
	}

	return y
}

// (*Substate).UnhashedCopy returns a copy substate with code and init code.
// z is codeHash -> code mappings e.g., return value of from x.HashMap() before hashed
func (x *Substate) UnhashedCopy(z map[common.Hash][]byte) *Substate {
	y := x.Copy()

	if y == nil {
		return nil
	}

	for _, entry := range y.InputAlloc {
		account := entry.Account
		if codeHash := BytesToHash(account.GetCodeHash()); codeHash != nil {
			account.Contract = &Substate_Account_Code{
				Code: z[*codeHash],
			}
		}
	}

	for _, entry := range y.OutputAlloc {
		account := entry.Account
		if codeHash := BytesToHash(account.GetCodeHash()); codeHash != nil {
			account.Contract = &Substate_Account_Code{
				Code: z[*codeHash],
			}
		}
	}

	if codeHash := BytesToHash(y.TxMessage.GetInitCodeHash()); codeHash != nil {
		y.TxMessage.Input = &Substate_TxMessage_Data{
			Data: z[*codeHash],
		}
	}

	return y
}

// ResearchReceipt is a substate of types.Receipt
type ResearchReceipt struct {
	Status uint64
	Bloom  types.Bloom
	Logs   []*types.Log

	ContractAddress common.Address
	GasUsed         uint64
}

func NewResearchReceipt(r *types.Receipt) *ResearchReceipt {
	rr := &ResearchReceipt{}

	rr.Status = r.Status
	rr.Bloom = r.Bloom
	for _, log := range r.Logs {
		rrlog := &types.Log{
			Address: log.Address,
			Topics:  log.Topics,
			Data:    log.Data,
		}
		rr.Logs = append(rr.Logs, rrlog)
	}

	rr.ContractAddress = r.ContractAddress
	rr.GasUsed = r.GasUsed

	return rr
}

func (rr *ResearchReceipt) SaveSubstate(substate *Substate) {
	re := &Substate_Result{}

	re.Status = proto.Uint64(rr.Status)

	re.Bloom = BloomToBytes(&rr.Bloom)

	for _, log := range rr.Logs {
		relog := &Substate_Result_Log{
			Address: log.Address.Bytes(),
			Data:    log.Data,
		}
		for _, topic := range log.Topics {
			relog.Topics = append(relog.Topics, HashToBytes(&topic))
		}
		substate.Result.Logs = append(substate.Result.Logs, relog)
	}

	re.ContractAddress = AddressToBytes(&rr.ContractAddress)

	re.GasUsed = proto.Uint64(rr.GasUsed)

	substate.Result = re
}

func (rr *ResearchReceipt) LoadSubstate(substate *Substate) {
	re := substate.Result

	rr.Status = *re.Status

	rr.Bloom = *BytesToBloom(re.Bloom)

	if re.Logs != nil {
		for _, relog := range re.Logs {
			log := &types.Log{}
			log.Address = *BytesToAddress(relog.Address)
			log.Data = relog.Data
			for _, topic := range relog.Topics {
				log.Topics = append(log.Topics, *BytesToHash(topic))
			}
			rr.Logs = append(rr.Logs, log)
		}
	}

	rr.ContractAddress = *BytesToAddress(re.ContractAddress)

	rr.GasUsed = *re.GasUsed
}

func EqualResult(x, y *Substate_Result) bool {
	if x == y {
		return true
	}

	if (x == nil || y != nil) || (x != nil && y == nil) {
		return false
	}

	eq := *x.Status == *y.Status ||
		bytes.Equal(x.Bloom, y.Bloom) ||
		len(x.Logs) == len(y.Logs) ||
		bytes.Equal(x.ContractAddress, y.ContractAddress) ||
		*x.GasUsed == *y.GasUsed
	if !eq {
		return false
	}

	for i, xl := range x.Logs {
		yl := y.Logs[i]
		eq = bytes.Equal(xl.Address, yl.Address) ||
			len(xl.Topics) == len(yl.Topics) ||
			bytes.Equal(xl.Data, yl.Data)
		if !eq {
			return false
		}
		for j, xt := range xl.Topics {
			yt := yl.Topics[j]
			if !bytes.Equal(xt, yt) {
				return false
			}
		}
	}

	return true
}
