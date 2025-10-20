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

	"encoding/json"

	"github.com/twinmind/newo-tool/internal/config"
	"github.com/twinmind/newo-tool/internal/customer"
	"github.com/twinmind/newo-tool/internal/diff"
	"github.com/twinmind/newo-tool/internal/fsutil"
	"github.com/twinmind/newo-tool/internal/state"
	"github.com/twinmind/newo-tool/internal/ui/console"
	"gopkg.in/yaml.v3"
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

func (c *MergeCommand) ensureConsole() {
	if c.console == nil {
		c.console = console.New(c.stdout, c.stderr)
	}
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
	keep := make(map[string]struct{})
	if err := filepath.WalkDir(sourceDir, func(path string, d fs.DirEntry, err error) error {
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

		keep[relPath] = struct{}{}
		sourceForCompare := sourceContent
		targetForCompare := targetContent
		writeContent := sourceContent

		switch {
		case strings.HasSuffix(path, ".meta.yaml"):
			sanitizedSource := canonicalizeSkillMeta(stripSkillMetaID(sourceContent))
			sanitizedTarget := canonicalizeSkillMeta(stripSkillMetaID(targetContent))
			targetID := extractSkillMetaID(targetContent)

			sourceForCompare = sanitizedSource
			targetForCompare = sanitizedTarget

			if targetID != "" {
				writeContent = prependSkillMetaID(targetID, sanitizedSource)
			} else {
				writeContent = ensureTrailingNewline(sanitizedSource)
			}
		case strings.HasSuffix(path, "metadata.yaml"):
			sanitizedSource := removeFlowStateFieldIDs(canonicalizeFlowMetadata(stripFlowMetaID(sourceContent)))
			sanitizedTarget := removeFlowStateFieldIDs(canonicalizeFlowMetadata(stripFlowMetaID(targetContent)))
			targetID := extractFlowMetaID(targetContent)
			targetFieldIDs := extractFlowStateFieldIDs(targetContent)

			sourceForCompare = sanitizedSource
			targetForCompare = sanitizedTarget

			bodyWithIDs := applyFlowStateFieldIDs(sanitizedSource, targetFieldIDs)
			if targetID != "" {
				writeContent = prependFlowMetaID(targetID, bodyWithIDs)
			} else {
				writeContent = ensureTrailingNewline(bodyWithIDs)
			}
		case strings.HasSuffix(path, "project.json"):
			sanitizedSource, sourceIDs := canonicalizeProjectJSON(sourceContent)
			sanitizedTarget, targetIDs := canonicalizeProjectJSON(targetContent)

			sourceForCompare = sanitizedSource
			targetForCompare = sanitizedTarget

			restoreIDs := targetIDs
			if len(restoreIDs) == 0 {
				restoreIDs = sourceIDs
			}
			writeContent = applyProjectIDs(sanitizedSource, restoreIDs)
		}

		if !force && !bytes.Equal(sourceForCompare, targetForCompare) {
			lines := diff.Generate(targetForCompare, sourceForCompare, 3)
			confirmed, applyAll, err := c.confirmOverwrite(targetPath, lines)
			if err != nil {
				return err
			}
			if applyAll {
				force = true
			}
			if !confirmed {
				c.console.Warn("Skipped %s (not confirmed)", targetPath)
				return nil
			}
		}

		if err := os.WriteFile(targetPath, writeContent, fsutil.FilePerm); err != nil {
			return fmt.Errorf("failed to write file %q: %w", targetPath, err)
		}
		c.console.Info("Copied %s → %s", path, targetPath)
		return nil
	}); err != nil {
		return err
	}
	return c.removeStaleFiles(targetDir, keep, force)
}

