package cli

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/twinmind/newo-tool/internal/config"
	"github.com/twinmind/newo-tool/internal/customer"
	"github.com/twinmind/newo-tool/internal/diff"
	"github.com/twinmind/newo-tool/internal/fsutil"
	"github.com/twinmind/newo-tool/internal/session"
	"github.com/twinmind/newo-tool/internal/state"
	skillsync "github.com/twinmind/newo-tool/internal/sync"
	"github.com/twinmind/newo-tool/internal/ui/console"
)

// PushCommand uploads local script changes to the NEWO platform.
type PushCommand struct {
	stdout    io.Writer
	stderr    io.Writer
	console   *console.Writer
	verbose   *bool
	customer  *string
	noPublish *bool
	force     *bool

	outputRoot string
	slugPrefix string
}

// NewPushCommand constructs a push command.
func NewPushCommand(stdout, stderr io.Writer) *PushCommand {
	return &PushCommand{
		stdout:  stdout,
		stderr:  stderr,
		console: console.New(stdout, stderr),
	}
}

func (c *PushCommand) ensureConsole() {
	if c.console == nil {
		c.console = console.New(c.stdout, c.stderr)
	}
}

func (c *PushCommand) Name() string {
	return "push"
}

func (c *PushCommand) Summary() string {
	return "Upload local changes back to NEWO"
}

func (c *PushCommand) RegisterFlags(fs *flag.FlagSet) {
	c.verbose = fs.Bool("verbose", false, "show detailed output")
	c.customer = fs.String("customer", "", "customer IDN to push")
	c.noPublish = fs.Bool("no-publish", false, "skip publishing flows after upload")
	c.force = fs.Bool("force", false, "skip interactive diff and confirmation")
}

func (c *PushCommand) Run(ctx context.Context, args []string) error {
	c.ensureConsole()
	if len(args) > 0 {
		return fmt.Errorf("unexpected arguments: %s", strings.Join(args, " "))
	}

	verbose := c.verbose != nil && *c.verbose
	customerFilter := ""
	if c.customer != nil {
		customerFilter = strings.TrimSpace(*c.customer)
	}
	shouldPublish := c.noPublish == nil || !*c.noPublish
	force := c.force != nil && *c.force

	env, err := config.LoadEnv()
	if err != nil {
		return err
	}

	c.outputRoot = env.OutputRoot
	c.slugPrefix = env.SlugPrefix

	cfg, err := customer.FromEnv(env)
	if err != nil {
		return err
	}

	registry, err := state.LoadAPIKeyRegistry()
	if err != nil {
		return err
	}

	releaseLock, err := fsutil.AcquireLock("push")
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

	registryDirty := false
	matchedFilter := false
	processed := map[string]bool{}

	for _, entry := range cfg.Entries {
		session, err := session.New(ctx, env, entry, registry)
		if err != nil {
			return err
		}
		if session.RegistryUpdated {
			registryDirty = true
		}

		if customerFilter != "" && !strings.EqualFold(session.IDN, customerFilter) {
			continue
		}

		key := strings.ToLower(session.IDN)
		if processed[key] {
			if customerFilter != "" && strings.EqualFold(session.IDN, customerFilter) {
				matchedFilter = true
				break
			}
			continue
		}

		if err := c.pushCustomer(ctx, session, shouldPublish, verbose, force); err != nil {
			return err
		}
		processed[key] = true

		if customerFilter != "" && strings.EqualFold(session.IDN, customerFilter) {
			matchedFilter = true
			break
		}
	}

	if customerFilter != "" && !matchedFilter {
		return fmt.Errorf("customer %s not configured", customerFilter)
	}

	if len(processed) == 0 {
		c.console.Info("No customers matched the selection. Run `newo pull` first to initialise state.")
	}

	if registryDirty {
		if err := registry.Save(); err != nil {
			return err
		}
	}

	return nil
}

