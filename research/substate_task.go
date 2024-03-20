package research

import (
	"bytes"
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/shirou/gopsutil/cpu"
	cli "github.com/urfave/cli/v2"
)

type SubstateTaskFunc func(block uint64, tx int, substate *Substate, taskPool *SubstateTaskPool) error

type SubstateTaskConfig struct {
	Workers int

	SkipTransferTxs bool
	SkipCallTxs     bool
	SkipCreateTxs   bool
}

func NewSubstateTaskConfigCli(ctx *cli.Context) *SubstateTaskConfig {
	return &SubstateTaskConfig{
		Workers: ctx.Int(WorkersFlag.Name),

		SkipTransferTxs: ctx.Bool(SkipTransferTxsFlag.Name),
		SkipCallTxs:     ctx.Bool(SkipCallTxsFlag.Name),
		SkipCreateTxs:   ctx.Bool(SkipCreateTxsFlag.Name),
	}
}

type SubstateTaskPool struct {
	Name     string
	TaskFunc SubstateTaskFunc
	Config   *SubstateTaskConfig

	DB *SubstateDB
}

func NewSubstateTaskPool(name string, taskFunc SubstateTaskFunc, config *SubstateTaskConfig) *SubstateTaskPool {
	return &SubstateTaskPool{
		Name:     name,
		TaskFunc: taskFunc,
		Config:   config,

		DB: staticSubstateDB,
	}
}

func NewSubstateTaskPoolCli(name string, taskFunc SubstateTaskFunc, ctx *cli.Context) *SubstateTaskPool {
	return &SubstateTaskPool{
		Name:     name,
		TaskFunc: taskFunc,
		Config:   NewSubstateTaskConfigCli(ctx),

		DB: staticSubstateDB,
	}
}

// NumWorkers calculates number of workers especially when --workers=0
func (pool *SubstateTaskPool) NumWorkers() int {
	// return pool.Workers if it is positive integer
	if pool.Config.Workers > 0 {
		return pool.Config.Workers
	}

	// try to return number of physical cores
	cores, err := cpu.Counts(false)
	if err == nil {
		return cores
	}

	// return number of logical cores
	return runtime.NumCPU()
}

// ExecuteBlock function iterates on substates of a given block call TaskFunc
func (pool *SubstateTaskPool) ExecuteBlock(block uint64) (numTx int64, err error) {
	for tx, substate := range pool.DB.GetBlockSubstates(block) {
		skipTx := false
		to := substate.TxMessage.To

		if !skipTx && pool.Config.SkipTransferTxs && to != nil {
			// skip regular transactions (ETH transfer)
			for _, entry := range substate.InputAlloc.Alloc {
				addr := entry.Address
				account := entry.Account
				if bytes.Equal(addr, to.Value) && len(account.GetCode()) == 0 {
					skipTx = true
					break
				}
			}
		}

		if !skipTx && pool.Config.SkipCallTxs && to != nil {
			// skip CALL trasnactions with contract bytecode
			for _, entry := range substate.InputAlloc.Alloc {
				addr := entry.Address
				account := entry.Account
				if bytes.Equal(addr, to.Value) && len(account.GetCode()) > 0 {
					skipTx = true
					break
				}
			}
		}

		if !skipTx && pool.Config.SkipCreateTxs && to == nil {
			// skip CREATE transactions
			skipTx = true
		}

		if skipTx {
			continue
		}

		err = pool.TaskFunc(block, tx, substate, pool)
		if err != nil {
			return numTx, fmt.Errorf("%s: %v_%v: %v", pool.Name, block, tx, err)
		}

		numTx++
	}

	return numTx, nil
}