func (c *MergeCommand) confirmOverwrite(path string, lines []diff.Line) (bool, bool, error) {
	c.promptMu.Lock()
	defer c.promptMu.Unlock()

	c.ensureConsole()
	c.console.Write(diff.Format(path, lines))
	c.console.Prompt("Overwrite local file %s? [y/N/a]: ", path)

	reader := bufio.NewReader(os.Stdin)
	text, err := reader.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return false, false, fmt.Errorf("read confirmation input: %w", err)
	}

	response := strings.TrimSpace(strings.ToLower(text))
	switch response {
	case "y":
		return true, false, nil
	case "a":
		if c.force != nil {
			*c.force = true
		}
		c.console.Info("Applying overwrite to all subsequent files.")
		return true, true, nil
	default:
		c.console.Info("Keeping existing file.")
		return false, false, nil
	}
}

func (c *MergeCommand) removeStaleFiles(targetDir string, keep map[string]struct{}, force bool) error {
	removeAll := force
	return filepath.WalkDir(targetDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(targetDir, path)
		if err != nil {
			return fmt.Errorf("stale-file rel path: %w", err)
		}
		if _, ok := keep[rel]; ok {
			return nil
		}
		remove := removeAll
		if !removeAll {
			confirmed, applyAll, err := c.confirmRemoval(path)
			if err != nil {
				return err
			}
			if applyAll {
				removeAll = true
				remove = true
			} else {
				remove = confirmed
			}
		}
		if !remove {
			return nil
		}
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("remove stale file %q: %w", path, err)
		}
		c.console.Info("Removed %s (not present in source).", path)
		return nil
	})
}

func (c *MergeCommand) confirmRemoval(path string) (bool, bool, error) {
	c.promptMu.Lock()
	defer c.promptMu.Unlock()

	c.ensureConsole()
	c.console.Prompt("Remove local file %s? [y/N/a]: ", path)

	reader := bufio.NewReader(os.Stdin)
	text, err := reader.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return false, false, fmt.Errorf("read confirmation input: %w", err)
	}

	switch strings.TrimSpace(strings.ToLower(text)) {
	case "y":
		return true, false, nil
	case "a":
		if c.force != nil {
			*c.force = true
		}
		c.console.Info("Applying removal to all subsequent files.")
		return true, true, nil
	default:
		c.console.Info("Keeping local file.")
		return false, false, nil
	}
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

func stripSkillMetaID(content []byte) []byte {
	if len(content) == 0 {
		return content
	}
	lines := strings.Split(string(content), "\n")
	trimmed := make([]string, 0, len(lines))
	removed := false
	for _, line := range lines {
		t := strings.TrimSpace(line)
		if !removed && strings.HasPrefix(t, "id:") && !strings.HasPrefix(t, "idn:") {
			removed = true
			continue
		}
		trimmed = append(trimmed, line)
	}
	for len(trimmed) > 0 && strings.TrimSpace(trimmed[0]) == "" {
		trimmed = trimmed[1:]
	}
	return []byte(strings.Join(trimmed, "\n"))
}

func extractSkillMetaID(content []byte) string {
	if len(content) == 0 {
		return ""
	}
	lines := strings.Split(string(content), "\n")
	for _, line := range lines {
		t := strings.TrimSpace(line)
		if strings.HasPrefix(t, "id:") && !strings.HasPrefix(t, "idn:") {
			value := strings.TrimSpace(strings.TrimPrefix(t, "id:"))
			return strings.Trim(value, "\"")
		}
	}
	return ""
}

func prependSkillMetaID(id string, body []byte) []byte {
	cleaned := strings.TrimLeft(string(body), "\n")
	if strings.TrimSpace(id) == "" {
		return ensureTrailingNewline([]byte(cleaned))
	}
	if cleaned != "" && !strings.HasSuffix(cleaned, "\n") {
		cleaned += "\n"
	}
	return []byte(fmt.Sprintf("id: %s\n%s", id, cleaned))
}

func ensureTrailingNewline(body []byte) []byte {
	cleaned := strings.TrimLeft(string(body), "\n")
	if cleaned == "" {
		return []byte{}
	}
	if !strings.HasSuffix(cleaned, "\n") {
		cleaned += "\n"
	}
	return []byte(cleaned)
}

