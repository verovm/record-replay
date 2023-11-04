package research

import (
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"google.golang.org/protobuf/proto"
)

func NewUint64(x uint64) *uint64 {
	y := new(uint64)
	*y = x
	return y
}

func HashToBytes(hash *common.Hash) []byte {
	if hash == nil {
		return nil
	}
	return hash.Bytes()
}

func BytesToHash(b []byte) *common.Hash {
	if b == nil {
		return nil
	}
	hash := common.BytesToHash(b)
	return &hash
}

func AddressToBytes(addr *common.Address) []byte {
	if addr == nil {
		return nil
	}
	return addr.Bytes()
}

func BytesToAddress(b []byte) *common.Address {
	if b == nil {
		return nil
	}
	addr := common.BytesToAddress(b)
	return &addr
}

func BigIntToBytes(x *big.Int) []byte {
	if x == nil {
		return nil
	}
	return x.Bytes()
}

func BytesToBigInt(b []byte) *big.Int {
	if b == nil {
		return nil
	}
	return new(big.Int).SetBytes(b)
}

func BloomToBytes(bloom *types.Bloom) []byte {
	if bloom == nil {
		return nil
	}
	return bloom.Bytes()
}

func BytesToBloom(b []byte) *types.Bloom {
	if b == nil {
		return nil
	}
	bloom := types.BytesToBloom(b)
	return &bloom
}

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

	re.Status = NewUint64(rr.Status)

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

	re.GasUsed = NewUint64(rr.GasUsed)

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