// Execute function spawns worker goroutines and schedule tasks.
func (pool *SubstateTaskPool) ExecuteSegment(segment *BlockSegment) error {
	start := time.Now()

	var totalNumBlock, totalNumTx int64
	defer func() {
		duration := time.Since(start) + 1*time.Nanosecond
		sec := duration.Seconds()

		nb, nt := atomic.LoadInt64(&totalNumBlock), atomic.LoadInt64(&totalNumTx)
		blkPerSec := float64(nb) / sec
		txPerSec := float64(nt) / sec
		fmt.Printf("%s: block segment = %v-%v\n", pool.Name, segment.First, segment.Last)
		fmt.Printf("%s: total #block = %v\n", pool.Name, nb)
		fmt.Printf("%s: total #tx    = %v\n", pool.Name, nt)
		fmt.Printf("%s: %.2f blk/s, %.2f tx/s\n", pool.Name, blkPerSec, txPerSec)
		fmt.Printf("%s done in %v\n", pool.Name, duration.Round(1*time.Millisecond))
	}()

	numWorkers := pool.NumWorkers()
	// numProcs = numWorkers + work producer (1) + main thread (1)
	numProcs := numWorkers + 2
	if goMaxProcs := runtime.GOMAXPROCS(0); goMaxProcs < numProcs {
		runtime.GOMAXPROCS(numProcs)
	}

	fmt.Printf("%s: block segment = %v-%v\n", pool.Name, segment.First, segment.Last)
	fmt.Printf("%s: workers = %v\n", pool.Name, numWorkers)

	workChan := make(chan uint64, numWorkers*1000)
	doneChan := make(chan interface{}, numWorkers*1000)
	stopChan := make(chan struct{}, numWorkers)
	wg := sync.WaitGroup{}
	defer func() {
		// stop all workers
		for i := 0; i < numWorkers; i++ {
			stopChan <- struct{}{}
		}
		// stop work producer (1)
		stopChan <- struct{}{}

		wg.Wait()
		close(workChan)
		close(doneChan)
	}()
	// dynamically schedule one block per worker
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		// worker goroutine
		go func() {
			defer wg.Done()

			for {
				select {

				case block := <-workChan:
					nt, err := pool.ExecuteBlock(block)
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

	// wait until all workers finish all tasks
	wg.Add(1)
	go func() {
		defer wg.Done()

		for block := segment.First; block <= segment.Last; block++ {
			select {

			case workChan <- block:
				continue

			case <-stopChan:
				return

			}
		}
	}()

	// Count finished blocks in order and report execution speed
	var lastSec float64
	var lastNumBlock, lastNumTx int64
	waitMap := make(map[uint64]struct{})
	for block := segment.First; block <= segment.Last; {

		// Count finshed blocks from waitMap in order
		if _, ok := waitMap[block]; ok {
			delete(waitMap, block)

			block++
			continue
		}

		duration := time.Since(start) + 1*time.Nanosecond
		sec := duration.Seconds()
		if block == segment.Last ||
			(block%10000 == 0 && sec > lastSec+5) ||
			(block%1000 == 0 && sec > lastSec+10) ||
			(block%100 == 0 && sec > lastSec+20) ||
			(block%10 == 0 && sec > lastSec+40) ||
			(sec > lastSec+60) {
			nb, nt := atomic.LoadInt64(&totalNumBlock), atomic.LoadInt64(&totalNumTx)
			blkPerSec := float64(nb-lastNumBlock) / (sec - lastSec)
			txPerSec := float64(nt-lastNumTx) / (sec - lastSec)
			fmt.Printf("%s: elapsed time: %v, number = %v\n", pool.Name, duration.Round(1*time.Millisecond), block)
			fmt.Printf("%s: %.2f blk/s, %.2f tx/s\n", pool.Name, blkPerSec, txPerSec)

			lastSec, lastNumBlock, lastNumTx = sec, nb, nt
		}

		data := <-doneChan
		switch t := data.(type) {

		case uint64:
			waitMap[data.(uint64)] = struct{}{}

		case error:
			err := data.(error)
			return err

		default:
			panic(fmt.Errorf("%s: unknown type %T value from doneChan", pool.Name, t))

		}
	}

	return nil
}