func canonicalizeSkillMeta(body []byte) []byte {
	trimmed := strings.TrimLeft(string(body), "\n")
	if strings.TrimSpace(trimmed) == "" {
		return []byte{}
	}

	var node yaml.Node
	if err := yaml.Unmarshal([]byte(trimmed), &node); err != nil {
		return []byte(trimmed)
	}

	root := &node
	if node.Kind == yaml.DocumentNode && len(node.Content) > 0 {
		root = node.Content[0]
	}

	if root.Kind == yaml.MappingNode {
		for i := 0; i < len(root.Content)-1; i += 2 {
			key := root.Content[i]
			value := root.Content[i+1]
			if key.Value == "parameters" && value.Kind == yaml.SequenceNode {
				sort.SliceStable(value.Content, func(a, b int) bool {
					return skillParameterName(value.Content[a]) < skillParameterName(value.Content[b])
				})
			}
		}
	}

	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(&node); err == nil {
		_ = enc.Close()
		return []byte(strings.TrimLeft(buf.String(), "\n"))
	}
	_ = enc.Close()
	return []byte(trimmed)
}

func skillParameterName(node *yaml.Node) string {
	if node == nil {
		return ""
	}
	if node.Kind == yaml.MappingNode {
		for i := 0; i < len(node.Content)-1; i += 2 {
			key := node.Content[i]
			if key.Value == "name" {
				return node.Content[i+1].Value
			}
		}
	}
	if node.Kind == yaml.DocumentNode && len(node.Content) > 0 {
		return skillParameterName(node.Content[0])
	}
	return ""
}

func stripFlowMetaID(content []byte) []byte {
	if len(content) == 0 {
		return content
	}
	lines := strings.Split(string(content), "\n")
	trimmed := make([]string, 0, len(lines))
	removed := false
	for _, line := range lines {
		t := strings.TrimSpace(line)
		if !removed && strings.HasPrefix(t, "id:") && !strings.HasPrefix(t, "idn:") {
			removed = true
			continue
		}
		trimmed = append(trimmed, line)
	}
	for len(trimmed) > 0 && strings.TrimSpace(trimmed[0]) == "" {
		trimmed = trimmed[1:]
	}
	return []byte(strings.Join(trimmed, "\n"))
}

func extractFlowMetaID(content []byte) string {
	if len(content) == 0 {
		return ""
	}
	lines := strings.Split(string(content), "\n")
	for _, line := range lines {
		t := strings.TrimSpace(line)
		if strings.HasPrefix(t, "id:") && !strings.HasPrefix(t, "idn:") {
			value := strings.TrimSpace(strings.TrimPrefix(t, "id:"))
			return strings.Trim(value, "\"")
		}
	}
	return ""
}

func prependFlowMetaID(id string, body []byte) []byte {
	cleaned := strings.TrimLeft(string(body), "\n")
	if strings.TrimSpace(id) == "" {
		return ensureTrailingNewline([]byte(cleaned))
	}
	if cleaned != "" && !strings.HasSuffix(cleaned, "\n") {
		cleaned += "\n"
	}
	return []byte(fmt.Sprintf("id: %s\n%s", id, cleaned))
}

