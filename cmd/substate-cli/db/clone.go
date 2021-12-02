package db

import (
	"fmt"
	"strconv"

	"github.com/ethereum/go-ethereum/core/rawdb"
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

	srcBackend, err := rawdb.NewLevelDBDatabase(srcPath, 1024, 100, "srcDB", true)
	if err != nil {
		return fmt.Errorf("substate-cli db clone: error opening %s: %v", srcPath, err)
	}
	srcDB := research.NewSubstateDB(srcBackend)
	defer srcDB.Close()

	// Create dst DB
	dstBackend, err := rawdb.NewLevelDBDatabase(dstPath, 1024, 100, "srcDB", false)
	if err != nil {
		return fmt.Errorf("substate-cli db clone: error creating %s: %v", dstPath, err)
	}
	dstDB := research.NewSubstateDB(dstBackend)
	defer dstDB.Close()

	cloneTask := func(block uint64, tx int, substate *research.Substate, taskPool *research.SubstateTaskPool) error {
		dstDB.PutSubstate(block, tx, substate)
		return nil
	}

	taskPool := research.NewSubstateTaskPool("substate-cli db clone", cloneTask, uint64(first), uint64(last), ctx)
	taskPool.DB = srcDB
	err = taskPool.Execute()
	return err
}
