package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"

	"github.com/twinmind/newo-tool/internal/config"
	"github.com/twinmind/newo-tool/internal/healthcheck"
)

// HealthcheckCommand performs health checks on the environment and configuration.
type HealthcheckCommand struct {
	stdout io.Writer
	stderr io.Writer
}

// NewHealthcheckCommand constructs a healthcheck command.
func NewHealthcheckCommand(stdout, stderr io.Writer) *HealthcheckCommand {
	return &HealthcheckCommand{stdout: stdout, stderr: stderr}
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
	var errs []error

	fmt.Fprintln(c.stdout, "Performing health checks...")

	// 1. Configuration Loading
	env, err := config.LoadEnv()
	if err != nil {
		// This is a fatal error, we can't proceed without the environment.
		return fmt.Errorf("failed to load configuration, cannot proceed with checks: %w", err)
	}
	fmt.Fprintln(c.stdout, "  [OK] Configuration loaded")

	// 2. Configuration Check
	if err := healthcheck.CheckConfig(); err != nil {
		fmt.Fprintln(c.stdout, "  [FAIL] Configuration check")
		errs = append(errs, fmt.Errorf("configuration check failed: %w", err))
	} else {
		fmt.Fprintln(c.stdout, "  [OK] Configuration check")
	}

	// 3. Filesystem Check
	if err := healthcheck.CheckFilesystem(env); err != nil {
		fmt.Fprintln(c.stdout, "  [FAIL] Filesystem check")
		errs = append(errs, fmt.Errorf("filesystem check failed: %w", err))
	} else {
		fmt.Fprintln(c.stdout, "  [OK] Filesystem check")
	}

	// 4. Platform Connectivity Check
	if customerIDN, err := healthcheck.CheckPlatformConnectivity(ctx, env); err != nil {
		fmt.Fprintln(c.stdout, "  [FAIL] Platform connectivity check")
		errs = append(errs, fmt.Errorf("platform connectivity check failed: %w", err))
	} else {
		fmt.Fprintf(c.stdout, "  [OK] Platform connectivity (customer: %s)\n", customerIDN)
	}

	// 5. Local State Check
	if _, err := healthcheck.CheckLocalState(env); err != nil {
		fmt.Fprintln(c.stdout, "  [FAIL] Local state check")
		errs = append(errs, fmt.Errorf("local state check failed: %w", err))
	} else {
		fmt.Fprintln(c.stdout, "  [OK] Local state check")
	}

	// 6. External Tools Check
	if err := healthcheck.CheckExternalTools(); err != nil {
		fmt.Fprintln(c.stdout, "  [FAIL] External tools check")
		errs = append(errs, fmt.Errorf("external tools check failed: %w", err))
	} else {
		fmt.Fprintln(c.stdout, "  [OK] External tools check")
	}

	if len(errs) > 0 {
		fmt.Fprintln(c.stderr, "\nHealth checks completed with errors:")
		// Return a single error containing all sub-errors.
		return errors.Join(errs...)
	}

	fmt.Fprintln(c.stdout, "\nAll health checks passed successfully.")
	return nil
}
