package cli

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/twinmind/newo-tool/internal/config"
	"github.com/twinmind/newo-tool/internal/customer"
	"github.com/twinmind/newo-tool/internal/diff"
	"github.com/twinmind/newo-tool/internal/fsutil"
	"github.com/twinmind/newo-tool/internal/state"
	"github.com/twinmind/newo-tool/internal/ui/console"
)

// MergeCommand merges changes from a source project to a target project.
type MergeCommand struct {
	stdout  io.Writer
	stderr  io.Writer
	console *console.Writer

	// Flags
	projectIDN        *string
	sourceCustomerIDN *string
	targetCustomerIDN *string
	noPull            *bool
	noPush            *bool
	force             *bool

	outputRoot string

	promptMu sync.Mutex

	// Command factories for dependency injection in tests.
	pullCmdFactory func(stdout, stderr io.Writer) Command
	pushCmdFactory func(stdout, stderr io.Writer) Command
}

// NewMergeCommand constructs a merge command.
func NewMergeCommand(stdout, stderr io.Writer) *MergeCommand {
	return &MergeCommand{
		stdout:  stdout,
		stderr:  stderr,
		console: console.New(stdout, stderr),

		projectIDN:        new(string),
		sourceCustomerIDN: new(string),
		targetCustomerIDN: new(string),
		noPull:            new(bool),
		noPush:            new(bool),
		force:             new(bool),

		pullCmdFactory: func(stdout, stderr io.Writer) Command { return NewPullCommand(stdout, stderr) },
		pushCmdFactory: func(stdout, stderr io.Writer) Command { return NewPushCommand(stdout, stderr) },
	}
}

func (c *MergeCommand) Name() string {
	return "merge"
}

func (c *MergeCommand) Summary() string {
	return "Merge changes from a source project to a target project"
}

func (c *MergeCommand) RegisterFlags(fs *flag.FlagSet) {
	fs.BoolVar(c.noPull, "no-pull", false, "Skip the initial pull step")
	fs.BoolVar(c.noPush, "no-push", false, "Skip the final push step")
	fs.BoolVar(c.force, "force", false, "Perform copy and push without interactive diff/confirmation")
	fs.StringVar(c.targetCustomerIDN, "target-customer", "", "IDN of the target customer (optional, auto-detects if unambiguous)")
}

