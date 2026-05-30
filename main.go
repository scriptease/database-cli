package main

import (
	"os"

	"github.com/scriptease/jdbc-cli/internal/cli"
	"github.com/scriptease/jdbc-cli/internal/daemon"
	"github.com/scriptease/jdbc-cli/internal/jsonerror"
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
