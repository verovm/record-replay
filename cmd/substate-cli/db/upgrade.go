package db

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/research"
	"github.com/syndtr/goleveldb/leveldb"
	leveldb_opt "github.com/syndtr/goleveldb/leveldb/opt"
	cli "gopkg.in/urfave/cli.v1"
)

var UpgradeCommand = cli.Command{
	Action:    upgrade,
	Name:      "upgrade",
	Usage:     "upgrade old DB layout (stage1-substate/) to new unified DB layout (substate.ethereum)",
	ArgsUsage: "<stage1-substate> <substate.ethereum>",
	Flags:     []cli.Flag{},
	Description: `
The substate db upgrade command requires two arguments:
<stage1-substate> <substate.ethereum>

<stage1-substate> is old DB layout:
- stage1-substate/substate is a DB with ("block_tx" -> substateRLP) pairs
- stage1-substate/code is a DB with (codeHash -> code) pairs

<substate.ethereum> is new unified DB layout. First 2 bytes of a key in substate DB
represents different data types as follows:
- 1s: substateRLP, a key is "1s"+N+T with transaction index T at block N.
T and N are encoded in a big-endian 64-bit binary.
- 1c: code, a key is "1c"+codeHash where codeHash is Keccak256 hash of the bytecode.
`,
}

func upgrade(ctx *cli.Context) error {
	if len(ctx.Args()) != 2 {
		return fmt.Errorf("substate-cli db upgrade: command requires exactly 2 arguments")
	}

	oldPath := ctx.Args().Get(0)
	oldSubstatePath := filepath.Join(oldPath, "substate")
	oldCodePath := filepath.Join(oldPath, "code")

	newPath := ctx.Args().Get(1)

	var (
		err           error
		oldSubstateDB *leveldb.DB
		oldCodeDB     *leveldb.DB
		oldOpt        *leveldb_opt.Options

		newSubstateDB *leveldb.DB
		newOpt        *leveldb_opt.Options
	)

	oldOpt = &leveldb_opt.Options{
		BlockCacheCapacity:     1 * leveldb_opt.GiB,
		OpenFilesCacheCapacity: 50,

		ErrorIfMissing: true,
		ReadOnly:       true,
	}
	oldSubstateDB, err = leveldb.OpenFile(oldSubstatePath, oldOpt)
	if err != nil {
		panic(err)
	}
	oldCodeDB, err = leveldb.OpenFile(oldCodePath, oldOpt)
	if err != nil {
		panic(err)
	}

	newOpt = &leveldb_opt.Options{
		BlockCacheCapacity:     1 * leveldb_opt.GiB,
		OpenFilesCacheCapacity: 50,
	}
	newSubstateDB, err = leveldb.OpenFile(newPath, newOpt)
	if err != nil {
		panic(err)
	}

	wg := &sync.WaitGroup{}

	// copy substate
	wg.Add(1)
	go func() {
		fmt.Printf("stage1-substate/substate -> substate.ethereum\n")
		substateIter := oldSubstateDB.NewIterator(nil, nil)
		n := int64(0)
		for substateIter.Next() {
			oldKey := substateIter.Key()
			value := substateIter.Value()
			slice := strings.Split(string(oldKey), "_")
			block, err := strconv.ParseUint(slice[0], 10, 64)
			if err != nil {
				panic(err)
			}
			tx, err := strconv.Atoi(slice[1])
			if err != nil {
				panic(err)
			}
			key := research.Stage1SubstateKey(block, tx)
			err = newSubstateDB.Put(key, value, nil)
			if err != nil {
				panic(err)
			}
			n += 1
			if n%1000 == 0 {
				fmt.Printf("substate %dk\n", n/1000)
			}
		}
		substateIter.Release()
		if err := substateIter.Error(); err != nil {
			panic(err)
		}
		wg.Done()
	}()

	// copy code
	wg.Add(1)
	go func() {
		fmt.Printf("stage1-substate/substate -> substate.ethereum\n")
		codeIter := oldCodeDB.NewIterator(nil, nil)
		n := int64(0)
		for codeIter.Next() {
			codeHash := codeIter.Key()
			value := codeIter.Value()
			key := research.Stage1CodeKey(common.BytesToHash(codeHash))
			err = newSubstateDB.Put(key, value, nil)
			if err != nil {
				panic(err)
			}
			n += 1
			if n%1000 == 0 {
				fmt.Printf("code %dk\n", n/1000)
			}
		}
		codeIter.Release()
		if err := codeIter.Error(); err != nil {
			panic(err)
		}
		wg.Done()
	}()

	// wait
	wg.Wait()

	return nil
}
