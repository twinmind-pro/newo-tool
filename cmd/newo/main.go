package main

import (
	"context"
	"fmt"
	"os"

	"github.com/twinmind/newo-tool/internal/cli"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	app := cli.New(os.Stdout, os.Stderr)

	return app.Execute(context.Background(), args)
}
