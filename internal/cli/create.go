package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"
	"unicode"

	"github.com/twinmind/newo-tool/internal/config"
	"github.com/twinmind/newo-tool/internal/create"
	"github.com/twinmind/newo-tool/internal/customer"
	"github.com/twinmind/newo-tool/internal/deploy"
	"github.com/twinmind/newo-tool/internal/fsutil"
	"github.com/twinmind/newo-tool/internal/serialize"
	"github.com/twinmind/newo-tool/internal/session"
	"github.com/twinmind/newo-tool/internal/state"
	"github.com/twinmind/newo-tool/internal/ui/console"
)

var defaultCreateFlows = []string{
	"SetupFlow",
	"AvailabilityFlow",
	"BookingFlow",
	"CancellationFlow",
	"ExistingClientFlow",
	"StaticDataFlow",
}

type createRunner interface {
	Create(ctx context.Context, req create.Request) (deploy.DeployResult, error)
}

// CreateCommand bootstraps a new integration project for a customer.
type CreateCommand struct {
	stdout io.Writer
	stderr io.Writer

	console       *console.Writer
	projectIDN    *string
	slug          *string
	verbose       *bool
	loadEnv       func() (config.Env, error)
	loadCustomers func(config.Env) (customer.Configuration, error)
	loadRegistry  func() (*state.APIKeyRegistry, error)
	newSession    func(ctx context.Context, env config.Env, entry customer.Entry, registry *state.APIKeyRegistry) (*session.Session, error)
	newService    func(client deploy.DeployClient) createRunner
	addProject    func(path, customerIDN, projectIDN, projectID string) error
}

// NewCreateCommand sets up the command with default dependencies.
func NewCreateCommand(stdout, stderr io.Writer) *CreateCommand {
	return &CreateCommand{
		stdout:        stdout,
		stderr:        stderr,
		console:       console.New(stdout, stderr),
		loadEnv:       config.LoadEnv,
		loadCustomers: customer.FromEnv,
		loadRegistry:  state.LoadAPIKeyRegistry,
		newSession:    session.New,
		newService: func(client deploy.DeployClient) createRunner {
			return create.NewService(client)
		},
		addProject: config.AddProjectToToml,
	}
}

func (c *CreateCommand) ensureConsole() {
	if c.console == nil {
		c.console = console.New(c.stdout, c.stderr)
	}
}

func (c *CreateCommand) Name() string {
	return "create"
}

func (c *CreateCommand) Summary() string {
	return "Bootstrap a new integration project for a customer"
}

func (c *CreateCommand) RegisterFlags(fs *flag.FlagSet) {
	c.projectIDN = fs.String("project-idn", "", "set explicit project IDN")
	c.slug = fs.String("slug", "", "override generated project slug")
	c.verbose = fs.Bool("verbose", false, "enable verbose logging")
}

