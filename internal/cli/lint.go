package cli

import (
	"context"
	"flag"
	"fmt"
	"io"

	"github.com/twinmind/newo-tool/internal/linter"
	"github.com/twinmind/newo-tool/internal/ui/console"
)

// LintCommand performs linting on .nsl files.
type LintCommand struct {
	stdout  io.Writer
	stderr  io.Writer
	console *console.Writer
}

// NewLintCommand constructs a lint command.
func NewLintCommand(stdout, stderr io.Writer) *LintCommand {
	return &LintCommand{
		stdout:  stdout,
		stderr:  stderr,
		console: console.New(stdout, stderr),
	}
}

func (c *LintCommand) ensureConsole() {
	if c.console == nil {
		c.console = console.New(c.stdout, c.stderr)
	}
}

func (c *LintCommand) Name() string {
	return "lint"
}

func (c *LintCommand) Summary() string {
	return "Lint .nsl files in downloaded projects"
}

func (c *LintCommand) RegisterFlags(_ *flag.FlagSet) {
	// No flags for the basic version
}

func (c *LintCommand) Run(ctx context.Context, _ []string) error {
	c.ensureConsole()
	c.console.Section("Lint")

	outputRoot, exists, err := findTargetDir()
	if err != nil {
		return err
	}
	if !exists {
		c.console.Info("Directory %q does not exist. Nothing to lint.", outputRoot)
		return nil
	}

	c.console.Info("Linting .nsl files in %s...", outputRoot)

	errors, err := linter.LintNSLFiles(outputRoot)
	if err != nil {
		return fmt.Errorf("error during linting: %w", err)
	}

	if len(errors) > 0 {
		for _, e := range errors {
			c.console.Warn("%s", e.Error())
		}
		return fmt.Errorf("%d total linting issues found", len(errors))
	}

	c.console.Success("No linting issues found.")
	return nil
}