func (c *MergeCommand) Run(ctx context.Context, args []string) error {
	prevTarget := ""
	if c.targetCustomerIDN != nil {
		prevTarget = *c.targetCustomerIDN
	}
	prevNoPull := false
	if c.noPull != nil {
		prevNoPull = *c.noPull
	}
	prevNoPush := false
	if c.noPush != nil {
		prevNoPush = *c.noPush
	}
	prevForce := false
	if c.force != nil {
		prevForce = *c.force
	}

	fs := flag.NewFlagSet("merge", flag.ContinueOnError)
	c.RegisterFlags(fs)

	// Preserve any preconfigured flag values (useful in tests that set flags directly on the command).
	if strings.TrimSpace(prevTarget) != "" {
		_ = fs.Set("target-customer", prevTarget)
	}
	if prevNoPull {
		_ = fs.Set("no-pull", "true")
	}
	if prevNoPush {
		_ = fs.Set("no-push", "true")
	}
	if prevForce {
		_ = fs.Set("force", "true")
	}
	if err := fs.Parse(args); err != nil {
		return err
	}

	positionalArgs := fs.Args()

	// Now validate the positional arguments.
	if len(positionalArgs) != 3 || positionalArgs[1] != "from" {
		return fmt.Errorf("usage: newo merge <project_idn> from <source_customer_idn> [flags]")
	}

	projectIDN := strings.TrimSpace(positionalArgs[0])
	sourceCustomerIDN := strings.TrimSpace(positionalArgs[2])
	if projectIDN == "" || sourceCustomerIDN == "" {
		return fmt.Errorf("usage: newo merge <project_idn> from <source_customer_idn> [flags]")
	}

	env, err := config.LoadEnv()
	if err != nil {
		return err
	}
	c.outputRoot = env.OutputRoot

	cfg, err := customer.FromEnv(env)
	if err != nil {
		return err
	}

	sourceEntry, err := c.lookupCustomer(cfg.Entries, sourceCustomerIDN, projectIDN, "source")
	if err != nil {
		return err
	}
	if !strings.EqualFold(sourceEntry.Type, "e2e") {
		return fmt.Errorf("source customer %q must be of type \"e2e\", but got \"%s\"", sourceEntry.HintIDN, sourceEntry.Type)
	}

	var targetEntry *customer.Entry
	targetID := strings.TrimSpace(*c.targetCustomerIDN)
	if targetID != "" {
		targetEntry, err = c.lookupCustomer(cfg.Entries, targetID, projectIDN, "target")
		if err != nil {
			return err
		}
	} else {
		targetEntry, err = c.detectTargetCustomer(cfg.Entries, projectIDN)
		if err != nil {
			return err
		}
	}
	if !strings.EqualFold(targetEntry.Type, "integration") {
		return fmt.Errorf("target customer %q must be of type \"integration\", but got \"%s\"", targetEntry.HintIDN, targetEntry.Type)
	}

	c.console.Section("Merge")
	c.console.Success(
		"Validated project %q: %s → %s",
		projectIDN,
		sourceEntry.HintIDN,
		targetEntry.HintIDN,
	)

	if !*c.noPull {
		c.console.Section("Pull")
		c.console.Info("Synchronising source project %s", sourceEntry.HintIDN)
		if err := c.runPullCommand(ctx, sourceEntry.HintIDN, projectIDN); err != nil {
			return fmt.Errorf("pull for source customer %q failed: %w", sourceEntry.HintIDN, err)
		}
		c.console.Success("Source project refreshed.")
		c.console.Info("Synchronising target project %s", targetEntry.HintIDN)
		if err := c.runPullCommand(ctx, targetEntry.HintIDN, projectIDN); err != nil {
			return fmt.Errorf("pull for target customer %q failed: %w", targetEntry.HintIDN, err)
		}
		c.console.Success("Target project refreshed.")
	} else {
		c.console.Info("Skipping initial pull (--no-pull flag).")
	}

	sourceSlug, err := c.projectSlugFromState(sourceEntry.HintIDN, projectIDN)
	if err != nil {
		return fmt.Errorf("determine source project path: %w", err)
	}
	targetSlug, err := c.projectSlugFromState(targetEntry.HintIDN, projectIDN)
	if err != nil {
		return fmt.Errorf("determine target project path: %w", err)
	}

	sourceProjectDir := fsutil.ExportProjectDir(c.outputRoot, sourceEntry.Type, sourceEntry.HintIDN, sourceSlug)
	targetProjectDir := fsutil.ExportProjectDir(c.outputRoot, targetEntry.Type, targetEntry.HintIDN, targetSlug)

	if _, err := os.Stat(sourceProjectDir); errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("source project directory %q does not exist. Run 'newo pull --customer %s --project-idn %s' first", sourceProjectDir, sourceEntry.HintIDN, projectIDN)
	} else if err != nil {
		return fmt.Errorf("stat source project directory: %w", err)
	}

	if err := os.MkdirAll(targetProjectDir, fsutil.DirPerm); err != nil {
		return fmt.Errorf("ensure target project directory: %w", err)
	}

	c.console.Section("Copy")
	c.console.Info("Source: %s", sourceProjectDir)
	c.console.Info("Target: %s", targetProjectDir)

	c.console.Info("Copying files from source to target...")
	if err := c.copyProjectFiles(sourceProjectDir, targetProjectDir, *c.force); err != nil {
		return fmt.Errorf("failed to copy project files: %w", err)
	}
	c.console.Success("File copy complete.")

	if !*c.noPush {
		c.console.Section("Push")
		c.console.Info("Pushing merged changes to target platform...")
		if err := c.runPushCommand(ctx, targetEntry.HintIDN, *c.force); err != nil {
			return fmt.Errorf("push for target customer %q failed: %w", targetEntry.HintIDN, err)
		}
		c.console.Success("Push complete.")
	} else {
		c.console.Info("Skipping push (--no-push flag).")
	}

	return nil
}

func (c *MergeCommand) runPullCommand(ctx context.Context, customerIDN, projectIDN string) error {
	pullCmd := c.pullCmdFactory(c.stdout, c.stderr)
	fs := flag.NewFlagSet("pull", flag.ContinueOnError)
	pullCmd.RegisterFlags(fs)

	_ = fs.Set("force", "true")
	_ = fs.Set("customer", customerIDN)
	if projectIDN != "" {
		_ = fs.Set("project-idn", projectIDN)
	}

	return pullCmd.Run(ctx, []string{})
}

func (c *MergeCommand) runPushCommand(ctx context.Context, customerIDN string, force bool) error {
	pushCmd := c.pushCmdFactory(c.stdout, c.stderr)
	fs := flag.NewFlagSet("push", flag.ContinueOnError)
	pushCmd.RegisterFlags(fs)

	_ = fs.Set("customer", customerIDN)
	if force {
		_ = fs.Set("force", "true")
	}

	return pushCmd.Run(ctx, []string{})
}

