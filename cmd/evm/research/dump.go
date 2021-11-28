package research

import (
	"fmt"
	"path/filepath"
	"runtime"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/syndtr/goleveldb/leveldb"
	leveldb_errors "github.com/syndtr/goleveldb/leveldb/errors"
	leveldb_opt "github.com/syndtr/goleveldb/leveldb/opt"
	cli "gopkg.in/urfave/cli.v1"

	// import go-ethereum/research
	"github.com/ethereum/go-ethereum/research"
)

var targetSubstateDir string
var targetSubstateDB, targetCodeDB *leveldb.DB

func OpenTargetSubstateDB() {
	fmt.Println("stage1-substate: OpenTargetSubstateDB")

	var err error
	var opt leveldb_opt.Options
	var path string

	// increase BlockCacheCapacity to 1GiB
	opt.BlockCacheCapacity = 1 * leveldb_opt.GiB
	// decrease OpenFilesCacheCapacity to avoid "Too many file opened" error
	opt.OpenFilesCacheCapacity = 50

	dbNameMap := map[string]*leveldb.DB{
		"substate": nil,
		"code":     nil,
	}

	for name := range dbNameMap {
		var db *leveldb.DB
		path = filepath.Join(targetSubstateDir, name)
		db, err = leveldb.OpenFile(path, &opt)
		if _, corrupted := err.(*leveldb_errors.ErrCorrupted); corrupted {
			db, err = leveldb.RecoverFile(path, &opt)
		}
		if err != nil {
			panic(fmt.Errorf("error opening target substate leveldb %s: %v", path, err))
		}

		fmt.Printf("stage1-substate: successfully opened target %s leveldb\n", name)

		dbNameMap[name] = db
	}

	targetSubstateDB = dbNameMap["substate"]
	targetCodeDB = dbNameMap["code"]
}

func CloseTargetSubstateDB() {
	defer fmt.Println("stage1-substate: CloseTargetSubstateDB")

	dbNameMap := map[string]*leveldb.DB{
		"substate": targetSubstateDB,
		"code":     targetCodeDB,
	}

	for name, db := range dbNameMap {
		db.Close()
		fmt.Printf("stage1-substate: successfully closed target %s leveldb\n", name)
	}
}

func PutTargetCode(code []byte) {
	codeHash := crypto.Keccak256Hash(code)
	err := targetCodeDB.Put(codeHash.Bytes(), code, nil)
	if err != nil {
		panic(fmt.Errorf("stage1-substate: error putting code %s: %v", codeHash.Hex(), err))
	}
}

func PutTargetSubstate(block uint64, tx int, substate *research.Substate) {
	var err error

	// put deployed/creation code
	for _, account := range substate.InputAlloc {
		PutTargetCode(account.Code)
	}
	for _, account := range substate.OutputAlloc {
		PutTargetCode(account.Code)
	}
	if msg := substate.Message; msg.To == nil {
		PutTargetCode(msg.Data)
	}

	key := []byte(fmt.Sprintf("%v_%v", block, tx))
	defer func() {
		if err != nil {
			panic(fmt.Errorf("stage1-substate: error putting substate %s into substate DB", key))
		}
	}()

	value, err := rlp.EncodeToBytes(substate)
	if err != nil {
		return
	}

	err = targetSubstateDB.Put(key, value, nil)
	if err != nil {
		return
	}
}

func dumpSubstateBlock(ctx *cli.Context, block int64) (int64, error) {
	var err error
	var numTx int64 = 0

	for block, tx := uint64(block), 0; ; tx++ {

		if !research.HasSubstate(block, tx) {
			break
		}

		substate := research.GetSubstate(block, tx)
		PutTargetSubstate(block, tx, substate)

		if err != nil {
			return numTx, fmt.Errorf("stage1-substate: dumpSubstateBlock %v_%v: %v", block, tx, err)
		}

		numTx++
	}

	return numTx, nil
}

