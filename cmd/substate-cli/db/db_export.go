package db

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/ethereum/go-ethereum/research"
	cli "github.com/urfave/cli/v2"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

var DbExportCommand = &cli.Command{
	Action: dbExport,
	Name:   "db-export",
	Usage:  "Export substates to files of a given block segment",
	Flags: []cli.Flag{
		research.WorkersFlag,
		research.BlockSegmentFlag,
		research.SubstateDirFlag,
		&cli.PathFlag{
			Name:  "out-dir",
			Usage: "output directory to save exported substates",
			Value: "substate-db-export",
		},
		&cli.BoolFlag{
			Name:  "json",
			Usage: "Output Base64 JSON instead of binary",
		},
		&cli.BoolFlag{
			Name:  "hashed",
			Usage: "output substates with code hashes instead of raw bytecode",
		},
	},
	Description: `
substate-cli db-export command reads substates of a given block segment
and save each substates as one binary or json file in the output directory.
`,
	Category: "db",
}

func dbExport(ctx *cli.Context) error {
	var err error

	outJson := ctx.Bool("json")
	outHashed := ctx.Bool("hashed")

	research.SetSubstateFlags(ctx)
	research.OpenSubstateDBReadOnly()
	defer research.CloseSubstateDB()

	outDir := ctx.Path("out-dir")
	err = os.MkdirAll(outDir, 0775)
	if err != nil {
		return err
	}

	var marshaler interface {
		Marshal(m protoreflect.ProtoMessage) ([]byte, error)
	}
	var ext string
	if outJson {
		marshaler = protojson.MarshalOptions{
			Indent: "  ",
		}
		ext = "json"
	} else {
		marshaler = proto.MarshalOptions{}
		ext = "bin"
	}

	var suffix string
	if outHashed {
		suffix = "hashed"
	} else {
		suffix = "unhashed"
	}

	exportTask := func(block uint64, tx int, substate *research.Substate, taskPool *research.SubstateTaskPool) error {
		var err error

		if outHashed {
			substate = substate.HashedCopy()
		}

		var bs []byte
		bs, err = marshaler.Marshal(substate)
		if err != nil {
			return fmt.Errorf("%v_%v marshal failed: %w", block, tx, err)
		}

		name := fmt.Sprintf("substate_%v_%v_%s.%s", block, tx, suffix, ext)
		path := filepath.Join(outDir, name)

		err = os.WriteFile(path, bs, 0664)
		if err != nil {
			return err
		}

		return nil
	}

	taskPool := research.NewSubstateTaskPoolCli("substate-cli db-export", exportTask, ctx)

	segment, err := research.ParseBlockSegment(ctx.String(research.BlockSegmentFlag.Name))
	if err != nil {
		return fmt.Errorf("substate-cli db-clone: error parsing block segment: %s", err)
	}

	err = taskPool.ExecuteSegment(segment)

	return err
}