func (c *CreateCommand) Run(ctx context.Context, args []string) error {
	c.ensureConsole()

	projectName, customerToken, err := parseCreateArgs(args)
	if err != nil {
		return err
	}

	releaseLock, err := fsutil.AcquireLock("create")
	if err != nil {
		if errors.Is(err, fsutil.ErrLocked) {
			return fmt.Errorf("another operation is already running; please retry later")
		}
		return err
	}
	defer func() {
		if err := releaseLock(); err != nil && c.verbose != nil && *c.verbose {
			c.console.Warn("Release lock: %v", err)
		}
	}()

	env, err := c.loadEnv()
	if err != nil {
		return err
	}
	cfg, err := c.loadCustomers(env)
	if err != nil {
		return err
	}

	entry, err := cfg.FindCustomer(customerToken)
	if err != nil {
		return err
	}
	if !strings.EqualFold(entry.Type, "integration") {
		return fmt.Errorf("customer %s must have type integration", customerToken)
	}

	registry, err := c.loadRegistry()
	if err != nil {
		return err
	}

	sess, err := c.newSession(ctx, env, *entry, registry)
	if err != nil {
		return err
	}
	registryDirty := sess.RegistryUpdated

	projectIDN, err := c.resolveProjectIDN(projectName)
	if err != nil {
		return err
	}
	slug, err := c.resolveSlug(projectIDN, env.SlugPrefix)
	if err != nil {
		return err
	}
	agentIDN, err := generateAgentIDN(projectName)
	if err != nil {
		return err
	}

	projectMap, err := state.LoadProjectMap(sess.IDN)
	if err != nil {
		return err
	}
	if hasProject(projectMap, projectIDN) {
		return fmt.Errorf("project %s already exists locally for %s", projectIDN, sess.IDN)
	}

	attrs, err := serialize.GenerateAttributesYAML(nil)
	if err != nil {
		return fmt.Errorf("generate attributes yaml: %w", err)
	}

	service := c.newService(sess.Client)
	if service == nil {
		return fmt.Errorf("create service not available")
	}

	req := create.Request{
		ProjectName:        projectName,
		ProjectIDN:         projectIDN,
		Slug:               slug,
		AgentIDN:           agentIDN,
		FlowIDNs:           defaultCreateFlows,
		TargetCustomerIDN:  sess.IDN,
		TargetCustomerType: sess.CustomerType,
		OutputRoot:         env.OutputRoot,
		WorkspaceDir:       ".",
		Reporter:           consoleReporter{writer: c.console},
		Attributes:         attrs,
	}

	result, err := service.Create(ctx, req)
	if err != nil {
		return err
	}

	if err := c.addProject(config.DefaultTomlPath, sess.IDN, projectIDN, result.ProjectID); err != nil {
		return fmt.Errorf("update newo.toml: %w", err)
	}

	if registryDirty {
		if err := registry.Save(); err != nil && c.verbose != nil && *c.verbose {
			c.console.Warn("Save API key registry: %v", err)
		}
	}

	c.console.Success("Project %s created in %s (ID %s)", projectIDN, sess.IDN, result.ProjectID)
	return nil
}

func parseCreateArgs(args []string) (string, string, error) {
	if len(args) == 0 {
		return "", "", fmt.Errorf("usage: newo create <project_name> in <customer>")
	}
	inIdx := -1
	for idx, token := range args {
		if strings.EqualFold(token, "in") {
			inIdx = idx
			break
		}
	}
	if inIdx <= 0 || inIdx == len(args)-1 {
		return "", "", fmt.Errorf("usage: newo create <project_name> in <customer>")
	}

	projectName := strings.TrimSpace(strings.Join(args[:inIdx], " "))
	if projectName == "" {
		return "", "", fmt.Errorf("project name is required")
	}

	tail := args[inIdx+1:]
	if len(tail) != 1 {
		return "", "", fmt.Errorf("customer identifier must follow 'in'")
	}
	customer := strings.TrimSpace(tail[0])
	if customer == "" {
		return "", "", fmt.Errorf("customer identifier is required")
	}
	return projectName, customer, nil
}

func (c *CreateCommand) resolveProjectIDN(projectName string) (string, error) {
	if c.projectIDN != nil && strings.TrimSpace(*c.projectIDN) != "" {
		return validateProjectIDN(strings.TrimSpace(*c.projectIDN))
	}
	return generateProjectIDN(projectName)
}

func (c *CreateCommand) resolveSlug(projectIDN, slugPrefix string) (string, error) {
	if c.slug != nil && strings.TrimSpace(*c.slug) != "" {
		return sanitizeSlug(strings.TrimSpace(*c.slug))
	}
	return generateSlug(projectIDN, slugPrefix)
}

func generateProjectIDN(projectName string) (string, error) {
	words := tokenize(projectName)
	if len(words) == 0 {
		return "", fmt.Errorf("project name must include alphanumeric characters")
	}
	var builder strings.Builder
	for idx, word := range words {
		lower := strings.ToLower(word)
		if idx == 0 {
			builder.WriteString(lower)
			continue
		}
		builder.WriteString(strings.ToUpper(lower[:1]))
		if len(lower) > 1 {
			builder.WriteString(lower[1:])
		}
	}
	return builder.String(), nil
}

