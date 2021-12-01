package research

import (
	"fmt"
	"strconv"
	"time"

	cli "gopkg.in/urfave/cli.v1"

	// import go-ethereum/research

	"github.com/ethereum/go-ethereum/research"
)

type SubstateSize struct {
	TotalSize       int64
	InputAllocSize  int64
	OutputAllocSize int64
	EnvSize         int64
	MessageSize     int64
	ResultSize      int64
}

func NewSubstateSize(substate *research.Substate) *SubstateSize {
	substateSize := SubstateSize{}

	// input accounts
	for _, account := range substate.InputAlloc {
		s := int64(0)
		// address, nonce, balance
		s += (20 + 8 + 32)
		// storage keys and values
		s += int64((32 + 32) * len(account.Storage))
		// code
		s += int64(len(account.Code))

		substateSize.InputAllocSize = s
	}

	// output accounts
	for _, account := range substate.OutputAlloc {
		s := int64(0)
		// address, nonce, balance
		s += (20 + 8 + 32)
		// storage keys and values
		s += int64((32 + 32) * len(account.Storage))
		// code
		s += int64(len(account.Code))

		substateSize.OutputAllocSize = s
	}

	// env
	{
		env := substate.Env
		s := int64(0)
		// coinbase, difficulty, gasLimit, number, timestamp
		s += (20 + 32 + 8 + 8 + 8)
		// block numbers and hashes
		s += int64((8 + 32) * len(env.BlockHashes))

		substateSize.EnvSize = s
	}

	// message
	{
		message := substate.Message
		s := int64(0)
		// nonce, checkNonce, gasPrice, gas, from
		s += (8 + 1 + 32 + 8 + 20)
		// to
		if message.To != nil {
			s += 20
		}
		// value
		s += 32
		// data
		s += int64(len(message.Data))

		substateSize.MessageSize = s
	}

	// result
	{
		result := substate.Result
		s := int64(0)
		// status, bloom
		s += (8 + 256)
		// logs
		for _, log := range result.Logs {
			// address
			s += 20
			// topics
			s += int64(32 * len(log.Topics))
			// data
			s += int64(len(log.Data))
		}
		// contractAddress, gasUsed
		s += (20 + 8)

		substateSize.ResultSize = s
	}

	substateSize.TotalSize = (substateSize.InputAllocSize +
		substateSize.OutputAllocSize +
		substateSize.EnvSize +
		substateSize.MessageSize +
		substateSize.ResultSize)

	return &substateSize
}

func NewSubstateSizeWithCodeHash(substate *research.Substate) *SubstateSize {
	substateSize := SubstateSize{}

	// input accounts
	for _, account := range substate.InputAlloc {
		s := int64(0)
		// address, nonce, balance
		s += (20 + 8 + 32)
		// storage keys and values
		s += int64((32 + 32) * len(account.Storage))
		// code hash
		s += 32

		substateSize.InputAllocSize = s
	}

	// output accounts
	for _, account := range substate.OutputAlloc {
		s := int64(0)
		// address, nonce, balance
		s += (20 + 8 + 32)
		// storage keys and values
		s += int64((32 + 32) * len(account.Storage))
		// code hash
		s += 32

		substateSize.OutputAllocSize = s
	}

	// env
	{
		env := substate.Env
		s := int64(0)
		// coinbase, difficulty, gasLimit, number, timestamp
		s += (20 + 32 + 8 + 8 + 8)
		// block numbers and hashes
		s += int64((8 + 32) * len(env.BlockHashes))

		substateSize.EnvSize = s
	}

	// message
	{
		message := substate.Message
		s := int64(0)
		// nonce, checkNonce, gasPrice, gas, from
		s += (8 + 1 + 32 + 8 + 20)
		// to
		if message.To != nil {
			s += 20
		}
		// value
		s += 32
		if message.To == nil {
			// data
			s += int64(len(message.Data))
		} else {
			// init code hash
			s += 32
		}

		substateSize.MessageSize = s
	}

	// result
	{
		result := substate.Result
		s := int64(0)
		// status, bloom
		s += (8 + 256)
		// logs
		for _, log := range result.Logs {
			// address
			s += 20
			// topics
			s += int64(32 * len(log.Topics))
			// data
			s += int64(len(log.Data))
		}
		// contractAddress, gasUsed
		s += (20 + 8)

		substateSize.ResultSize = s
	}
	substateSize.TotalSize = (substateSize.InputAllocSize +
		substateSize.OutputAllocSize +
		substateSize.EnvSize +
		substateSize.MessageSize +
		substateSize.ResultSize)

	return &substateSize
}

func (x *SubstateSize) Add(y *SubstateSize) *SubstateSize {
	z := SubstateSize{}

	z.TotalSize = x.TotalSize + y.TotalSize
	z.InputAllocSize = x.InputAllocSize + y.InputAllocSize
	z.OutputAllocSize = x.OutputAllocSize + y.OutputAllocSize
	z.EnvSize = x.EnvSize + y.EnvSize
	z.MessageSize = x.MessageSize + y.MessageSize
	z.ResultSize = x.ResultSize + y.ResultSize

	return &z
}

func SizeSubstate(ctx *cli.Context) error {
	if len(ctx.Args()) != 2 {
		return fmt.Errorf("stage1-substate: inspect-substate command requires exactly 2 arguments")
	}

	start := time.Now()

	first, ferr := strconv.ParseInt(ctx.Args().Get(0), 10, 64)
	last, lerr := strconv.ParseInt(ctx.Args().Get(1), 10, 64)
	if ferr != nil || lerr != nil {
		return fmt.Errorf("stage1-substate: InspectSubstate: error in parsing parameters: block number not an integer")
	}
	if first < 0 || last < 0 {
		return fmt.Errorf("stage1-substate: InspectSubstate: error: block number must be greater than 0")
	}
	if first > last {
		return fmt.Errorf("stage1-substate: InspectSubstate: error: first block has larger number than last block")
	}

	defer func() {
		duration := time.Since(start) + 1*time.Nanosecond
		fmt.Printf("stage1-substate: InspectSubstate done in %v\n", duration.Round(1*time.Millisecond))
	}()

	research.OpenSubstateDBReadOnly()
	defer research.CloseSubstateDB()

	ts := &SubstateSize{}
	tsch := &SubstateSize{}
	for block := uint64(first); block <= uint64(last); block++ {
		if block%10000 == 0 {
			duration := time.Since(start) + 1*time.Nanosecond
			fmt.Printf("stage1-substate: elapsed time: %v, number = %v\n", duration.Round(1*time.Millisecond), block)
		}
		for tx := 0; ; tx++ {
			if !research.HasSubstate(block, tx) {
				break
			}

			substate := research.GetSubstate(block, tx)
			ts = ts.Add(NewSubstateSize(substate))
			tsch = tsch.Add(NewSubstateSizeWithCodeHash(substate))
		}
	}
	fmt.Printf("total substate size: %v\nInputAlloc: %v\nOutputAlloc: %v\nEnv: %v\nMessage: %v\nResult: %v\n\n",
		ts.TotalSize, ts.InputAllocSize, ts.OutputAllocSize, ts.EnvSize, ts.MessageSize, ts.ResultSize)
	fmt.Printf("total substate size (replaced code with code hash): %v\nInputAlloc: %v\nOutputAlloc: %v\nEnv: %v\nMessage: %v\nResult: %v\n\n",
		tsch.TotalSize, tsch.InputAllocSize, tsch.OutputAllocSize, tsch.EnvSize, tsch.MessageSize, tsch.ResultSize)

	return nil
}
