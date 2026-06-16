package main

import (
	"os"

	"github.com/scriptease/database-cli/internal/cli"
	"github.com/scriptease/database-cli/internal/daemon"
	"github.com/scriptease/database-cli/internal/jsonerror"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		jsonerror.Write(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) > 0 && args[0] == "daemon" {
		return daemon.Run(args[1:])
	}
	return cli.Run(args)
}