func validateProjectIDN(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", fmt.Errorf("project idn is required")
	}
	for _, r := range value {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' {
			continue
		}
		return "", fmt.Errorf("project idn %q contains invalid characters", value)
	}
	return value, nil
}

func generateSlug(projectIDN, prefix string) (string, error) {
	var builder strings.Builder
	prevClass := runeClassNone
	pendingHyphen := false
	for _, r := range projectIDN {
		class := classifyRune(r)
		switch class {
		case runeClassLower:
			if pendingHyphen {
				builder.WriteRune('-')
				pendingHyphen = false
			}
			builder.WriteRune(r)
		case runeClassUpper:
			if pendingHyphen || (prevClass == runeClassLower || prevClass == runeClassDigit) {
				builder.WriteRune('-')
			}
			builder.WriteRune(unicode.ToLower(r))
			pendingHyphen = false
		case runeClassDigit:
			if pendingHyphen {
				builder.WriteRune('-')
				pendingHyphen = false
			}
			builder.WriteRune(r)
		default:
			if builder.Len() > 0 && !pendingHyphen {
				pendingHyphen = true
			}
		}
		if class != runeClassOther {
			prevClass = class
		} else {
			prevClass = runeClassNone
		}
	}
	slug := builder.String()
	if pendingHyphen {
		slug += "-"
	}
	slug = normalizeHyphens(slug)
	if slug == "" {
		return "", fmt.Errorf("could not derive slug from project idn %s", projectIDN)
	}
	slug = prefix + slug
	return slug, nil
}

func sanitizeSlug(value string) (string, error) {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return "", fmt.Errorf("slug override cannot be empty")
	}
	var builder strings.Builder
	lastHyphen := false
	for _, r := range value {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			builder.WriteRune(unicode.ToLower(r))
			lastHyphen = false
		case r == '-' || r == '_' || unicode.IsSpace(r):
			if !lastHyphen && builder.Len() > 0 {
				builder.WriteRune('-')
				lastHyphen = true
			}
		default:
			// skip invalid characters
		}
	}
	slug := strings.Trim(builder.String(), "-")
	if slug == "" {
		return "", fmt.Errorf("slug override %q is invalid", value)
	}
	return slug, nil
}

func generateAgentIDN(projectName string) (string, error) {
	words := tokenize(projectName)
	if len(words) == 0 {
		return "", fmt.Errorf("project name must include alphanumeric characters")
	}
	var builder strings.Builder
	for _, word := range words {
		lower := strings.ToLower(word)
		builder.WriteString(strings.ToUpper(lower[:1]))
		if len(lower) > 1 {
			builder.WriteString(lower[1:])
		}
	}
	if builder.Len() == 0 {
		builder.WriteString("Project")
	}
	builder.WriteString("Agent")
	return builder.String(), nil
}

func tokenize(value string) []string {
	var words []string
	var current strings.Builder
	for _, r := range value {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			current.WriteRune(r)
			continue
		}
		if current.Len() > 0 {
			words = append(words, current.String())
			current.Reset()
		}
	}
	if current.Len() > 0 {
		words = append(words, current.String())
	}
	return words
}

const (
	runeClassNone = iota
	runeClassLower
	runeClassUpper
	runeClassDigit
	runeClassOther
)

func classifyRune(r rune) int {
	switch {
	case unicode.IsLower(r):
		return runeClassLower
	case unicode.IsUpper(r):
		return runeClassUpper
	case unicode.IsDigit(r):
		return runeClassDigit
	default:
		return runeClassOther
	}
}

func normalizeHyphens(value string) string {
	var builder strings.Builder
	lastHyphen := false
	for _, r := range value {
		if r == '-' {
			if !lastHyphen && builder.Len() > 0 {
				builder.WriteRune(r)
				lastHyphen = true
			}
			continue
		}
		builder.WriteRune(r)
		lastHyphen = false
	}
	return strings.Trim(builder.String(), "-")
}

func hasProject(pm state.ProjectMap, projectIDN string) bool {
	for key := range pm.Projects {
		if strings.EqualFold(key, projectIDN) {
			return true
		}
	}
	return false
}
