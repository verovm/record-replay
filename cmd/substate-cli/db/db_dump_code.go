package db

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/rawdb"
	"github.com/ethereum/go-ethereum/ethdb"
	"github.com/ethereum/go-ethereum/research"
	"github.com/status-im/keycard-go/hexutils"
	cli "github.com/urfave/cli/v2"
	"google.golang.org/protobuf/proto"
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
		&cli.BoolFlag{
			Name:  "deployed-only",
			Usage: "output only deployed contracts, no init code",
			Value: false,
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
	db, err := rawdb.NewLevelDBDatabase(dbPath, 1024, 100, "substatedir", true)
	if err != nil {
		return fmt.Errorf("substate-cli: db-dump-code: error opening dbPath %s: %v", dbPath, err)
	}

	deployedOnly := ctx.Bool("deployed-only")

	var iter ethdb.Iterator
	var iterCount int
	codeHashSet := make(map[common.Hash]bool)
	substateDB := research.NewSubstateDB(db)
	var lastSec float64

	if deployedOnly {
		iter = db.NewIterator([]byte(research.Stage1SubstatePrefix), nil)
	} else {
		iter = db.NewIterator([]byte(research.Stage1CodePrefix), nil)
	}

	for iterCount = 0; iter.Next(); iterCount++ {
		key := iter.Key()
		value := iter.Value()

		if deployedOnly {
			// Dump only deployed code
			block, tx, err := research.DecodeStage1SubstateKey(key)
			if err != nil {
				return fmt.Errorf("substate-cli: db-dump-code: error decoding substate key: %v", err)
			}

			hashedSubstate := &research.Substate{}
			err = proto.Unmarshal(value, hashedSubstate)
			if err != nil {
				panic(fmt.Errorf("record-replay: error decoding substate %v_%v: %v", block, tx, err))
			}

			codeHashSlice := []common.Hash{}
			for _, entry := range hashedSubstate.InputAlloc.Alloc {
				account := entry.Account
				if codeHash := research.BytesToHash(account.GetCodeHash()); codeHash != nil {
					if !codeHashSet[*codeHash] {
						codeHashSlice = append(codeHashSlice, *codeHash)
					}
				}
			}

			for _, entry := range hashedSubstate.OutputAlloc.Alloc {
				account := entry.Account
				if codeHash := research.BytesToHash(account.GetCodeHash()); codeHash != nil {
					if !codeHashSet[*codeHash] {
						codeHashSlice = append(codeHashSlice, *codeHash)
					}
				}
			}

			for _, codeHash := range codeHashSlice {
				code := substateDB.GetCode(codeHash)

				outputPath := filepath.Join(outputDir, codeHash.Hex()+".hex")
				outputCode := hexutils.BytesToHex(code)
				err = ioutil.WriteFile(outputPath, []byte(outputCode), 0o644)
				if err != nil {
					return fmt.Errorf("substate-cli: db-dump-code: error writing file: %v", err)
				}
				codeHashSet[codeHash] = true
			}

			duration := time.Since(start)
			sec := duration.Seconds()
			if iterCount%1000 == 0 && sec > lastSec+5 {
				fmt.Printf("substate-cli: db-dump-code: elapsed time: %v, #substates = %v, #bytecodes = %v\n", duration.Round(1*time.Microsecond), iterCount, len(codeHashSet))
				lastSec = sec
			}
		} else {
			// Dump both deployed and init code
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
			codeHashSet[codeHash] = true

			duration := time.Since(start)
			sec := duration.Seconds()
			if iterCount%1000 == 0 && sec > lastSec+5 {
				fmt.Printf("substate-cli: db-dump-code: elapsed time: %v, #bytecodes = %v\n", duration.Round(1*time.Microsecond), len(codeHashSet))
				lastSec = sec
			}
		}
	}
	iter.Release()
	err = iter.Error()
	if err != nil {
		return fmt.Errorf("substate-cli: db-dump-code: error releasing iterator: %v", err)
	}

	duration := time.Since(start)
	fmt.Printf("substate-cli: db-dump-code: elapsed time: %v, #bytecodes = %v\n", duration.Round(1*time.Millisecond), len(codeHashSet))

	return nil
}