func canonicalizeFlowMetadata(body []byte) []byte {
	trimmed := strings.TrimLeft(string(body), "\n")
	if strings.TrimSpace(trimmed) == "" {
		return []byte{}
	}

	var node yaml.Node
	if err := yaml.Unmarshal([]byte(trimmed), &node); err != nil {
		return []byte(trimmed)
	}

	root := &node
	if node.Kind == yaml.DocumentNode && len(node.Content) > 0 {
		root = node.Content[0]
	}

	if root.Kind == yaml.MappingNode {
		for i := 0; i < len(root.Content)-1; i += 2 {
			key := root.Content[i]
			value := root.Content[i+1]
			switch key.Value {
			case "events":
				if value.Kind == yaml.SequenceNode {
					sort.SliceStable(value.Content, func(a, b int) bool {
						return flowEventKey(value.Content[a]) < flowEventKey(value.Content[b])
					})
				}
			case "states":
				if value.Kind == yaml.SequenceNode {
					sort.SliceStable(value.Content, func(a, b int) bool {
						return flowStateKey(value.Content[a]) < flowStateKey(value.Content[b])
					})
					for _, stateNode := range value.Content {
						normalizeFlowState(stateNode)
					}
				}
			case "state_fields":
				if value.Kind == yaml.SequenceNode {
					sort.SliceStable(value.Content, func(a, b int) bool {
						return flowStateFieldName(value.Content[a]) < flowStateFieldName(value.Content[b])
					})
					for _, fieldNode := range value.Content {
						removeMappingKey(fieldNode, "id")
					}
				}
			}
		}
	}

	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(&node); err == nil {
		_ = enc.Close()
		return []byte(strings.TrimLeft(buf.String(), "\n"))
	}
	_ = enc.Close()
	return []byte(trimmed)
}

func flowEventKey(node *yaml.Node) string {
	if node == nil {
		return ""
	}
	if node.Kind == yaml.MappingNode {
		var idn, skill string
		for i := 0; i < len(node.Content)-1; i += 2 {
			key := node.Content[i]
			value := node.Content[i+1]
			switch key.Value {
			case "idn":
				idn = value.Value
			case "skillidn":
				skill = value.Value
			}
		}
		return idn + "::" + skill
	}
	if node.Kind == yaml.DocumentNode && len(node.Content) > 0 {
		return flowEventKey(node.Content[0])
	}
	return ""
}

func flowStateKey(node *yaml.Node) string {
	if node == nil {
		return ""
	}
	if node.Kind == yaml.MappingNode {
		for i := 0; i < len(node.Content)-1; i += 2 {
			key := node.Content[i]
			if key.Value == "idn" {
				return node.Content[i+1].Value
			}
		}
	}
	if node.Kind == yaml.DocumentNode && len(node.Content) > 0 {
		return flowStateKey(node.Content[0])
	}
	return ""
}

func normalizeFlowState(node *yaml.Node) {
	if node == nil {
		return
	}
	if node.Kind == yaml.MappingNode {
		var fieldsNode *yaml.Node
		for i := 0; i < len(node.Content)-1; i += 2 {
			key := node.Content[i]
			value := node.Content[i+1]
			if key.Value == "state_fields" && value.Kind == yaml.SequenceNode {
				fieldsNode = value
				break
			}
		}
		if fieldsNode != nil {
			sort.SliceStable(fieldsNode.Content, func(a, b int) bool {
				return flowStateFieldName(fieldsNode.Content[a]) < flowStateFieldName(fieldsNode.Content[b])
			})
			for _, fieldNode := range fieldsNode.Content {
				removeMappingKey(fieldNode, "id")
			}
		}
	}
}

func flowStateFieldName(node *yaml.Node) string {
	if node == nil {
		return ""
	}
	if node.Kind == yaml.MappingNode {
		for i := 0; i < len(node.Content)-1; i += 2 {
			key := node.Content[i]
			if key.Value == "name" {
				return node.Content[i+1].Value
			}
		}
	}
	if node.Kind == yaml.DocumentNode && len(node.Content) > 0 {
		return flowStateFieldName(node.Content[0])
	}
	return ""
}

func removeMappingKey(node *yaml.Node, target string) {
	if node == nil || node.Kind != yaml.MappingNode {
		return
	}
	for i := 0; i < len(node.Content)-1; i += 2 {
		if node.Content[i].Value == target {
			node.Content = append(node.Content[:i], node.Content[i+2:]...)
			i -= 2
		}
	}
}

