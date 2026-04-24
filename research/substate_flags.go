package research

import (
	"bufio"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"

	cli "github.com/urfave/cli/v2"
)

var (
	SubstateDirFlag = &cli.PathFlag{
		Name: "substatedir",
		Aliases: []string{
			"substate-db",
		},
		Usage: "Data directory for substate recorder/replayer",
		Value: "substate.ethereum",
	}
	substateDir      = SubstateDirFlag.Value
	staticSubstateDB *SubstateDB

	AsyncDbWriteFlag = &cli.BoolFlag{
		Name:  "async-db-write",
		Usage: "Asynchronously write substates to substate DB",
		Value: true,
	}
	asyncDbWrite = AsyncDbWriteFlag.Value
)

var (
	WorkersFlag = &cli.IntFlag{
		Name:  "workers",
		Usage: "Number of worker threads (goroutines), 0 for current CPU physical cores",
		Value: 4,
	}
	SkipTransferTxsFlag = &cli.BoolFlag{
		Name:  "skip-transfer-txs",
		Usage: "Skip executing transactions that only transfer ETH",
	}
	SkipCallTxsFlag = &cli.BoolFlag{
		Name:  "skip-call-txs",
		Usage: "Skip executing CALL transactions to accounts with contract bytecode",
	}
	SkipCreateTxsFlag = &cli.BoolFlag{
		Name:  "skip-create-txs",
		Usage: "Skip executing CREATE transactions",
	}
	BlockSegmentFlag = &cli.StringFlag{
		Name:     "block-segment",
		Usage:    "Single block segment (e.g. 1001, 1_001, 1_001-2_000, 1-2k, 1-2M)",
		Required: true,
	}
	BlockSegmentListFlag = &cli.StringFlag{
		Name:     "block-segment-list",
		Usage:    "One or more block segments, e.g. '0-1M,1000-1100k,1100001,1_100_002-1_101_000'",
		Required: true,
	}
	TxListFlag = &cli.PathFlag{
		Name:  "tx-list",
		Usage: "Path of txt file with block numbers (e.g., 1001) and/or tx indexes (e.g., 1001_0 or 1001,1) to replay",
	}
)

type BlockSegment struct {
	First, Last uint64
}

func NewBlockSegment(first, last uint64) *BlockSegment {
	return &BlockSegment{First: first, Last: last}
}

func ParseBlockSegment(s string) (*BlockSegment, error) {
	var err error
	// <first>: first block number
	// <last>: optional, last block number
	// <siunit>: optinal, k for 1000, M for 1000000
	re := regexp.MustCompile(`^(?P<first>[0-9][0-9_]*)((-|~)(?P<last>[0-9][0-9_]*)(?P<siunit>[kM]?))?$`)
	seg := &BlockSegment{}
	if !re.MatchString(s) {
		return nil, fmt.Errorf("invalid block segment string: %q", s)
	}
	matches := re.FindStringSubmatch(s)
	first := strings.ReplaceAll(matches[re.SubexpIndex("first")], "_", "")
	seg.First, err = strconv.ParseUint(first, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid block segment first: %s", err)
	}
	last := strings.ReplaceAll(matches[re.SubexpIndex("last")], "_", "")
	if len(last) == 0 {
		seg.Last = seg.First
	} else {
		seg.Last, err = strconv.ParseUint(last, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid block segment last: %s", err)
		}
	}
	siunit := matches[re.SubexpIndex("siunit")]
	switch siunit {
	case "k":
		seg.First = seg.First*1_000 + 1
		seg.Last = seg.Last * 1_000
	case "M":
		seg.First = seg.First*1_000_000 + 1
		seg.Last = seg.Last * 1_000_000
	}
	if seg.First > seg.Last {
		return nil, fmt.Errorf("block segment first is larger than last: %v-%v", seg.First, seg.Last)
	}
	return seg, nil
}

type BlockSegmentList = []*BlockSegment

func ParseBlockSegmentList(s string) (BlockSegmentList, error) {
	var err error

	lxs := strings.Split(s, ",")
	br := make(BlockSegmentList, len(lxs))
	for i, lx := range lxs {
		br[i], err = ParseBlockSegment(lx)
		if err != nil {
			return nil, err
		}
	}

	return br, nil
}

type TxListElem struct {
	block uint64
	tx    int
}

func ParseTxList(listPath string) (map[uint64]struct{}, map[TxListElem]struct{}) {
	blockSet := make(map[uint64]struct{})
	txSet := make(map[TxListElem]struct{})

	listFile, err := os.Open(listPath)
	if err != nil {
		panic(err)
	}
	defer listFile.Close()

	sc := bufio.NewScanner(listFile)
	for sc.Scan() {
		line := sc.Text()
		s := strings.TrimSpace(line)
		s = strings.ReplaceAll(s, "_", " ")
		s = strings.ReplaceAll(s, ",", " ")
		ts := strings.Fields(s)
		switch len(ts) {
		case 1:
			block, err := strconv.ParseUint(ts[0], 10, 64)
			if err != nil {
				panic(err)
			}
			blockSet[block] = struct{}{}
		case 2:
			// TODO: parse block and tx
			block, err := strconv.ParseUint(ts[0], 10, 64)
			if err != nil {
				panic(err)
			}
			tx, err := strconv.ParseInt(ts[1], 10, 64)
			if err != nil {
				panic(err)
			}
			txSet[TxListElem{block, int(tx)}] = struct{}{}
		}
	}
	fmt.Printf("record-replay: --tx-list=%s\n", listPath)
	fmt.Printf("record-replay: %v block numbers, %v tx indexes in tx list\n", len(blockSet), len(txSet))

	return blockSet, txSet
}
