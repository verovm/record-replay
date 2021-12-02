package db

import (
	"fmt"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/ethereum/go-ethereum/ethdb/leveldb"
	"github.com/ethereum/go-ethereum/research"
	cli "gopkg.in/urfave/cli.v1"
)

var CloneCommand = cli.Command{
	Action:    clone,
	Name:      "clone",
	Usage:     "Create a clone DB of a given range of blocks",
	ArgsUsage: "<srcPath> <dstPath> <blockNumFirst> <blockNumLast>",
	Flags: []cli.Flag{
		research.WorkersFlag,
	},
	Description: `
The substate-cli db clone command requires four arguments:
    <srcPath> <dstPath> <blockNumFirst> <blockNumLast>
<srcPath> is the original substate database to read the information.
<dstPath> is the target substate database to write the information
<blockNumFirst> and <blockNumLast> are the first and
last block of the inclusive range of blocks to clone.`,
}

func clone(ctx *cli.Context) error {
	var err error

	start := time.Now()

	if len(ctx.Args()) != 4 {
		return fmt.Errorf("substate-cli db clone command requires exactly 4 arguments")
	}

	srcPath := ctx.Args().Get(0)
	dstPath := ctx.Args().Get(1)
	first, ferr := strconv.ParseInt(ctx.Args().Get(2), 10, 64)
	last, lerr := strconv.ParseInt(ctx.Args().Get(3), 10, 64)
	if ferr != nil || lerr != nil {
		return fmt.Errorf("substate-cli db clone: error in parsing parameters: block number not an integer")
	}
	if first < 0 || last < 0 {
		return fmt.Errorf("substate-cli db clone: error: block number must be greater than 0")
	}
	if first > last {
		return fmt.Errorf("substate-cli db clone: error: first block has larger number than last block")
	}

	srcBackend, err := leveldb.New(srcPath, 1024, 100, "srcDB", true)
	if err != nil {
		return fmt.Errorf("substate-cli db clone: error opening %s: %v", srcPath, err)
	}
	srcDB := research.NewSubstateDB(srcBackend)

	// Create dst DB
	dstBackend, err := leveldb.New(srcPath, 1024, 100, "srcDB", false)
	if err != nil {
		return fmt.Errorf("substate-cli db clone: error creating %s: %v", dstPath, err)
	}
	dstDB := research.NewSubstateDB(dstBackend)

	var totalNumBlock, totalNumTx int64
	var lastSec float64
	var lastNumBlock, lastNumTx int64

	firstU64 := uint64(first)
	lastU64 := uint64(last)
	for block := firstU64; block <= lastU64; block++ {
		substates := srcDB.GetBlockSubstates(block)
		for tx, substate := range substates {
			dstDB.PutSubstate(block, tx, substate)
		}
		duration := time.Since(start) + 1*time.Nanosecond
		sec := duration.Seconds()
		if block == lastU64 ||
			(block%10000 == 0 && sec > lastSec+5) ||
			(block%1000 == 0 && sec > lastSec+10) ||
			(block%100 == 0 && sec > lastSec+20) ||
			(block%10 == 0 && sec > lastSec+40) ||
			(sec > lastSec+60) {
			nb, nt := atomic.LoadInt64(&totalNumBlock), atomic.LoadInt64(&totalNumTx)
			blkPerSec := float64(nb-lastNumBlock) / (sec - lastSec)
			txPerSec := float64(nt-lastNumTx) / (sec - lastSec)
			fmt.Printf("substate-cli db clone: elapsed time: %v, number = %v\n", duration.Round(1*time.Millisecond), block)
			fmt.Printf("substate-cli db clone: %.2f blk/s, %.2f tx/s\n", blkPerSec, txPerSec)

			lastSec, lastNumBlock, lastNumTx = sec, nb, nt
		}
	}

	err = srcDB.Close()
	if err != nil {
		return fmt.Errorf("substate-cli db clone: error closing srcDB %s: %v", srcPath, err)
	}

	err = dstDB.Close()
	if err != nil {
		return fmt.Errorf("substate-cli db clone: error closing dstDB %s: %v", dstPath, err)
	}

	return nil
}
