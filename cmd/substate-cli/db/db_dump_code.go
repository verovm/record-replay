package db

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"time"

	"github.com/ethereum/go-ethereum/research"
	"github.com/status-im/keycard-go/hexutils"
	"github.com/syndtr/goleveldb/leveldb"
	leveldb_opt "github.com/syndtr/goleveldb/leveldb/opt"
	leveldb_util "github.com/syndtr/goleveldb/leveldb/util"
	cli "github.com/urfave/cli/v2"
)

var DbDumpCodeCommand = &cli.Command{
	Action: dbDumpCode,
	Name:   "db-dump-code",
	Usage:  "Dump all bytecodes stored in substate DB",
	Flags: []cli.Flag{
		research.SubstateDirFlag,
		&cli.PathFlag{
			Name:  "out-dir",
			Usage: "output directory to save dumped bytecode",
			Value: "substate-db-code",
		},
	},
	Description: `
The db-dump-code reads and saves all bytecodes in substate DB in the output directory.
`,
	Category: "db",
}

func dbDumpCode(ctx *cli.Context) error {
	var err error

	start := time.Now()
	fmt.Printf("substate-cli: db-dump-code: begin")

	outputDir := ctx.Path("out-dir")
	err = os.MkdirAll(outputDir, 0o755)
	if err != nil {
		return err
	}

	dbPath := ctx.Path(research.SubstateDirFlag.Name)
	dbOpt := &leveldb_opt.Options{
		BlockCacheCapacity:     1 * leveldb_opt.GiB,
		OpenFilesCacheCapacity: 50,

		ErrorIfMissing: true,
		ReadOnly:       true,
	}
	db, err := leveldb.OpenFile(dbPath, dbOpt)
	if err != nil {
		return fmt.Errorf("substate-cli: db-dump-code: error opening dbPath %s: %v", dbPath, err)
	}

	iterRange := leveldb_util.BytesPrefix([]byte(research.Stage1CodePrefix))
	iter := db.NewIterator(iterRange, nil)
	for iter.Next() {
		key := iter.Key()
		value := iter.Value()

		codeHash, err := research.DecodeStage1CodeKey(key)
		if err != nil {
			return fmt.Errorf("substate-cli: db-dump-code: error decoding code key: %v", err)
		}
		code := value

		outputPath := filepath.Join(outputDir, codeHash.Hex()+".hex")
		outputCode := hexutils.BytesToHex(code)
		err = ioutil.WriteFile(outputPath, []byte(outputCode), 0o644)
		if err != nil {
			return fmt.Errorf("substate-cli: db-dump-code: error writing file: %v", err)
		}
	}
	iter.Release()
	err = iter.Error()
	if err != nil {
		return fmt.Errorf("substate-cli: db-dump-code: error releasing iterator: %v", err)
	}

	duration := time.Since(start)
	fmt.Printf("substate-cli: db-dump-code: elapsed time: %v\n", duration.Round(1*time.Millisecond))

	return nil
}