func removeFlowStateFieldIDs(body []byte) []byte {
	if len(body) == 0 {
		return body
	}
	lines := strings.Split(string(body), "\n")
	result := make([]string, 0, len(lines))
	inFields := false
	baseIndent := ""
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "state_fields:") {
			inFields = true
			baseIndent = leadingWhitespace(line)
			result = append(result, line)
			continue
		}
		if inFields {
			if trimmed == "" {
				result = append(result, line)
				continue
			}
			lineIndent := leadingWhitespace(line)
			if len(lineIndent) <= len(baseIndent) && !strings.HasPrefix(trimmed, "-") {
				inFields = false
			}
		}
		if inFields && strings.HasPrefix(trimmed, "id:") && !strings.HasPrefix(trimmed, "idn:") {
			continue
		}
		result = append(result, line)
	}
	return []byte(strings.Join(result, "\n"))
}

func leadingWhitespace(s string) string {
	for i, r := range s {
		if r != ' ' && r != '\t' {
			return s[:i]
		}
	}
	return s
}

func canonicalizeProjectJSON(content []byte) ([]byte, map[string]string) {
	if len(bytes.TrimSpace(content)) == 0 {
		return []byte("{}\n"), nil
	}
	var data map[string]any
	if err := json.Unmarshal(content, &data); err != nil {
		return ensureJSONNewline(content), nil
	}
	ids := map[string]string{}
	if v, ok := data["customer_idn"].(string); ok {
		ids["customer_idn"] = v
		delete(data, "customer_idn")
	}
	if v, ok := data["project_id"].(string); ok {
		ids["project_id"] = v
		delete(data, "project_id")
	}
	normalized, err := json.Marshal(data)
	if err != nil {
		return ensureJSONNewline(content), ids
	}
	return ensureJSONNewline(normalized), ids
}

func applyProjectIDs(body []byte, ids map[string]string) []byte {
	var data map[string]any
	if err := json.Unmarshal(body, &data); err != nil {
		return ensureJSONNewline(body)
	}
	for k, v := range ids {
		if v != "" {
			data[k] = v
		}
	}
	normalized, err := json.Marshal(data)
	if err != nil {
		return ensureJSONNewline(body)
	}
	return ensureJSONNewline(normalized)
}

func ensureJSONNewline(content []byte) []byte {
	trimmed := bytes.TrimSpace(content)
	if len(trimmed) == 0 {
		return []byte("{}")
	}
	return trimmed
}

func extractFlowStateFieldIDs(content []byte) map[string]map[string]string {
	if len(content) == 0 {
		return nil
	}
	var node yaml.Node
	if err := yaml.Unmarshal(content, &node); err != nil {
		return nil
	}
	root := &node
	if node.Kind == yaml.DocumentNode && len(node.Content) > 0 {
		root = node.Content[0]
	}
	ids := make(map[string]map[string]string)
	if root.Kind != yaml.MappingNode {
		return ids
	}
	for i := 0; i < len(root.Content)-1; i += 2 {
		key := root.Content[i]
		value := root.Content[i+1]
		switch key.Value {
		case "states":
			if value.Kind != yaml.SequenceNode {
				continue
			}
			for _, stateNode := range value.Content {
				stateIDN := flowStateKey(stateNode)
				if stateIDN == "" {
					continue
				}
				if stateMap := collectStateFieldIDs(stateNode); len(stateMap) > 0 {
					ids[stateIDN] = stateMap
				}
			}
		case "state_fields":
			if value.Kind != yaml.SequenceNode {
				continue
			}
			if rootMap := collectFieldListIDs(value); len(rootMap) > 0 {
				ids["__root__"] = rootMap
			}
		}
	}
	return ids
}

