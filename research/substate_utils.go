package research

import (
	"bytes"
	"math/big"
	"sort"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/holiman/uint256"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/wrapperspb"
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

// HashToBytesValue in research package strictly returns nil if hash is nil
func HashToBytesValue(hash *common.Hash) *wrapperspb.BytesValue {
	if hash == nil {
		return nil
	}
	return wrapperspb.Bytes(hash.Bytes())
}

// BytesValueToHash in research package strictly returns nil if bv is nil
func BytesValueToHash(bv *wrapperspb.BytesValue) *common.Hash {
	if bv == nil {
		return nil
	}
	hash := common.BytesToHash(bv.Value)
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

// AddressToBytesValue in research package strictly returns nil if addr is nil
func AddressToBytesValue(addr *common.Address) *wrapperspb.BytesValue {
	if addr == nil {
		return nil
	}
	return wrapperspb.Bytes(addr.Bytes())
}

// BytesValueToAddress in research package strictly returns nil if bv is nil
func BytesValueToAddress(bv *wrapperspb.BytesValue) *common.Address {
	if bv == nil {
		return nil
	}
	addr := common.BytesToAddress(bv.Value)
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

// Uint256ToBytes in research package strictly returns nil if x is nil
func Uint256ToBytes(x *uint256.Int) []byte {
	if x == nil {
		return nil
	}
	return x.ToBig().Bytes()
}

// BytesToUint256 in research package strictly returns nil if b is nil
func BytesToUint256(b []byte) *uint256.Int {
	if b == nil {
		return nil
	}
	return uint256.MustFromBig(BytesToBigInt(b))
}

// BigIntToBytesValue in research package strictly returns nil if x is nil
func BigIntToBytesValue(x *big.Int) *wrapperspb.BytesValue {
	if x == nil {
		return nil
	}
	return wrapperspb.Bytes(x.Bytes())
}

// BytesValueToBigInt in research package strictly returns nil if bv is nil
func BytesValueToBigInt(b *wrapperspb.BytesValue) *big.Int {
	if b == nil {
		return nil
	}
	return new(big.Int).SetBytes(b.Value)
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

func SortStorage(x []*Substate_Account_StorageEntry) {
	sort.Slice(x, func(i, j int) bool {
		return bytes.Compare(x[i].Key, x[j].Key) < 0
	})
}

func SortAlloc(x []*Substate_AllocEntry) {
	sort.Slice(x, func(i, j int) bool {
		return bytes.Compare(x[i].Address, x[j].Address) < 0
	})
}

// (*Substate).Hashes returns codeHash -> code from unhashed substate
func (x *Substate) HashMap() map[common.Hash][]byte {
	if x == nil {
		return nil
	}

	z := make(map[common.Hash][]byte)

	for _, entry := range x.InputAlloc.Alloc {
		account := entry.Account
		if code := account.GetCode(); code != nil {
			codeHash := CodeHash(code)
			z[codeHash] = code
		}
	}

	for _, entry := range x.OutputAlloc.Alloc {
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

	for _, entry := range x.InputAlloc.Alloc {
		account := entry.Account
		if codeHash := BytesToHash(account.GetCodeHash()); codeHash != nil {
			z[*codeHash] = struct{}{}
		}
	}

	for _, entry := range x.OutputAlloc.Alloc {
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
	y := proto.Clone(x).(*Substate)

	if y == nil {
		return nil
	}

	for _, entry := range y.InputAlloc.Alloc {
		account := entry.Account
		if code := account.GetCode(); code != nil {
			codeHash := CodeHash(code)
			account.Contract = &Substate_Account_CodeHash{
				CodeHash: HashToBytes(&codeHash),
			}
		}
	}

	for _, entry := range y.OutputAlloc.Alloc {
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
	y := proto.Clone(x).(*Substate)

	if y == nil {
		return nil
	}

	for _, entry := range y.InputAlloc.Alloc {
		account := entry.Account
		if codeHash := BytesToHash(account.GetCodeHash()); codeHash != nil {
			account.Contract = &Substate_Account_Code{
				Code: z[*codeHash],
			}
		}
	}

	for _, entry := range y.OutputAlloc.Alloc {
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

	GasUsed uint64
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

	rr.GasUsed = r.GasUsed

	return rr
}

func (rr *ResearchReceipt) SaveSubstate(substate *Substate) {
	re := &Substate_Result{}

	re.Status = proto.Uint64(rr.Status)

	re.Bloom = BloomToBytes(&rr.Bloom)

	for _, log := range rr.Logs {
		relog := &Substate_Result_Log{}
		relog.Address = log.Address.Bytes()
		relog.Data = log.Data
		// Log.Data is required, so it cannot be nil
		if relog.Data == nil {
			relog.Data = []byte{}
		}
		for _, topic := range log.Topics {
			relog.Topics = append(relog.Topics, HashToBytes(&topic))
		}
		re.Logs = append(re.Logs, relog)
	}

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
