package cli

import (
	"context"
	"flag"
)

// Command is an interface for CLI commands.
type Command interface {
	Name() string
	Summary() string
	RegisterFlags(fs *flag.FlagSet)
	Run(ctx context.Context, args []string) error
}