func collectStateFieldIDs(stateNode *yaml.Node) map[string]string {
	if stateNode == nil || stateNode.Kind != yaml.MappingNode {
		return nil
	}
	result := make(map[string]string)
	for i := 0; i < len(stateNode.Content)-1; i += 2 {
		key := stateNode.Content[i]
		value := stateNode.Content[i+1]
		if key.Value != "state_fields" || value.Kind != yaml.SequenceNode {
			continue
		}
		for _, fieldNode := range value.Content {
			if fieldNode.Kind != yaml.MappingNode {
				continue
			}
			var name, id string
			for j := 0; j < len(fieldNode.Content)-1; j += 2 {
				k := fieldNode.Content[j]
				v := fieldNode.Content[j+1]
				switch k.Value {
				case "name":
					name = v.Value
				case "id":
					id = v.Value
				}
			}
			if name != "" && id != "" {
				result[name] = id
			}
		}
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

func collectFieldListIDs(seq *yaml.Node) map[string]string {
	if seq == nil || seq.Kind != yaml.SequenceNode {
		return nil
	}
	result := make(map[string]string)
	for _, fieldNode := range seq.Content {
		if fieldNode.Kind != yaml.MappingNode {
			continue
		}
		var name, id string
		for i := 0; i < len(fieldNode.Content)-1; i += 2 {
			k := fieldNode.Content[i]
			v := fieldNode.Content[i+1]
			switch k.Value {
			case "name":
				name = v.Value
			case "id":
				id = v.Value
			}
		}
		if name != "" && id != "" {
			result[name] = id
		}
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

func applyFlowStateFieldIDs(body []byte, ids map[string]map[string]string) []byte {
	if len(ids) == 0 {
		return ensureTrailingNewline(body)
	}
	var node yaml.Node
	if err := yaml.Unmarshal(body, &node); err != nil {
		return ensureTrailingNewline(body)
	}
	root := &node
	if node.Kind == yaml.DocumentNode && len(node.Content) > 0 {
		root = node.Content[0]
	}
	if root.Kind == yaml.MappingNode {
		for i := 0; i < len(root.Content)-1; i += 2 {
			key := root.Content[i]
			value := root.Content[i+1]
			switch key.Value {
			case "states":
				if value.Kind != yaml.SequenceNode {
					continue
				}
				for _, stateNode := range value.Content {
					stateIDN := flowStateKey(stateNode)
					if stateIDN == "" {
						continue
					}
					fieldIDs := ids[stateIDN]
					if len(fieldIDs) == 0 {
						continue
					}
					applyIDsToStateFields(stateNode, fieldIDs)
				}
			case "state_fields":
				if value.Kind != yaml.SequenceNode {
					continue
				}
				applyIDsToFieldList(value, ids["__root__"])
			}
		}
	}
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(&node); err == nil {
		_ = enc.Close()
		return []byte(strings.TrimLeft(buf.String(), "\n"))
	}
	_ = enc.Close()
	return ensureTrailingNewline(body)
}

func applyIDsToStateFields(stateNode *yaml.Node, ids map[string]string) {
	if stateNode == nil || stateNode.Kind != yaml.MappingNode {
		return
	}
	for i := 0; i < len(stateNode.Content)-1; i += 2 {
		key := stateNode.Content[i]
		value := stateNode.Content[i+1]
		if key.Value != "state_fields" || value.Kind != yaml.SequenceNode {
			continue
		}
		for _, fieldNode := range value.Content {
			name := flowStateFieldName(fieldNode)
			if name == "" {
				continue
			}
			if id, ok := ids[name]; ok {
				setMappingValue(fieldNode, "id", id)
			}
		}
	}
}

func applyIDsToFieldList(seq *yaml.Node, ids map[string]string) {
	if seq == nil || seq.Kind != yaml.SequenceNode || len(ids) == 0 {
		return
	}
	for _, fieldNode := range seq.Content {
		name := flowStateFieldName(fieldNode)
		if name == "" {
			continue
		}
		if id, ok := ids[name]; ok {
			setMappingValue(fieldNode, "id", id)
		}
	}
}

func setMappingValue(node *yaml.Node, key, val string) {
	if node == nil || node.Kind != yaml.MappingNode {
		return
	}
	for i := 0; i < len(node.Content)-1; i += 2 {
		if node.Content[i].Value == key {
			node.Content[i+1].Value = val
			return
		}
	}
	node.Content = append(node.Content, &yaml.Node{Kind: yaml.ScalarNode, Value: key}, &yaml.Node{Kind: yaml.ScalarNode, Value: val})
}
