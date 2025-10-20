package main

import (
	"context"
	"fmt"
	"os"

	"github.com/twinmind/newo-tool/internal/cli"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		exitCode := 1
		if coder, ok := err.(interface{ ExitCode() int }); ok {
			exitCode = coder.ExitCode()
		}
		if silent, ok := err.(interface{ Silent() bool }); ok && silent.Silent() {
			os.Exit(exitCode)
		}
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(exitCode)
	}
}

func run(args []string) error {
	app := cli.New(os.Stdout, os.Stderr)

	return app.Execute(context.Background(), args)
}
