package cli

import (
	"context"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/twinmind/newo-tool/internal/formatter"
)

// FmtCommand performs formatting on .nsl files.
type FmtCommand struct {
	stdout io.Writer
	stderr io.Writer
}

// NewFmtCommand constructs a fmt command.
func NewFmtCommand(stdout, stderr io.Writer) *FmtCommand {
	return &FmtCommand{stdout: stdout, stderr: stderr}
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
	outputRoot, err := getOutputRoot()
	if err != nil {
		return err
	}

	if outputRoot == "" {
		outputRoot = "."
	}

	if _, err := os.Stat(outputRoot); os.IsNotExist(err) {
		_, _ = fmt.Fprintf(c.stdout, "Directory %q does not exist. Nothing to format.\n", outputRoot)
		return nil
	}

	var formattedFiles []string

	err = filepath.WalkDir(outputRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && strings.HasSuffix(d.Name(), ".nsl") {
			formatted, err := formatter.FormatNSLFile(path)
			if err != nil {
				// Report error but continue formatting other files
				_, _ = fmt.Fprintf(c.stderr, "Error formatting %s: %v\n", path, err)
			}
			if formatted {
				formattedFiles = append(formattedFiles, path)
			}
		}
		return nil
	})

	if err != nil {
		return fmt.Errorf("error during formatting: %w", err)
	}

	if len(formattedFiles) == 0 {
		_, _ = fmt.Fprintln(c.stdout, "No files to format.")
		return nil
	}

	for _, path := range formattedFiles {
		_, _ = fmt.Fprintln(c.stdout, path)
	}

	return nil
}
