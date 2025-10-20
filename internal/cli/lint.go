package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/twinmind/newo-tool/internal/linter"
	"github.com/twinmind/newo-tool/internal/ui/console"
)

// LintCommand performs linting on .nsl files.
type LintCommand struct {
	stdout   io.Writer
	stderr   io.Writer
	console  *console.Writer
	customer *string
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

func (c *LintCommand) RegisterFlags(fs *flag.FlagSet) {
	c.customer = fs.String("customer", "", "customer IDN to lint")
}

func (c *LintCommand) Run(ctx context.Context, _ []string) error {
	c.ensureConsole()
	c.console.Section("Lint")

	outputRoot, err := getOutputRoot()
	if err != nil {
		return err
	}
	if outputRoot == "" {
		outputRoot = "."
	}

	if _, err := os.Stat(outputRoot); errors.Is(err, os.ErrNotExist) {
		c.console.Info("Directory %q does not exist. Nothing to lint.", outputRoot)
		return nil
	}

	filter := ""
	if c.customer != nil {
		filter = strings.TrimSpace(*c.customer)
	}
	dirs, resolvedIDN, missingState, err := resolveCustomerDirectories(outputRoot, filter)
	if err != nil {
		return err
	}
	if missingState {
		id := strings.TrimSpace(resolvedIDN)
		if id == "" {
			id = filter
		}
		c.console.Info("No project map for %s. Run `newo pull --customer %s` first.", id, id)
		return nil
	}
	if len(dirs) == 0 {
		c.console.Success("No linting issues found.")
		return nil
	}

	visitedRoots := make(map[string]struct{})
	grouped := make(map[string][]linter.LintError)
	totalErrors := 0
	totalWarnings := 0

	for _, dir := range dirs {
		root := filepath.Clean(dir)
		if _, seen := visitedRoots[root]; seen {
			continue
		}
		visitedRoots[root] = struct{}{}

		info, statErr := os.Stat(root)
		if statErr != nil {
			if errors.Is(statErr, os.ErrNotExist) {
				continue
			}
			return fmt.Errorf("stat %s: %w", root, statErr)
		}
		if !info.IsDir() {
			continue
		}

		c.console.Info("Linting .nsl files in %s...", root)

		lintErrors, lintErr := linter.LintNSLFiles(root)
		if lintErr != nil {
			return fmt.Errorf("error during linting: %w", lintErr)
		}

		for _, issue := range lintErrors {
			displayPath := displayLintPath(issue.FilePath)
			issue.FilePath = displayPath
			grouped[displayPath] = append(grouped[displayPath], issue)
			if issue.Severity == linter.SeverityWarning {
				totalWarnings++
			} else {
				totalErrors++
			}
		}
	}

	if totalErrors == 0 && totalWarnings == 0 {
		c.console.Success("No linting issues found.")
		return nil
	}

	printLintReport(c.console, grouped)

	summary := fmt.Sprintf("Summary: %d file(s) with issues | %d error(s) | %d warning(s)", len(grouped), totalErrors, totalWarnings)
	if totalErrors > 0 {
		c.console.Warn("%s", summary)
	} else {
		c.console.Info("%s", summary)
	}

	return newSilentExitError(1)
}

func displayLintPath(path string) string {
	cleaned := filepath.Clean(path)
	if rel, err := filepath.Rel(".", cleaned); err == nil {
		return filepath.ToSlash(rel)
	}
	return filepath.ToSlash(cleaned)
}

func printLintReport(writer *console.Writer, grouped map[string][]linter.LintError) {
	if len(grouped) == 0 {
		return
	}

	files := make([]string, 0, len(grouped))
	for file := range grouped {
		files = append(files, file)
	}
	sort.Strings(files)

	severityRank := map[linter.Severity]int{
		linter.SeverityError:   0,
		linter.SeverityWarning: 1,
	}

	colorEnabled := writer.ColorsEnabled()

	const (
		ansiReset  = "\033[0m"
		ansiYellow = "\033[33m"
		ansiRed    = "\033[31m"
	)

	for idx, file := range files {
		if idx > 0 {
			writer.RawLine("")
		}
		writer.Section(file)

		issues := grouped[file]
		sort.SliceStable(issues, func(i, j int) bool {
			if severityRank[issues[i].Severity] != severityRank[issues[j].Severity] {
				return severityRank[issues[i].Severity] < severityRank[issues[j].Severity]
			}
			return issues[i].Line < issues[j].Line
		})

		for _, issue := range issues {
			line := "-"
			if issue.Line > 0 {
				line = fmt.Sprintf("%d", issue.Line)
			}

			formatted := fmt.Sprintf("  line %-4s | %-7s | %s", line, issue.Severity, issue.Message)
			if colorEnabled {
				switch issue.Severity {
				case linter.SeverityWarning:
					formatted = ansiYellow + formatted + ansiReset
				case linter.SeverityError:
					formatted = ansiRed + formatted + ansiReset
				}
			}
			writer.RawLine("%s", formatted)

			snippet := strings.TrimSpace(issue.Snippet)
			if snippet != "" {
				writer.RawLine("    > %s", snippet)
			}
		}
	}
}
