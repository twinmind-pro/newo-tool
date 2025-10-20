package cli

import (
	"context"
	"flag"
	"io"

	"github.com/twinmind/newo-tool/internal/ui/console"
	"github.com/twinmind/newo-tool/internal/version"
)

// VersionCommand prints the application's version details.
type VersionCommand struct {
	writer  io.Writer
	console *console.Writer
}

func (c *VersionCommand) Name() string {
	return "version"
}

func (c *VersionCommand) Summary() string {
	return "Show build version and commit"
}

func (c *VersionCommand) RegisterFlags(_ *flag.FlagSet) {}

func (c *VersionCommand) Run(ctx context.Context, _ []string) error {
	if c.console == nil {
		c.console = console.New(c.writer, c.writer)
	}
	c.console.Section("Version")
	c.console.Info("version: %s", version.Version)
	c.console.Info("commit: %s", version.Commit)
	return nil
}
