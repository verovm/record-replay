package db

import (
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/research"
	"github.com/syndtr/goleveldb/leveldb"
	leveldb_opt "github.com/syndtr/goleveldb/leveldb/opt"
	leveldb_util "github.com/syndtr/goleveldb/leveldb/util"
	cli "github.com/urfave/cli/v2"
)

var DbCompactCommand = &cli.Command{
	Action: dbCompact,
	Name:   "db-compact",
	Usage:  "Run compaction functionality of the backend DB",
	Flags: []cli.Flag{
		research.SubstateDirFlag,
	},
	Description: `
The substate-cli db-compact command runs compaction functionality of
the backend of the given substate DB`,
	Category: "db",
}

func dbCompact(ctx *cli.Context) error {
	var err error

	dbPath := ctx.Path(research.SubstateDirFlag.Name)
	dbOpt := &leveldb_opt.Options{
		BlockCacheCapacity:     1 * leveldb_opt.GiB,
		OpenFilesCacheCapacity: 50,

		ErrorIfMissing: true,
		ReadOnly:       false,
	}
	db, err := leveldb.OpenFile(dbPath, dbOpt)
	if err != nil {
		return fmt.Errorf("substate-cli db-compact: error opening dbPath %s: %w", dbPath, err)
	}

	start := time.Now()
	fmt.Printf("substate-cli db-compact: compaction begin\n")
	wg := &sync.WaitGroup{}
	wg.Add(1)
	go func() {
		fmt.Printf("substate-cli db-compact: compacting substates...\n")
		for b, m, s := 0, 0xffffffff, 0xffff; b <= m; b += s {
			start := research.Stage1SubstateKey(uint64(b), 0)
			end := research.Stage1SubstateKey(uint64(b+s), 0)
			if b+s >= m {
				end = research.Stage1SubstateKey(math.MaxUint64, math.MaxInt)
			}
			err = db.CompactRange(leveldb_util.Range{Start: start, Limit: end})
			if err != nil {
				panic(fmt.Errorf("substate-cli db-compact: error compacting dbPath %s: %w", dbPath, err))
			}
		}

		fmt.Printf("substate-cli db-compact: compacting bytecodes...\n")
		for b := 0; b <= 255; b++ {
			start := research.Stage1CodeKey(common.Hash{byte(b)})
			end := research.Stage1CodeKey(common.Hash{byte(b + 1)})
			if b == 255 {
				end = research.Stage1CodeKey(common.MaxHash)
			}
			err = db.CompactRange(leveldb_util.Range{Start: start, Limit: end})
			if err != nil {
				panic(fmt.Errorf("substate-cli db-compact: error compacting dbPath %s: %w", dbPath, err))
			}
		}

		if err != nil {
			panic(fmt.Errorf("substate-cli db-compact: error compacting dbPath %s: %w", dbPath, err))
		}
		wg.Done()
	}()
	wg.Wait()
	duration := time.Since(start)
	fmt.Printf("substate-cli db-compact: compaction completed\n")
	fmt.Printf("substate-cli db-compact: elapsed time: %v\n", duration.Round(1*time.Millisecond))

	return nil
}
