package db

import (
	"fmt"

	"github.com/ethereum/go-ethereum/cmd/substate-cli/rr03/research"
	"github.com/ethereum/go-ethereum/core/rawdb"
	cli "github.com/urfave/cli/v2"
)

var CloneCommand = &cli.Command{
	Action: clone,
	Name:   "rr0.3-db-clone",
	Usage:  "Create a clone of rr0.3 DB of a given block segment",
	Flags: []cli.Flag{
		research.WorkersFlag,
		research.BlockSegmentFlag,
		&cli.PathFlag{
			Name:     "src-path",
			Usage:    "Source DB path",
			Required: true,
		},
		&cli.PathFlag{
			Name:     "dst-path",
			Usage:    "Destination DB path",
			Required: true,
		},
	},
	Description: `
substate-cli db clone creates a clone of rr0.3 DB of a given block segment.
This loads a complete substate from src-path, then save it to dst path.
The dst-path will always store substates in the latest encoding.
`,
	Category: "rr0.3-db",
}

func clone(ctx *cli.Context) error {
	var err error

	srcPath := ctx.Path("src-path")
	srcBackend, err := rawdb.NewLevelDBDatabase(srcPath, 1024, 100, "srcDB", true)
	if err != nil {
		return fmt.Errorf("substate-cli db clone: error opening %s: %v", srcPath, err)
	}
	srcDB := research.NewSubstateDB(srcBackend)
	defer srcDB.Close()

	// Create dst DB
	dstPath := ctx.Path("dst-path")
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

	taskPool := &research.SubstateTaskPool{
		Name:     "substate-cli db clone",
		TaskFunc: cloneTask,
		Config:   research.NewSubstateTaskConfigCli(ctx),

		DB: srcDB,
	}

	segment, err := research.ParseBlockSegment(ctx.String(research.BlockSegmentFlag.Name))
	if err != nil {
		return fmt.Errorf("substate-cli db clone: error parsing block segment: %s", err)
	}

	err = taskPool.ExecuteSegment(segment)

	return err
}
