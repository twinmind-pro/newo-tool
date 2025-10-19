
package cli

import (
	"context"
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
	_, _ = fmt.Fprintln(c.stdout, "Loading configuration...")
	env, err := config.LoadEnv()
	if err != nil {
		return fmt.Errorf("[FAIL] Failed to load configuration: %w", err)
	}
	_, _ = fmt.Fprintln(c.stdout, "[OK] Configuration loaded successfully.")

	_, _ = fmt.Fprintln(c.stdout, "Performing configuration health checks...")
	if err := healthcheck.CheckConfig(); err != nil {
		return fmt.Errorf("  [FAIL] Configuration check failed: %w", err)
	}
	_, _ = fmt.Fprintln(c.stdout, "  [OK] Configuration check passed.")

	_, _ = fmt.Fprintln(c.stdout, "Performing filesystem health checks...")
	if err := healthcheck.CheckFilesystem(env); err != nil {
		return fmt.Errorf("  [FAIL] Filesystem check failed: %w", err)
	}
	_, _ = fmt.Fprintln(c.stdout, "  [OK] Filesystem check passed.")

	_, _ = fmt.Fprintln(c.stdout, "Performing platform connectivity health checks...")
	customerIDN, err := healthcheck.CheckPlatformConnectivity(ctx, env)
	if err != nil {
		return fmt.Errorf("  [FAIL] Platform connectivity check failed: %w", err)
	}
	_, _ = fmt.Fprintf(c.stdout, "  [OK] Platform connectivity check passed for customer %s.\n", customerIDN)

	_, _ = fmt.Fprintln(c.stdout, "Performing local state health checks...")
	customerIDN, err = healthcheck.CheckLocalState(env)
	if err != nil {
		return fmt.Errorf("  [FAIL] Local state check failed: %w", err)
	}
	_, _ = fmt.Fprintf(c.stdout, "  [OK] Local state check passed for customer %s.\n", customerIDN)

	_, _ = fmt.Fprintln(c.stdout, "Performing external tools health checks...")
	if err := healthcheck.CheckExternalTools(); err != nil {
		return fmt.Errorf("  [FAIL] External tools check failed: %w", err)
	}
	_, _ = fmt.Fprintln(c.stdout, "  [OK] External tools check passed.")
	// TODO: Implement actual health checks
	return nil
}
