package cli

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
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

type lintTomlConfig struct {
	Defaults struct {
		OutputRoot *string `toml:"output_root"`
	} `toml:"defaults"`
}

func (c *LintCommand) getOutputRoot() (string, error) {
	// 1. Check environment variable
	if root := strings.TrimSpace(os.Getenv("NEWO_OUTPUT_ROOT")); root != "" {
		return root, nil
	}

	// 2. Check newo.toml
	path := filepath.Join(".", "newo.toml")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			// 3. Fallback to default
			return "newo_customers", nil
		}
		return "", fmt.Errorf("read newo.toml: %w", err)
	}

	var cfg lintTomlConfig
	if err := toml.Unmarshal(data, &cfg); err != nil {
		return "", fmt.Errorf("parse newo.toml: %w", err)
	}

	if cfg.Defaults.OutputRoot != nil {
		return strings.TrimSpace(*cfg.Defaults.OutputRoot), nil
	}

	// 3. Fallback to default
	return "newo_customers", nil
}

func (c *LintCommand) Run(ctx context.Context, _ []string) error {
	outputRoot, err := c.getOutputRoot()
	if err != nil {
		return err
	}

	if outputRoot == "" {
		outputRoot = "."
	}

	_, _ = fmt.Fprintf(c.stdout, "Linting .nsl files in %s...\n", outputRoot)

	if _, err := os.Stat(outputRoot); os.IsNotExist(err) {
		_, _ = fmt.Fprintf(c.stdout, "Directory %q does not exist. Nothing to lint.\n", outputRoot)
		return nil
	}

	errors, err := linter.LintNSLFiles(outputRoot)
	if err != nil {
		return fmt.Errorf("error during linting: %w", err)
	}

	if len(errors) > 0 {
		for _, e := range errors {
			_, _ = fmt.Fprintln(c.stderr, e.Error())
		}
		return fmt.Errorf("%d linting errors found", len(errors))
	}

	_, _ = fmt.Fprintln(c.stdout, "No linting errors found.")
	return nil
}
