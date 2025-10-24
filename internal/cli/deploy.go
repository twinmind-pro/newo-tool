package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/twinmind/newo-tool/internal/config"
	"github.com/twinmind/newo-tool/internal/customer"
	"github.com/twinmind/newo-tool/internal/deploy"
	"github.com/twinmind/newo-tool/internal/fsutil"
	"github.com/twinmind/newo-tool/internal/session"
	"github.com/twinmind/newo-tool/internal/state"
	"github.com/twinmind/newo-tool/internal/ui/console"
)

// DeployCommand provisions a project into a target customer.
type DeployCommand struct {
	stdout  io.Writer
	stderr  io.Writer
	console *console.Writer

	verbose        *bool
	sourceCustomer *string
}

// NewDeployCommand constructs a deploy command.
func NewDeployCommand(stdout, stderr io.Writer) *DeployCommand {
	return &DeployCommand{
		stdout:  stdout,
		stderr:  stderr,
		console: console.New(stdout, stderr),
	}
}

func (c *DeployCommand) ensureConsole() {
	if c.console == nil {
		c.console = console.New(c.stdout, c.stderr)
	}
}

func (c *DeployCommand) Name() string {
	return "deploy"
}

func (c *DeployCommand) Summary() string {
	return "Create a project in a target customer based on a local integration"
}

func (c *DeployCommand) RegisterFlags(fs *flag.FlagSet) {
	c.verbose = fs.Bool("verbose", false, "enable verbose logging")
	c.sourceCustomer = fs.String("source-customer", "", "integration customer IDN to use as source")
}

func (c *DeployCommand) Run(ctx context.Context, args []string) error {
	c.ensureConsole()

	if len(args) != 3 || !strings.EqualFold(args[1], "to") {
		return fmt.Errorf("usage: newo deploy <project_idn> to <target_customer_idn> [--source-customer]")
	}

	projectIDN := strings.TrimSpace(args[0])
	targetCustomerIDN := strings.TrimSpace(args[2])
	if projectIDN == "" || targetCustomerIDN == "" {
		return fmt.Errorf("project_idn and target_customer_idn are required")
	}

	verbose := c.verbose != nil && *c.verbose
	sourceCustomerHint := ""
	if c.sourceCustomer != nil {
		sourceCustomerHint = strings.TrimSpace(*c.sourceCustomer)
	}

	env, err := config.LoadEnv()
	if err != nil {
		return err
	}

	cfg, err := customer.FromEnv(env)
	if err != nil {
		return err
	}

	sourceEntry, err := c.resolveSourceCustomer(cfg, projectIDN, sourceCustomerHint)
	if err != nil {
		return err
	}
	if !strings.EqualFold(sourceEntry.Type, "integration") {
		return fmt.Errorf("source customer %s must have type integration (got %s)", sourceEntry.HintIDN, sourceEntry.Type)
	}

	targetEntry, err := cfg.FindCustomer(targetCustomerIDN)
	if err != nil {
		return err
	}
	if strings.EqualFold(targetEntry.Type, "integration") {
		return fmt.Errorf("target customer %s must not have type integration", targetEntry.HintIDN)
	}

	releaseLock, err := fsutil.AcquireLock("deploy")
	if err != nil {
		if errors.Is(err, fsutil.ErrLocked) {
			return fmt.Errorf("another operation is already running; please retry later")
		}
		return err
	}
	defer func() {
		if err := releaseLock(); err != nil && verbose {
			c.console.Warn("Release lock: %v", err)
		}
	}()

	registry, err := state.LoadAPIKeyRegistry()
	if err != nil {
		return err
	}

	sourceSession, err := session.New(ctx, env, *sourceEntry, registry)
	if err != nil {
		return err
	}

	targetSession, err := session.New(ctx, env, *targetEntry, registry)
	if err != nil {
		return err
	}
	registryDirty := sourceSession.RegistryUpdated || targetSession.RegistryUpdated

	sourceConfig := deploy.SourceConfig{
		OutputRoot:   env.OutputRoot,
		CustomerType: sourceSession.CustomerType,
		CustomerIDN:  sourceSession.IDN,
		ProjectIDN:   projectIDN,
		SlugPrefix:   env.SlugPrefix,
	}
	projectPlan, err := deploy.LoadSourceProject(sourceConfig)
	if err != nil {
		return err
	}

	deployService := deploy.NewService(targetSession.Client)
	reporter := consoleReporter{writer: c.console}
	request := deploy.DeployRequest{
		Project:            projectPlan,
		TargetCustomerIDN:  targetSession.IDN,
		TargetCustomerType: targetSession.CustomerType,
		OutputRoot:         env.OutputRoot,
		WorkspaceDir:       ".",
		Reporter:           reporter,
	}

	result, err := deployService.Deploy(ctx, request)
	if err != nil {
		return err
	}

	if err := config.AddProjectToToml(config.DefaultTomlPath, targetSession.IDN, projectIDN, result.ProjectID); err != nil {
		return fmt.Errorf("update newo.toml: %w", err)
	}

	if registryDirty {
		if err := registry.Save(); err != nil && verbose {
			c.console.Warn("Save API key registry: %v", err)
		}
	}

	c.console.Success("Project %s deployed to %s (ID %s)", projectIDN, targetSession.IDN, result.ProjectID)
	return nil
}

func (c *DeployCommand) resolveSourceCustomer(cfg customer.Configuration, projectIDN, hint string) (*customer.Entry, error) {
	if hint != "" {
		entry, err := cfg.FindCustomer(hint)
		if err != nil {
			return nil, err
		}
		return entry, nil
	}

	var matches []*customer.Entry
	for idx := range cfg.Entries {
		entry := &cfg.Entries[idx]
		if strings.EqualFold(entry.ProjectIDN, projectIDN) {
			matches = append(matches, entry)
		}
	}

	if len(matches) == 0 {
		return nil, fmt.Errorf("source customer for project %s not found; use --source-customer", projectIDN)
	}
	if len(matches) > 1 {
		ids := make([]string, 0, len(matches))
		for _, entry := range matches {
			ids = append(ids, entry.HintIDN)
		}
		return nil, fmt.Errorf("multiple integration customers provide project %s; specify one with --source-customer (candidates: %s)", projectIDN, strings.Join(ids, ", "))
	}
	return matches[0], nil
}
