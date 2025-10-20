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
)

// MergeCommand merges changes from a source project to a target project.
type MergeCommand struct {
	stdout io.Writer
	stderr io.Writer

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
		stdout: stdout,
		stderr: stderr,

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
	// Manually separate flags from positional arguments to allow flags to be placed anywhere.
	var positionalArgs []string
	var flagArgs []string
	for _, arg := range args {
		if strings.HasPrefix(arg, "-") {
			flagArgs = append(flagArgs, arg)
		} else {
			positionalArgs = append(positionalArgs, arg)
		}
	}

	// Parse only the flags.
	fs := flag.NewFlagSet("merge", flag.ContinueOnError)
	c.RegisterFlags(fs)
	if err := fs.Parse(flagArgs); err != nil {
		return err
	}

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

	_, _ = fmt.Fprintf(
		c.stdout,
		"Validation successful: Merging project %q from e2e customer %q to integration customer %q\n",
		projectIDN,
		sourceEntry.HintIDN,
		targetEntry.HintIDN,
	)

	if !*c.noPull {
		_, _ = fmt.Fprintln(c.stdout, "Performing initial pull for source and target projects...")
		if err := c.runPullCommand(ctx, sourceEntry.HintIDN, projectIDN); err != nil {
			return fmt.Errorf("pull for source customer %q failed: %w", sourceEntry.HintIDN, err)
		}
		if err := c.runPullCommand(ctx, targetEntry.HintIDN, projectIDN); err != nil {
			return fmt.Errorf("pull for target customer %q failed: %w", targetEntry.HintIDN, err)
		}
		_, _ = fmt.Fprintln(c.stdout, "Initial pull complete.")
	} else {
		_, _ = fmt.Fprintln(c.stdout, "Skipping initial pull step (--no-pull flag is set).")
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

	_, _ = fmt.Fprintf(c.stdout, "Source project directory: %q\n", sourceProjectDir)
	_, _ = fmt.Fprintf(c.stdout, "Target project directory: %q\n", targetProjectDir)

	_, _ = fmt.Fprintln(c.stdout, "Copying files from source to target...")
	if err := c.copyProjectFiles(sourceProjectDir, targetProjectDir, *c.force); err != nil {
		return fmt.Errorf("failed to copy project files: %w", err)
	}
	_, _ = fmt.Fprintln(c.stdout, "File copying complete.")

	if !*c.noPush {
		_, _ = fmt.Fprintln(c.stdout, "Pushing merged changes to target platform...")
		if err := c.runPushCommand(ctx, targetEntry.HintIDN, *c.force); err != nil {
			return fmt.Errorf("push for target customer %q failed: %w", targetEntry.HintIDN, err)
		}
		_, _ = fmt.Fprintln(c.stdout, "Push complete.")
	} else {
		_, _ = fmt.Fprintln(c.stdout, "Skipping push step (--no-push flag is set).")
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
				_, _ = fmt.Fprintf(c.stdout, "Skipping %q (not confirmed by user).\n", targetPath)
				return nil
			}
		}

		if err := os.WriteFile(targetPath, sourceContent, fsutil.FilePerm); err != nil {
			return fmt.Errorf("failed to write file %q: %w", targetPath, err)
		}
		_, _ = fmt.Fprintf(c.stdout, "Copied %q to %q\n", path, targetPath)
		return nil
	})
}

func (c *MergeCommand) confirmOverwrite(path string, lines []diff.Line) (bool, error) {
	c.promptMu.Lock()
	defer c.promptMu.Unlock()

	_, _ = fmt.Fprint(c.stdout, diff.Format(path, lines))
	_, _ = fmt.Fprintf(c.stdout, "Overwrite local file %s? [y/N]: ", path)

	reader := bufio.NewReader(os.Stdin)
	text, err := reader.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return false, fmt.Errorf("read confirmation input: %w", err)
	}

	response := strings.TrimSpace(strings.ToLower(text))
	if response != "y" {
		_, _ = fmt.Fprintln(c.stdout, "Skipping overwrite.")
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