func DumpSubstate(ctx *cli.Context) error {
	if len(ctx.Args()) != 3 {
		return fmt.Errorf("stage1-substate: dump-substate command requires exactly 3 arguments")
	}

	start := time.Now()

	targetSubstateDir = ctx.Args().Get(0)
	first, ferr := strconv.ParseInt(ctx.Args().Get(1), 10, 64)
	last, lerr := strconv.ParseInt(ctx.Args().Get(2), 10, 64)
	if ferr != nil || lerr != nil {
		return fmt.Errorf("stage1-substate: DumpSubstate: error in parsing parameters: block number not an integer")
	}
	if first < 0 || last < 0 {
		return fmt.Errorf("stage1-substate: DumpSubstate: error: block number must be greater than 0")
	}
	if first > last {
		return fmt.Errorf("stage1-substate: DumpSubstate: error: first block has larger number than last block")
	}

	research.OpenSubstateDBReadOnly()
	defer research.CloseSubstateDB()

	OpenTargetSubstateDB()
	defer CloseTargetSubstateDB()

	var totalNumBlock, totalNumTx int64
	defer func() {
		duration := time.Since(start) + 1*time.Nanosecond
		sec := duration.Seconds()

		nb, nt := atomic.LoadInt64(&totalNumBlock), atomic.LoadInt64(&totalNumTx)
		blkPerSec := float64(nb) / sec
		txPerSec := float64(nt) / sec
		fmt.Printf("stage1-substate: DumpSubstate: total #block = %v\n", nb)
		fmt.Printf("stage1-substate: DumpSubstate: total #tx    = %v\n", nt)
		fmt.Printf("stage1-substate: DumpSubstate: %.2f blk/s, %.2f tx/s\n", blkPerSec, txPerSec)
		fmt.Printf("stage1-substate: DumpSubstate done in %v\n", duration.Round(1*time.Millisecond))
	}()

	numWorker := ctx.Int(WorkersFlag.Name)
	// numProcs + work producer (1) + main thread (1)
	numProcs := numWorker + 2
	if goMaxProcs := runtime.GOMAXPROCS(0); goMaxProcs < numProcs {
		runtime.GOMAXPROCS(numProcs)
	}

	fmt.Printf("stage1-substate: DumpSubstate: #CPU = %v, #worker = %v, #thread = %v\n", runtime.NumCPU(), numWorker, runtime.GOMAXPROCS(0))

	workChan := make(chan int64, numWorker*10)
	doneChan := make(chan interface{}, numWorker*10)
	stopChan := make(chan struct{}, numWorker)
	wg := sync.WaitGroup{}
	defer func() {
		// stop all workers
		for i := 0; i < numWorker; i++ {
			stopChan <- struct{}{}
		}
		// stop work producer (1)
		stopChan <- struct{}{}

		wg.Wait()
		close(workChan)
		close(doneChan)
	}()
	for i := 0; i < numWorker; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			for {
				select {

				case block := <-workChan:
					nt, err := dumpSubstateBlock(ctx, block)
					atomic.AddInt64(&totalNumTx, nt)
					atomic.AddInt64(&totalNumBlock, 1)
					if err != nil {
						doneChan <- err
					} else {
						doneChan <- block
					}

				case <-stopChan:
					return

				}
			}
		}()
	}

	wg.Add(1)
	go func() {
		defer wg.Done()

		for block := first; block <= last; block++ {
			select {

			case workChan <- block:
				continue

			case <-stopChan:
				return

			}
		}
	}()

	var lastSec float64
	var lastNumBlock, lastNumTx int64
	for block := first; block <= last; block++ {
		data := <-doneChan
		switch t := data.(type) {

		case int64:
			block := data.(int64)
			if block%10000 == 0 {
				duration := time.Since(start) + 1*time.Nanosecond
				sec := duration.Seconds()

				nb, nt := atomic.LoadInt64(&totalNumBlock), atomic.LoadInt64(&totalNumTx)
				blkPerSec := float64(nb-lastNumBlock) / (sec - lastSec)
				txPerSec := float64(nt-lastNumTx) / (sec - lastSec)
				fmt.Printf("stage1-substate: elapsed time: %v, number = %v\n", duration.Round(1*time.Millisecond), block)
				fmt.Printf("stage1-substate: %.2f blk/s, %.2f tx/s\n", blkPerSec, txPerSec)

				lastSec, lastNumBlock, lastNumTx = sec, nb, nt
			}

		case error:
			err := data.(error)
			return err

		default:
			return fmt.Errorf("stage1-substate: DumpSubstate: unknown type %T value from doneChan", t)

		}
	}

	return nil
}
