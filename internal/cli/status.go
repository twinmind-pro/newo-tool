package cli

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/twinmind/newo-tool/internal/config"
	"github.com/twinmind/newo-tool/internal/customer"
	"github.com/twinmind/newo-tool/internal/fsutil"
	"github.com/twinmind/newo-tool/internal/state"
	"github.com/twinmind/newo-tool/internal/status"
)

// StatusCommand reports local workspace changes compared to the last pull.
type StatusCommand struct {
	stdout   io.Writer
	stderr   io.Writer
	verbose  *bool
	customer *string
}

// NewStatusCommand constructs a status command.
func NewStatusCommand(stdout, stderr io.Writer) *StatusCommand {
	return &StatusCommand{stdout: stdout, stderr: stderr}
}

func (c *StatusCommand) Name() string {
	return "status"
}

func (c *StatusCommand) Summary() string {
	return "Show local changes since the last pull"
}

func (c *StatusCommand) RegisterFlags(fs *flag.FlagSet) {
	c.verbose = fs.Bool("verbose", false, "show detailed information")
	c.customer = fs.String("customer", "", "customer IDN to inspect")
}

func (c *StatusCommand) Run(ctx context.Context, _ []string) error {
	verbose := c.verbose != nil && *c.verbose
	customerFlag := ""
	if c.customer != nil {
		customerFlag = strings.TrimSpace(*c.customer)
	}

	env, err := config.LoadEnv()
	if err != nil {
		return err
	}

	cfg, err := customer.FromEnv(env)
	if err != nil {
		return err
	}

	registry, err := state.LoadAPIKeyRegistry()
	if err != nil {
		return err
	}

	customers := make(map[string]string)
	for _, entry := range cfg.Entries {
		idn := strings.TrimSpace(entry.HintIDN)
		if idn == "" {
			if entry.APIKey != "" {
				if regIDN, ok := registry.Lookup(entry.APIKey); ok {
					idn = regIDN
				}
			}
		}
		if idn != "" {
			customers[strings.ToLower(idn)] = idn
		}
	}

	localCustomers, err := listCustomersWithState()
	if err != nil {
		return err
	}
	for _, idn := range localCustomers {
		if idn == "" {
			continue
		}
		customers[strings.ToLower(idn)] = idn
	}

	var targetList []string
	targetIDN := customerFlag
	if targetIDN != "" {
		resolved := targetIDN
		if idn, ok := customers[strings.ToLower(targetIDN)]; ok {
			resolved = idn
		} else if _, err := os.Stat(fsutil.MapPath(strings.ToLower(targetIDN))); err == nil {
			resolved = strings.ToLower(targetIDN)
		} else if _, err := os.Stat(fsutil.MapPath(targetIDN)); err != nil {
			return fmt.Errorf("customer %s not configured or has no local state", targetIDN)
		}

		_, err := status.Run(resolved, env.OutputRoot, verbose, c.stdout, c.stderr)
		return err
	}

	for _, idn := range customers {
		targetList = append(targetList, idn)
	}
	sort.Strings(targetList)

	if len(targetList) == 0 && cfg.DefaultCustomer != "" {
		targetList = append(targetList, cfg.DefaultCustomer)
	}

	if len(targetList) == 0 {
		_, _ = fmt.Fprintln(c.stdout, "No customers with local state. Run `newo pull` first.")
		return nil
	}

	for idx, idn := range targetList {
		if len(targetList) > 1 {
			_, _ = fmt.Fprintf(c.stdout, "\n== %s (%d/%d) ==\n", idn, idx+1, len(targetList))
		}
		if _, err := status.Run(idn, env.OutputRoot, verbose, c.stdout, c.stderr); err != nil {
			return err
		}
	}

	return nil
}

func listCustomersWithState() ([]string, error) {
	entries, err := os.ReadDir(".newo")
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var customers []string
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		mapPath := filepath.Join(".newo", entry.Name(), "map.json")
		if _, err := os.Stat(mapPath); err == nil {
			customers = append(customers, strings.ToUpper(entry.Name()))
		}
	}
	return customers, nil
}
