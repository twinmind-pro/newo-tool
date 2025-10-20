package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"

	"github.com/twinmind/newo-tool/internal/config"
	"github.com/twinmind/newo-tool/internal/healthcheck"
	"github.com/twinmind/newo-tool/internal/ui/console"
)

// HealthcheckCommand performs health checks on the environment and configuration.
type HealthcheckCommand struct {
	stdout  io.Writer
	stderr  io.Writer
	console *console.Writer
}

// NewHealthcheckCommand constructs a healthcheck command.
func NewHealthcheckCommand(stdout, stderr io.Writer) *HealthcheckCommand {
	return &HealthcheckCommand{
		stdout:  stdout,
		stderr:  stderr,
		console: console.New(stdout, stderr),
	}
}

func (c *HealthcheckCommand) ensureConsole() {
	if c.console == nil {
		c.console = console.New(c.stdout, c.stderr)
	}
}

func (c *HealthcheckCommand) Name() string {
	return "healthcheck"
}

func (c *HealthcheckCommand) Summary() string {
	return "Performs health checks on the environment and configuration"
}

func (c *HealthcheckCommand) RegisterFlags(_ *flag.FlagSet) {
	// No flags for this command yet.
}

func (c *HealthcheckCommand) Run(ctx context.Context, _ []string) error {
	c.ensureConsole()
	c.console.Section("Healthcheck")
	var errs []error

	// 1. Configuration Loading
	env, err := config.LoadEnv()
	if err != nil {
		c.console.Error("Failed to load configuration: %v", err)
		// This is a fatal error, we can't proceed without the environment.
		return fmt.Errorf("failed to load configuration, cannot proceed with checks: %w", err)
	}
	c.console.Success("Configuration loaded.")

	// 2. Configuration Check
	if err := healthcheck.CheckConfig(); err != nil {
		c.console.Warn("Configuration check failed: %v", err)
		errs = append(errs, fmt.Errorf("configuration check failed: %w", err))
	} else {
		c.console.Success("Configuration check passed.")
	}

	// 3. Filesystem Check
	if err := healthcheck.CheckFilesystem(env); err != nil {
		c.console.Warn("Filesystem check failed: %v", err)
		errs = append(errs, fmt.Errorf("filesystem check failed: %w", err))
	} else {
		c.console.Success("Filesystem check passed.")
	}

	// 4. Platform Connectivity Check
	if customerIDN, err := healthcheck.CheckPlatformConnectivity(ctx, env); err != nil {
		c.console.Warn("Platform connectivity check failed: %v", err)
		errs = append(errs, fmt.Errorf("platform connectivity check failed: %w", err))
	} else {
		c.console.Success("Platform connectivity ok (customer: %s).", customerIDN)
	}

	// 5. Local State Check
	if customerIDN, err := healthcheck.CheckLocalState(env); err != nil {
		c.console.Warn("Local state check failed: %v", err)
		errs = append(errs, fmt.Errorf("local state check failed: %w", err))
	} else {
		c.console.Success("Local state check passed (customer: %s).", customerIDN)
	}

	// 6. External Tools Check
	if err := healthcheck.CheckExternalTools(); err != nil {
		c.console.Warn("External tools check failed: %v", err)
		errs = append(errs, fmt.Errorf("external tools check failed: %w", err))
	} else {
		c.console.Success("External tools check passed.")
	}

	if len(errs) > 0 {
		c.console.Warn("Health checks completed with errors.")
		return errors.Join(errs...)
	}

	c.console.Success("All health checks passed successfully.")
	return nil
}
