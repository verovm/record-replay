package db

import (
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	rr03_research "github.com/ethereum/go-ethereum/cmd/substate-cli/rr03/research"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/rawdb"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/research"
	"github.com/ethereum/go-ethereum/rlp"
	cli "github.com/urfave/cli/v2"
	"google.golang.org/protobuf/proto"
)

var DbRr03ToRr04Command = &cli.Command{
	Action: dbRr03ToRr04,
	Name:   "db-rr0.3-to-rr0.4",
	Aliases: []string{
		"db-rlp2proto",
	},
	Usage: "upgrade old rr0.3 DB layout (RLP) to new rr0.4 DB layout (Protobuf)",
	Flags: []cli.Flag{
		research.WorkersFlag,
		research.BlockSegmentFlag,
		&cli.PathFlag{
			Name:     "old-path",
			Usage:    "Old rr0.3 substate DB path, e.g., rr0.3.substate.ethereum)",
			Required: true,
		},
		&cli.PathFlag{
			Name:     "new-path",
			Usage:    "New rr0.4 substate DB path, e.g., rr0.4.substate.ethereum)",
			Required: true,
		},
		&cli.PathFlag{
			Name:     "blockchain",
			Usage:    "Optional blockchain file from geth export for missing info",
			Required: false,
		},
		core.SkipCheckReplayFlag,
	},
	Description: `
The substate db-rr0.3-to-rr0.4 command upgrade substate encoding from old rr0.3 RLP to
new rr0.4 Protobuf and copy into the new DB of a given block segment.

old-path is old rr0.3 DB layout using RLP for encoding substates.
new-path is new rr0.4 DB layout using Protobuf instead of RLP.

blockchain is optional chain file from the geth export coommand to supplement
tx types missing in rr0.3. If txs do not exist in the blockchain file,
db-upgrade guess tx types based on values of access lists and dynamic gas fee.
`,
	Category: "db",
}

// readBcTxTypes is based on (*AdminAPI).ImportChain
func readBcTxTypes(file string) (map[uint64][]uint8, error) {
	fmt.Printf("Reading blockchain file %s ...\n", file)
	bcTxTypes := make(map[uint64][]uint8)

	// Make sure the can access the file to import
	in, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	defer in.Close()

	var reader io.Reader = in
	if strings.HasSuffix(file, ".gz") {
		if reader, err = gzip.NewReader(reader); err != nil {
			return nil, err
		}
	}

	// Run actual the import in pre-configured batches
	stream := rlp.NewStream(reader, 0)

	// Read files
	for index := 0; ; index++ {
		block := new(types.Block)
		if err := stream.Decode(block); err == io.EOF {
			break
		} else if err != nil {
			return nil, fmt.Errorf("block %d: failed to parse: %v", index, err)
		}
		num64 := block.NumberU64()
		for _, tx := range block.Transactions() {
			bcTxTypes[num64] = append(bcTxTypes[num64], tx.Type())
		}
	}

	return bcTxTypes, nil
}

func dbRr03ToRr04(ctx *cli.Context) error {
	var err error

	core.SkipCheckReplay = ctx.Bool(core.SkipCheckReplayFlag.Name)

	oldPath := ctx.Path("old-path")
	oldBackend, err := rawdb.NewLevelDBDatabase(oldPath, 1024, 100, "srcDB", true)
	if err != nil {
		return fmt.Errorf("substate-cli db-upgrade: error opening %s: %v", oldPath, err)
	}
	oldDB := rr03_research.NewSubstateDB(oldBackend)
	defer oldDB.Close()

	// Create new rr0.4 DB
	newPath := ctx.Path("new-path")
	newBackend, err := rawdb.NewLevelDBDatabase(newPath, 1024, 100, "srcDB", false)
	if err != nil {
		return fmt.Errorf("substate-cli db-upgrade: error creating %s: %v", newPath, err)
	}
	newDB := research.NewSubstateDB(newBackend)
	defer newDB.Close()

	// Read blockchain file and store tx types
	bcPath := ctx.Path("blockchain")
	bcTxTypes := make(map[uint64][]uint8)
	if len(bcPath) > 0 {
		bcTxTypes, err = readBcTxTypes(bcPath)
		if err != nil {
			panic(err)
		}
	}
	// getBcTxType returns nil if value not found in bcTxTypes
	getBcTxType := func(block uint64, tx int) *research.Substate_TxMessage_TxType {
		txTypes := bcTxTypes[block]

		if tx >= len(txTypes) {
			return nil
		}

		switch x := txTypes[tx]; x {
		case types.LegacyTxType:
			return research.Substate_TxMessage_TXTYPE_LEGACY.Enum()
		case types.AccessListTxType:
			return research.Substate_TxMessage_TXTYPE_ACCESSLIST.Enum()
		case types.DynamicFeeTxType:
			return research.Substate_TxMessage_TXTYPE_DYNAMICFEE.Enum()
		default:
			panic(fmt.Errorf("tx type %v is not supported", x))
		}
	}

	upgradeTask := func(block uint64, tx int, s03 *rr03_research.Substate, taskPool *rr03_research.SubstateTaskPool) error {
		// Convert RLP to Substate
		s04 := &research.Substate{}
		s04.InputAlloc = upgradeAlloc(s03.InputAlloc)
		s04.OutputAlloc = upgradeAlloc(s03.OutputAlloc)
		s04.BlockEnv = upgradeBlockEnv(s03.Env)
		s04.TxMessage = upgradeMessage(s03.Message, getBcTxType(block, tx))
		s04.Result = upgradeResult(*s03.Result)

		// Check faithful replay with upgraded substate
		if err := core.CheckReplay(block, tx, s04); err != nil {
			return err
		}

		newDB.PutSubstate(block, tx, s04)

		return nil
	}

	taskPool := &rr03_research.SubstateTaskPool{
		Name:     "substate-cli db-upgrade",
		TaskFunc: upgradeTask,
		Config:   rr03_research.NewSubstateTaskConfigCli(ctx),

		DB: oldDB,
	}

	segment, err := rr03_research.ParseBlockSegment(ctx.String(research.BlockSegmentFlag.Name))
	if err != nil {
		return fmt.Errorf("substate-cli db-upgrade: error parsing block segment: %s", err)
	}

	err = taskPool.ExecuteSegment(segment)

	return err
}