func (c *PushCommand) pushCustomer(ctx context.Context, session *session.Session, shouldPublish bool, verbose bool, force bool) error {
	c.ensureConsole()
	if verbose {
		c.console.Section(fmt.Sprintf("Push %s", session.IDN))
	}

	projectMap, err := state.LoadProjectMap(session.IDN)
	if err != nil {
		return err
	}
	if len(projectMap.Projects) == 0 {
		c.console.Info("No project map for %s. Run `newo pull --customer %s` first.", session.IDN, session.IDN)
		return nil
	}

	hashes, err := state.LoadHashes(session.IDN)
	if err != nil {
		return err
	}
	if len(hashes) == 0 {
		c.console.Info("No hash snapshot for %s. Run `newo pull --customer %s` to initialise tracking.", session.IDN, session.IDN)
		return nil
	}

	service := skillsync.NewSkillSyncService(session.Client, nil)
	reporter := consoleReporter{writer: c.console}

	result, err := service.SyncCustomer(ctx, skillsync.SkillSyncRequest{
		SessionIDN:    session.IDN,
		CustomerType:  session.CustomerType,
		OutputRoot:    c.outputRoot,
		ProjectMap:    &projectMap,
		Hashes:        hashes,
		ShouldPublish: shouldPublish,
		Verbose:       verbose,
		Force:         force,
		Reporter:      reporter,
		ProjectSlugger: func(projectIDN string, data state.ProjectData) string {
			return c.projectSlug(projectIDN, data)
		},
		ConfirmPush:     c.confirmSkillUpdate,
		ConfirmDeletion: c.confirmSkillRemoval,
	})
	if err != nil {
		return err
	}

	if result.Force && c.force != nil {
		*c.force = true
	}

	if result.Updated == 0 && result.Removed == 0 && result.Created == 0 {
		c.console.Info("No changes to push for %s.", session.IDN)
		return nil
	}

	if result.Updated > 0 {
		if verbose {
			c.console.Success("Updated %d skill(s) for %s", result.Updated, session.IDN)
		} else {
			c.console.Success("Push complete for %s (%d skill(s) updated)", session.IDN, result.Updated)
		}
	}
	if result.Removed > 0 {
		c.console.Success("Removed %d skill(s) for %s", result.Removed, session.IDN)
	}
	if result.Created > 0 {
		c.console.Success("Created %d skill(s) for %s", result.Created, session.IDN)
	}
	if shouldPublish && result.Published > 0 && verbose {
		c.console.Info("Published %d flow(s) for %s", result.Published, session.IDN)
	}

	return nil
}

func (c *PushCommand) confirmSkillUpdate(req skillsync.ConfirmPushRequest) (skillsync.Decision, error) {
	c.ensureConsole()

	if len(req.Diff) > 0 {
		c.console.Write(diff.Format(req.Path, req.Diff))
	}

	c.console.Prompt("Push changes? [y/N/a]: ")
	reader := bufio.NewReader(os.Stdin)
	text, err := reader.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return skillsync.Decision{}, err
	}

	switch strings.TrimSpace(strings.ToLower(text)) {
	case "y":
		return skillsync.Decision{Apply: true}, nil
	case "a":
		return skillsync.Decision{Apply: true, ApplyAll: true}, nil
	default:
		c.console.Info("Skipping.")
		return skillsync.Decision{}, nil
	}
}

func (c *PushCommand) confirmSkillRemoval(path, skillIDN string) (skillsync.Decision, error) {
	c.ensureConsole()
	c.console.Prompt("Skill %s missing locally. Delete remote version %s? [y/N/a]: ", skillIDN, path)
	reader := bufio.NewReader(os.Stdin)
	text, err := reader.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return skillsync.Decision{}, err
	}
	switch strings.TrimSpace(strings.ToLower(text)) {
	case "y":
		return skillsync.Decision{Apply: true}, nil
	case "a":
		return skillsync.Decision{Apply: true, ApplyAll: true}, nil
	default:
		c.console.Info("Keeping remote skill.")
		return skillsync.Decision{}, nil
	}
}

type consoleReporter struct {
	writer *console.Writer
}

func (r consoleReporter) Infof(format string, args ...any) {
	if r.writer != nil {
		r.writer.Info(format, args...)
	}
}

func (r consoleReporter) Warnf(format string, args ...any) {
	if r.writer != nil {
		r.writer.Warn(format, args...)
	}
}

func (r consoleReporter) Successf(format string, args ...any) {
	if r.writer != nil {
		r.writer.Success(format, args...)
	}
}

func (c *PushCommand) projectSlug(projectIDN string, data state.ProjectData) string {
	slug := strings.TrimSpace(data.Path)
	if slug != "" {
		return slug
	}

	base := strings.TrimSpace(projectIDN)
	if base == "" {
		base = "project"
	}

	if c.slugPrefix != "" {
		return c.slugPrefix + strings.ToLower(base)
	}
	return strings.ToLower(base)
}
