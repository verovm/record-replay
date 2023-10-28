package main

import (
	"fmt"
	"os"

	"github.com/ethereum/go-ethereum/cmd/substate-cli/db"
	"github.com/ethereum/go-ethereum/cmd/substate-cli/replay"
	"github.com/ethereum/go-ethereum/internal/flags"
	cli "github.com/urfave/cli/v2"
)

var (
	app = flags.NewApp("Ethereum substate command line interface")
)

func init() {
	app.Flags = []cli.Flag{}
	app.Commands = []*cli.Command{
		replay.ReplayCommand,
		replay.ReplayForkCommand,
		db.UpgradeCommand,
		db.CloneCommand,
		db.CompactCommand,
	}
}

func main() {
	if err := app.Run(os.Args); err != nil {
		code := 1
		fmt.Fprintln(os.Stderr, err)
		os.Exit(code)
	}
}
