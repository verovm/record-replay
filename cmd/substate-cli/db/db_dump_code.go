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
	fmt.Printf("substate-cli: db-dump-code: outputDir: %s\n", outputDir)
	err = os.MkdirAll(outputDir, 0o755)
	if err != nil {
		return err
	}

	dbPath := ctx.Path(research.SubstateDirFlag.Name)
	fmt.Printf("substate-cli: db-dump-code: dbPath: %s\n", dbPath)
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
	var count int
	var lastSec float64
	for count = 0; iter.Next(); count++ {
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

		duration := time.Since(start)
		sec := duration.Seconds()
		if count%1000 == 0 && sec > lastSec+5 {
			fmt.Printf("substate-cli: db-dump-code: elapsed time: %v, #bytecodes = %v\n", duration.Round(1*time.Microsecond), count)
			lastSec = sec
		}
	}
	iter.Release()
	err = iter.Error()
	if err != nil {
		return fmt.Errorf("substate-cli: db-dump-code: error releasing iterator: %v", err)
	}

	duration := time.Since(start)
	fmt.Printf("substate-cli: db-dump-code: elapsed time: %v, #bytecodes = %v\n", duration.Round(1*time.Millisecond), count)

	return nil
}
