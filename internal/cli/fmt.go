package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/twinmind/newo-tool/internal/formatter"
	"github.com/twinmind/newo-tool/internal/ui/console"
)

// FmtCommand performs formatting on .nsl files.
type FmtCommand struct {
	stdout  io.Writer
	stderr  io.Writer
	console *console.Writer
}

// NewFmtCommand constructs a fmt command.
func NewFmtCommand(stdout, stderr io.Writer) *FmtCommand {
	return &FmtCommand{
		stdout:  stdout,
		stderr:  stderr,
		console: console.New(stdout, stderr),
	}
}

func (c *FmtCommand) ensureConsole() {
	if c.console == nil {
		c.console = console.New(c.stdout, c.stderr)
	}
}

func (c *FmtCommand) Name() string {
	return "fmt"
}

func (c *FmtCommand) Summary() string {
	return "Format .nsl files in downloaded projects"
}

func (c *FmtCommand) RegisterFlags(_ *flag.FlagSet) {
	// No flags for the basic version
}

func (c *FmtCommand) Run(ctx context.Context, _ []string) error {
	c.ensureConsole()
	c.console.Section("Format")
	outputRoot, err := getOutputRoot()
	if err != nil {
		return err
	}

	if outputRoot == "" {
		outputRoot = "."
	}

	if _, err := os.Stat(outputRoot); os.IsNotExist(err) {
		c.console.Info("Directory %q does not exist. Nothing to format.", outputRoot)
		return nil
	}

	var formattedFiles []string
	var formatErrors []error

	err = filepath.WalkDir(outputRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && strings.HasSuffix(d.Name(), ".nsl") {
			formatted, err := formatter.FormatNSLFile(path)
			if err != nil {
				// Report error but continue formatting other files
				formatErrors = append(formatErrors, fmt.Errorf("failed to format %s: %w", path, err))
			}
			if formatted {
				formattedFiles = append(formattedFiles, path)
			}
		}
		return nil
	})

	if err != nil {
		return fmt.Errorf("error walking directory for formatting: %w", err)
	}

	if len(formatErrors) > 0 {
		for _, e := range formatErrors {
			c.console.Warn("%s", e.Error())
		}
		return errors.Join(formatErrors...)
	}

	if len(formattedFiles) == 0 {
		c.console.Info("No files to format.")
		return nil
	}

	for _, path := range formattedFiles {
		c.console.Info("Formatted %s", path)
	}

	return nil
}
