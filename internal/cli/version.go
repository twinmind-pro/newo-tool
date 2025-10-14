package cli

import (
	"context"
	"flag"
	"fmt"
	"io"

	"github.com/twinmind/newo-tool/internal/version"
)

// VersionCommand prints the application's version details.
type VersionCommand struct {
	writer io.Writer
}

func (c *VersionCommand) Name() string {
	return "version"
}

func (c *VersionCommand) Summary() string {
	return "Show build version and commit"
}

func (c *VersionCommand) RegisterFlags(_ *flag.FlagSet) {}

func (c *VersionCommand) Run(ctx context.Context, _ []string) error {
	_, err := fmt.Fprintf(c.writer, "version: %s\ncommit: %s\n", version.Version, version.Commit)
	return err
}