func (c *MergeCommand) copyProjectFiles(sourceDir, targetDir string, force bool) error {
	return filepath.WalkDir(sourceDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		relPath, err := filepath.Rel(sourceDir, path)
		if err != nil {
			return fmt.Errorf("failed to get relative path: %w", err)
		}

		targetPath := filepath.Join(targetDir, relPath)

		if d.IsDir() {
			return os.MkdirAll(targetPath, fsutil.DirPerm)
		}

		sourceContent, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("failed to read source file %q: %w", path, err)
		}

		targetContent, err := os.ReadFile(targetPath)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("failed to read target file %q: %w", targetPath, err)
		}

		if !force && !bytes.Equal(sourceContent, targetContent) {
			lines := diff.Generate(targetContent, sourceContent, 3)
			confirmed, err := c.confirmOverwrite(targetPath, lines)
			if err != nil {
				return err
			}
			if !confirmed {
				c.console.Warn("Skipped %s (not confirmed)", targetPath)
				return nil
			}
		}

		if err := os.WriteFile(targetPath, sourceContent, fsutil.FilePerm); err != nil {
			return fmt.Errorf("failed to write file %q: %w", targetPath, err)
		}
		c.console.Info("Copied %s → %s", path, targetPath)
		return nil
	})
}

func (c *MergeCommand) confirmOverwrite(path string, lines []diff.Line) (bool, error) {
	c.promptMu.Lock()
	defer c.promptMu.Unlock()

	c.console.Write(diff.Format(path, lines))
	c.console.Prompt("Overwrite local file %s? [y/N]: ", path)

	reader := bufio.NewReader(os.Stdin)
	text, err := reader.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return false, fmt.Errorf("read confirmation input: %w", err)
	}

	response := strings.TrimSpace(strings.ToLower(text))
	if response != "y" {
		c.console.Info("Keeping existing file.")
		return false, nil
	}
	return true, nil
}

func (c *MergeCommand) lookupCustomer(entries []customer.Entry, customerIDN, projectIDN, role string) (*customer.Entry, error) {
	customerIDN = strings.TrimSpace(customerIDN)
	projectIDN = strings.TrimSpace(projectIDN)

	var fallback *customer.Entry
	for i := range entries {
		entry := &entries[i]
		if !strings.EqualFold(entry.HintIDN, customerIDN) {
			continue
		}
		if projectIDN == "" || entry.ProjectIDN == "" || strings.EqualFold(entry.ProjectIDN, projectIDN) {
			return entry, nil
		}
		if fallback == nil {
			fallback = entry
		}
	}

	if fallback != nil {
		return fallback, nil
	}

	roleLabel := fmt.Sprintf("%s customer", role)
	if role == "source" || role == "target" {
		roleLabel = fmt.Sprintf("%s customer", role)
	}

	return nil, fmt.Errorf("%s %q not found in configuration", roleLabel, customerIDN)
}

func (c *MergeCommand) detectTargetCustomer(entries []customer.Entry, projectIDN string) (*customer.Entry, error) {
	matches := map[string]*customer.Entry{}
	projectIDN = strings.TrimSpace(projectIDN)

	for i := range entries {
		entry := &entries[i]
		if !strings.EqualFold(entry.Type, "integration") {
			continue
		}
		if projectIDN != "" && entry.ProjectIDN != "" && !strings.EqualFold(entry.ProjectIDN, projectIDN) {
			continue
		}
		key := strings.ToLower(strings.TrimSpace(entry.HintIDN))
		if key == "" {
			continue
		}
		if _, exists := matches[key]; !exists {
			matches[key] = entry
		}
	}

	if len(matches) == 0 {
		return nil, fmt.Errorf("no integration customer found for project %q", projectIDN)
	}

	if len(matches) > 1 {
		ids := make([]string, 0, len(matches))
		for _, entry := range matches {
			ids = append(ids, entry.HintIDN)
		}
		sort.Strings(ids)
		return nil, fmt.Errorf("multiple integration customers found for project %q: %s. Please specify with --target-customer flag", projectIDN, strings.Join(ids, ", "))
	}

	for _, entry := range matches {
		return entry, nil
	}
	return nil, fmt.Errorf("no integration customer found for project %q", projectIDN)
}

func (c *MergeCommand) projectSlugFromState(customerIDN, projectIDN string) (string, error) {
	projectIDN = strings.TrimSpace(projectIDN)
	customerIDN = strings.TrimSpace(customerIDN)

	pm, err := state.LoadProjectMap(customerIDN)
	if err != nil {
		return "", err
	}

	for idn, data := range pm.Projects {
		if strings.EqualFold(idn, projectIDN) {
			slug := strings.TrimSpace(data.Path)
			if slug == "" {
				break
			}
			return slug, nil
		}
	}

	return "", fmt.Errorf("project %q not found in local state for customer %q. Run 'newo pull --customer %s --project-idn %s' first", projectIDN, customerIDN, customerIDN, projectIDN)
}
