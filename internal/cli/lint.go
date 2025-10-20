package cli

import (
	"context"
	"flag"
	"fmt"
	"io"

	"github.com/twinmind/newo-tool/internal/linter"
)

// LintCommand performs linting on .nsl files.
type LintCommand struct {
	stdout io.Writer
	stderr io.Writer
}

// NewLintCommand constructs a lint command.
func NewLintCommand(stdout, stderr io.Writer) *LintCommand {
	return &LintCommand{stdout: stdout, stderr: stderr}
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
	outputRoot, err := findTargetDir(c.stdout)
	if err != nil {
		return err
	}

	_, _ = fmt.Fprintf(c.stdout, "Linting .nsl files in %s...\n", outputRoot)

	errors, err := linter.LintNSLFiles(outputRoot)
	if err != nil {
		return fmt.Errorf("error during linting: %w", err)
	}

	if len(errors) > 0 {
		for _, e := range errors {
			_, _ = fmt.Fprintln(c.stderr, e.Error())
		}
		return fmt.Errorf("%d total linting issues found", len(errors))
	}

	_, _ = fmt.Fprintln(c.stdout, "No linting issues found.")
	return nil
}
