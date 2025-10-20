package cli

import (
	"bufio"
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
	fix      *bool
	input    io.Reader
}

// NewLintCommand constructs a lint command.
func NewLintCommand(stdout, stderr io.Writer) *LintCommand {
	return &LintCommand{
		stdout:  stdout,
		stderr:  stderr,
		console: console.New(stdout, stderr),
		input:   os.Stdin,
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
	c.fix = fs.Bool("fix", false, "interactively fix supported lint warnings")
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

	fixRequested := c.fix != nil && *c.fix
	if fixRequested {
		if file, ok := c.input.(*os.File); !ok || !isTerminalFile(file) {
			return fmt.Errorf("--fix requires an interactive terminal")
		}
	}

	grouped, totalErrors, totalWarnings, err := c.collectIssues(dirs, true)
	if err != nil {
		return err
	}

	if fixRequested {
		modified, err := c.applyFixes(grouped)
		if err != nil {
			return err
		}
		if modified {
			grouped, totalErrors, totalWarnings, err = c.collectIssues(dirs, false)
			if err != nil {
				return err
			}
		} else {
			totalErrors, totalWarnings = countIssues(grouped)
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

func (c *LintCommand) collectIssues(dirs []string, log bool) (map[string][]linter.LintError, int, int, error) {
	grouped := make(map[string][]linter.LintError)
	visitedRoots := make(map[string]struct{})
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
			return nil, 0, 0, fmt.Errorf("stat %s: %w", root, statErr)
		}
		if !info.IsDir() {
			continue
		}

		if log {
			c.console.Info("Linting .nsl files in %s...", root)
		}

		lintErrors, lintErr := linter.LintNSLFiles(root)
		if lintErr != nil {
			return nil, 0, 0, fmt.Errorf("error during linting: %w", lintErr)
		}

		for _, issue := range lintErrors {
			canonical := filepath.ToSlash(filepath.Clean(issue.FilePath))
			issue.FilePath = canonical
			grouped[canonical] = append(grouped[canonical], issue)
			if issue.Severity == linter.SeverityWarning {
				totalWarnings++
			} else {
				totalErrors++
			}
		}
	}

	return grouped, totalErrors, totalWarnings, nil
}

func (c *LintCommand) applyFixes(grouped map[string][]linter.LintError) (bool, error) {
	reader := bufio.NewReader(c.input)
	applyAll := false
	modified := false

	files := make([]string, 0, len(grouped))
	for file := range grouped {
		files = append(files, file)
	}
	sort.Strings(files)

	for _, file := range files {
		display := displayLintPath(file)
		issues := grouped[file]

		for _, issue := range issues {
			if !isFixableIssue(issue) {
				continue
			}

			if !applyAll {
				c.console.Info("Fix %s (line %d): %s", display, issue.Line, issue.Message)
				c.console.Prompt("Apply fix? [y/N/a]: ")
				response, err := reader.ReadString('\n')
				if err != nil {
					return modified, fmt.Errorf("read input: %w", err)
				}
				decision := strings.ToLower(strings.TrimSpace(response))
				switch decision {
				case "y":
					// proceed
				case "a":
					applyAll = true
				default:
					c.console.Info("Skipped.")
					continue
				}
			}

			changed, err := fixNSLComment(issue)
			if err != nil {
				c.console.Warn("Failed to fix %s:%d: %v", display, issue.Line, err)
				continue
			}
			if changed {
				modified = true
				c.console.Success("Fixed %s:%d", display, issue.Line)
			}
		}
	}

	return modified, nil
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
		display := displayLintPath(file)
		writer.Section(display)

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

func countIssues(grouped map[string][]linter.LintError) (int, int) {
	errorsCount := 0
	warningsCount := 0
	for _, issues := range grouped {
		for _, issue := range issues {
			if issue.Severity == linter.SeverityWarning {
				warningsCount++
			} else {
				errorsCount++
			}
		}
	}
	return errorsCount, warningsCount
}

func isFixableIssue(issue linter.LintError) bool {
	if issue.Message != "Line contains an NSL comment" {
		return false
	}
	trimmed := strings.TrimSpace(issue.Snippet)
	if trimmed == "" {
		return false
	}
	return strings.Contains(trimmed, "{#") || strings.Contains(trimmed, "#}")
}

func fixNSLComment(issue linter.LintError) (bool, error) {
	path := filepath.FromSlash(issue.FilePath)
	info, err := os.Stat(path)
	if err != nil {
		return false, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return false, err
	}

	lines := strings.SplitAfter(string(data), "\n")
	if issue.Line <= 0 || issue.Line > len(lines) {
		return false, fmt.Errorf("line %d out of range", issue.Line)
	}

	idx := issue.Line - 1
	original := lines[idx]
	trimmed := strings.TrimSpace(original)

	changed := false

	if strings.HasPrefix(trimmed, "{#") && strings.HasSuffix(trimmed, "#}") {
		lines = append(lines[:idx], lines[idx+1:]...)
		changed = true
	} else {
		lineContent := original
		newline := ""
		if strings.HasSuffix(lineContent, "\n") {
			newline = "\n"
			lineContent = strings.TrimSuffix(lineContent, "\n")
		}

		start := strings.Index(lineContent, "{#")
		end := strings.Index(lineContent, "#}")

		switch {
		case start >= 0 && end >= 0 && end >= start:
			lineContent = lineContent[:start] + lineContent[end+2:]
			changed = true
		case start >= 0:
			lineContent = strings.TrimRight(lineContent[:start], " \t")
			changed = true
		case end >= 0:
			lineContent = strings.TrimRight(lineContent[:end], " \t")
			changed = true
		}

		if changed {
			lines[idx] = lineContent + newline
		}
	}

	if !changed {
		return false, nil
	}

	result := strings.Join(lines, "")
	if err := os.WriteFile(path, []byte(result), info.Mode().Perm()); err != nil {
		return false, fmt.Errorf("write %s: %w", path, err)
	}
	return true, nil
}

func isTerminalFile(f *os.File) bool {
	if f == nil {
		return false
	}
	info, err := f.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}
