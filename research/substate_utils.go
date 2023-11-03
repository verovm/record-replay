package research

import (
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"google.golang.org/protobuf/proto"
)

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
	substate.Result = &Substate_Result{
		Status:          rr.Status,
		Bloom:           rr.Bloom.Bytes(),
		ContractAddress: rr.ContractAddress.Bytes(),
		GasUsed:         rr.GasUsed,
	}
	for _, log := range rr.Logs {
		logPb := &Substate_Result_Log{
			Address: log.Address.Bytes(),
			Data:    log.Data,
		}
		for _, topic := range log.Topics {
			logPb.Topics = append(logPb.Topics, topic.Bytes())
		}
		substate.Result.Logs = append(substate.Result.Logs, logPb)
	}
}

func (rr *ResearchReceipt) LoadSubstate(substate *Substate) {
	r := &types.Receipt{}
	re := substate.Result
	r.Status = re.Status
	r.Bloom = types.BytesToBloom(re.Bloom)
	r.ContractAddress = common.BytesToAddress(re.ContractAddress)
	r.GasUsed = re.GasUsed
	for _, relog := range re.Logs {
		log := &types.Log{}
		log.Address = common.BytesToAddress(relog.Address)
		log.Data = relog.Data
		for _, topic := range relog.Topics {
			log.Topics = append(log.Topics, common.BytesToHash(topic))
		}
		r.Logs = append(r.Logs, log)
	}
}