func upgradeAlloc(alloc03 rr03_research.SubstateAlloc) *research.Substate_Alloc {
	alloc04 := &research.Substate_Alloc{}
	for addr, account03 := range alloc03 {
		account04 := &research.Substate_Account{
			Nonce:    proto.Uint64(account03.Nonce),
			Balance:  research.BigIntToBytes(account03.Balance),
			Contract: &research.Substate_Account_Code{Code: account03.Code},
		}
		for key, value := range account03.Storage {
			entry := &research.Substate_Account_StorageEntry{
				Key:   research.HashToBytes(&key),
				Value: research.HashToBytes(&value),
			}
			account04.Storage = append(account04.Storage, entry)
		}
		research.SortStorage(account04.Storage)
		alloc04.Alloc = append(alloc04.Alloc, &research.Substate_AllocEntry{
			Address: research.AddressToBytes(&addr),
			Account: account04,
		})
	}
	research.SortAlloc(alloc04.Alloc)
	return alloc04
}

func upgradeBlockEnv(b *rr03_research.SubstateEnv) *research.Substate_BlockEnv {
	e := &research.Substate_BlockEnv{}

	e.Coinbase = research.AddressToBytes(&b.Coinbase)

	e.Difficulty = research.BigIntToBytes(b.Difficulty)

	e.GasLimit = proto.Uint64(b.GasLimit)

	e.Number = proto.Uint64(b.Number)

	e.Timestamp = proto.Uint64(b.Timestamp)

	if b.BlockHashes != nil {
		for num64, blockHash := range b.BlockHashes {
			entry := &research.Substate_BlockEnv_BlockHashEntry{
				Key:   proto.Uint64(num64),
				Value: research.HashToBytes(&blockHash),
			}
			e.BlockHashes = append(e.BlockHashes, entry)
		}
		sort.Slice(e.BlockHashes, func(i, j int) bool {
			return *e.BlockHashes[i].Key < *e.BlockHashes[j].Key
		})
	}

	e.BaseFee = research.BigIntToBytesValue(b.BaseFee)

	return e
}

func guessTxType(msg *rr03_research.SubstateMessage) *research.Substate_TxMessage_TxType {
	txType := research.Substate_TxMessage_TXTYPE_LEGACY.Enum()
	if len(msg.AccessList) > 0 {
		txType = research.Substate_TxMessage_TXTYPE_ACCESSLIST.Enum()
	}
	if msg.GasFeeCap.Cmp(msg.GasPrice) != 0 && msg.GasTipCap.Cmp(msg.GasPrice) != 0 {
		txType = research.Substate_TxMessage_TXTYPE_DYNAMICFEE.Enum()
	}
	return txType
}

func upgradeMessage(m *rr03_research.SubstateMessage, txType *research.Substate_TxMessage_TxType) *research.Substate_TxMessage {
	t := &research.Substate_TxMessage{}

	t.Nonce = proto.Uint64(m.Nonce)

	t.GasPrice = research.BigIntToBytes(m.GasPrice)

	t.Gas = proto.Uint64(m.Gas)

	t.From = research.AddressToBytes(&m.From)

	t.To = research.AddressToBytesValue(m.To)

	t.Value = research.BigIntToBytes(m.Value)

	t.Input = &research.Substate_TxMessage_Data{Data: m.Data}

	if txType == nil {
		t.TxType = guessTxType(m)
	} else {
		t.TxType = txType
	}

	switch *t.TxType {
	case research.Substate_TxMessage_TXTYPE_ACCESSLIST,
		research.Substate_TxMessage_TXTYPE_DYNAMICFEE:
		t.AccessList = make([]*research.Substate_TxMessage_AccessListEntry, 0, len(m.AccessList))
		for _, tuple := range m.AccessList {
			entry := &research.Substate_TxMessage_AccessListEntry{}
			entry.Address = research.AddressToBytes(&tuple.Address)
			for _, key := range tuple.StorageKeys {
				entry.StorageKeys = append(entry.StorageKeys, research.HashToBytes(&key))
			}
			t.AccessList = append(t.AccessList, entry)
		}
	}

	switch *t.TxType {
	case research.Substate_TxMessage_TXTYPE_DYNAMICFEE:
		t.GasFeeCap = research.BigIntToBytesValue(m.GasFeeCap)
		t.GasTipCap = research.BigIntToBytesValue(m.GasTipCap)
	}

	return t
}

func upgradeResult(rr rr03_research.SubstateResult) *research.Substate_Result {
	re := &research.Substate_Result{}

	re.Status = proto.Uint64(rr.Status)

	re.Bloom = research.BloomToBytes(&rr.Bloom)

	for _, log := range rr.Logs {
		relog := &research.Substate_Result_Log{}
		relog.Address = log.Address.Bytes()
		relog.Data = log.Data
		// Log.Data is required, so it cannot be nil
		if relog.Data == nil {
			relog.Data = []byte{}
		}
		for _, topic := range log.Topics {
			relog.Topics = append(relog.Topics, research.HashToBytes(&topic))
		}
		re.Logs = append(re.Logs, relog)
	}

	re.GasUsed = proto.Uint64(rr.GasUsed)

	return re
}
