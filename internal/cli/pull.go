package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/twinmind/newo-tool/internal/config"
	"github.com/twinmind/newo-tool/internal/customer"
	"github.com/twinmind/newo-tool/internal/diff"
	"github.com/twinmind/newo-tool/internal/fsutil"
	"github.com/twinmind/newo-tool/internal/platform"
	"github.com/twinmind/newo-tool/internal/serialize"
	"github.com/twinmind/newo-tool/internal/session"
	"github.com/twinmind/newo-tool/internal/state"
	"github.com/twinmind/newo-tool/internal/ui/console"
	"github.com/twinmind/newo-tool/internal/util"
	"golang.org/x/sync/errgroup"
	"gopkg.in/yaml.v3"
)

func (c *PullCommand) projectSlug(project platform.Project) string {
	slug := strings.ToLower(strings.TrimSpace(project.IDN))
	slug = strings.ReplaceAll(slug, " ", "-")
	slug = strings.Trim(slug, "-")
	if slug == "" {
		slug = strings.ToLower(strings.ReplaceAll(strings.TrimSpace(project.Title), " ", "-"))
	}
	if c.slugPrefix != "" {
		slug = c.slugPrefix + slug
	}
	return slug
}

// PullCommand synchronises remote platform data to the local workspace.
type PullCommand struct {
	stdout      io.Writer
	stderr      io.Writer
	console     *console.Writer
	force       *bool
	verbose     *bool
	customer    *string
	projectUUID *string
	projectIDN  *string
	outputRoot  string
	slugPrefix  string
	verboseOn   bool
	promptMu    sync.Mutex
}

// NewPullCommand constructs a pull command using provided output writers.
func NewPullCommand(stdout, stderr io.Writer) *PullCommand {
	return &PullCommand{
		stdout:  stdout,
		stderr:  stderr,
		console: console.New(stdout, stderr),
	}
}

func (c *PullCommand) ensureConsole() {
	if c.console == nil {
		c.console = console.New(c.stdout, c.stderr)
	}
}

func (c *PullCommand) Name() string {
	return "pull"
}

func (c *PullCommand) Summary() string {
	return "Synchronise projects, agents, flows, and skills from NEWO to disk"
}

func (c *PullCommand) RegisterFlags(fs *flag.FlagSet) {
	c.force = fs.Bool("force", false, "overwrite local skill scripts without prompting")
	c.verbose = fs.Bool("verbose", false, "enable verbose logging")
	c.customer = fs.String("customer", "", "customer IDN to limit the pull to")
	c.projectUUID = fs.String("project-uuid", "", "restrict pull to a single project UUID")
	c.projectIDN = fs.String("project-idn", "", "restrict pull to a single project IDN")
}

func (c *PullCommand) Run(ctx context.Context, _ []string) error {
	c.ensureConsole()
	force := c.force != nil && *c.force
	verbose := c.verbose != nil && *c.verbose
	c.verboseOn = verbose
	customerFilter := ""
	if c.customer != nil {
		customerFilter = strings.TrimSpace(*c.customer)
	}

	env, err := config.LoadEnv()
	if err != nil {
		return err
	}

	projectUUIDFilter := ""
	if c.projectUUID != nil {
		projectUUIDFilter = strings.TrimSpace(*c.projectUUID)
	}

	projectIDNFilter := ""
	if c.projectIDN != nil {
		projectIDNFilter = strings.TrimSpace(*c.projectIDN)
	}

	c.outputRoot = env.OutputRoot
	c.slugPrefix = env.SlugPrefix

	cfg, err := customer.FromEnv(env)
	if err != nil {
		return err
	}

	releaseLock, err := fsutil.AcquireLock("pull")
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

	var processed bool
	var registryDirty bool

	for _, entry := range cfg.Entries {
		session, err := session.New(ctx, env, entry, registry)
		if err != nil {
			return err
		}

		if customerFilter != "" && !strings.EqualFold(session.IDN, customerFilter) {
			if verbose {
				c.console.Info("Skipping customer %s (target is %s)", session.IDN, customerFilter)
			}
			continue
		}

		// Determine the effective project IDN filter with correct precedence.
		// 1. Command-line flag
		// 2. Per-customer `project_idn` in newo.toml
		// 3. Global `project_idn` in newo.toml's [defaults]
		effectiveProjectIDN := projectIDNFilter // 1. Flag
		if effectiveProjectIDN == "" {
			effectiveProjectIDN = session.ProjectIDN // 2. Per-customer config
		}
		if effectiveProjectIDN == "" {
			effectiveProjectIDN = env.ProjectIDN // 3. Global config
		}

		if err := c.syncCustomer(ctx, session, projectUUIDFilter, effectiveProjectIDN, session.CustomerType, session.IDN, verbose, force); err != nil {
			return err
		}

		processed = true
		if session.RegistryUpdated {
			registryDirty = true
		}

		if customerFilter != "" {
			break
		}
	}

	if customerFilter != "" && !processed {
		return fmt.Errorf("customer %s not configured", customerFilter)
	}

	if registryDirty {
		if err := registry.Save(); err != nil {
			return err
		}
	}

	if !processed && verbose {
		c.console.Info("No customers matched the selection.")
	}

	return nil
}

func (c *PullCommand) syncCustomer(
	ctx context.Context,
	session *session.Session,
	projectUUIDOverride string,
	projectIDNOverride string,
	customerType string,
	customerIDN string,
	verbose bool,
	force bool,
) error {
	c.ensureConsole()
	if verbose {
		c.console.Section(fmt.Sprintf("Customer %s (%s)", session.Profile.IDN, session.Profile.ID))
	}

	if err := fsutil.EnsureWorkspace(session.IDN); err != nil {
		return fmt.Errorf("prepare workspace: %w", err)
	}

	projectMapValue, err := state.LoadProjectMap(session.IDN)
	if err != nil {
		return err
	}
	projectMap := &projectMapValue

	hashes, err := state.LoadHashes(session.IDN)
	if err != nil {
		return err
	}
	newHashes := state.HashStore{}

	var projects []platform.Project
	var pulledProjectIDs []string

	projectUUIDScope := strings.TrimSpace(projectUUIDOverride)
	projectIDNScope := strings.TrimSpace(projectIDNOverride)

	if projectUUIDScope != "" {
		project, err := session.Client.GetProject(ctx, projectUUIDScope)
		if err != nil {
			return fmt.Errorf("fetch project %s: %w", projectUUIDScope, err)
		}
		projects = []platform.Project{project}
	} else if projectIDNScope != "" {
		allProjects, err := session.Client.ListProjects(ctx)
		if err != nil {
			return fmt.Errorf("list projects: %w", err)
		}
		var foundProject *platform.Project
		for i, p := range allProjects {
			if strings.EqualFold(p.IDN, projectIDNScope) {
				foundProject = &allProjects[i]
				break
			}
		}
		if foundProject != nil {
			projects = []platform.Project{*foundProject}
		} else {
			return fmt.Errorf("project with idn %q not found for customer %s", projectIDNScope, session.IDN)
		}
	} else {
		projectScope := strings.TrimSpace(session.ProjectID)
		if projectScope != "" {
			project, err := session.Client.GetProject(ctx, projectScope)
			if err != nil {
				return fmt.Errorf("fetch project %s: %w", projectScope, err)
			}
			projects = []platform.Project{project}
		}
	}

	if len(projects) == 0 {
		if verbose {
			c.console.Info("No projects found for %s", session.IDN)
		}
		return nil
	}

	if verbose {
		c.console.Info("Pulling %d project(s) for %s", len(projects), session.IDN)
	}

	var mu sync.Mutex
	g, gCtx := errgroup.WithContext(ctx)
	g.SetLimit(4) // Limit concurrency to 4 projects at a time

	for _, project := range projects {
		project := project // https://golang.org/doc/faq#closures_and_goroutines
		g.Go(func() error {
			if err := c.pullProject(gCtx, session.Client, session.IDN, project, projectMap, hashes, newHashes, customerType, session.IDN, verbose, force, &mu); err != nil {
				return fmt.Errorf("pull project %s: %w", project.IDN, err)
			}
			mu.Lock()
			pulledProjectIDs = append(pulledProjectIDs, strings.TrimSpace(project.IDN))
			mu.Unlock()
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		return err
	}

	c.exportAttributes(ctx, session, projectMap.Projects, hashes, newHashes, session.CustomerType, session.IDN, verbose, force, &mu)

	if err := state.SaveProjectMap(session.IDN, *projectMap); err != nil {
		return err
	}
	if err := state.SaveHashes(session.IDN, newHashes); err != nil {
		return err
	}

	projectLabel := "no projects"
	if len(pulledProjectIDs) > 0 {

		unique := uniqueStrings(pulledProjectIDs)
		projectLabel = strings.Join(unique, ", ")
	}
	c.console.Success("Pull complete for %s (%s)", projectLabel, session.IDN)
	return nil
}

func (c *PullCommand) pullProject(
	ctx context.Context,
	client *platform.Client,
	customerIDN string,
	project platform.Project,
	projectMap *state.ProjectMap,
	oldHashes state.HashStore,
	newHashes state.HashStore,
	customerType string,
	customerIDNForPath string, // New parameter for path generation
	verbose bool,
	force bool,
	mu *sync.Mutex,
) error {
	c.ensureConsole()
	if verbose {
		c.console.Info("Project %s (%s)", project.Title, project.IDN)
	}

	slug := c.projectSlug(project)
	if err := os.MkdirAll(fsutil.ExportProjectDir(c.outputRoot, customerType, customerIDNForPath, slug), fsutil.DirPerm); err != nil {
		return fmt.Errorf("ensure project directory: %w", err)
	}

	projectData := state.ProjectData{
		ProjectID:  project.ID,
		ProjectIDN: project.IDN,
		Path:       slug,
		Agents:     map[string]state.AgentData{},
	}

	agents, err := client.ListAgents(ctx, project.ID)
	if err != nil {
		return fmt.Errorf("list agents: %w", err)
	}

	g, gCtx := errgroup.WithContext(ctx)
	g.SetLimit(8)

	for _, agent := range agents {
		agent := agent
		g.Go(func() error {
			return c.pullAgent(gCtx, client, customerIDN, slug, project, agent, &projectData, oldHashes, newHashes, customerType, customerIDNForPath, verbose, force, mu)
		})
	}
	if err := g.Wait(); err != nil {
		return err
	}

	if err := c.writeProjectJSON(oldHashes, newHashes, customerType, customerIDNForPath, project, slug, force, mu); err != nil {
		return err
	}

	if err := c.writeFlowsYAML(oldHashes, newHashes, customerType, customerIDNForPath, project, projectData, slug, force, mu); err != nil {
		return err
	}

	mu.Lock()
	if projectMap.Projects == nil {
		projectMap.Projects = map[string]state.ProjectData{}
	}
	projectMap.Projects[project.IDN] = projectData
	mu.Unlock()
	return nil
}

func (c *PullCommand) pullAgent(
	ctx context.Context,
	client *platform.Client,
	customerIDN string,
	projectSlug string,
	project platform.Project,
	agent platform.Agent,
	projectData *state.ProjectData,
	oldHashes state.HashStore,
	newHashes state.HashStore,
	customerType string,
	customerIDNForPath string,
	verbose bool,
	force bool,
	mu *sync.Mutex,
) error {
	c.ensureConsole()
	if verbose {
		c.console.Info("  Agent %s (%s)", agent.Title, agent.IDN)
	}

	agentData := state.AgentData{
		ID:          agent.ID,
		Title:       agent.Title,
		Description: agent.Description,
		Flows:       map[string]state.FlowData{},
	}

	g, gCtx := errgroup.WithContext(ctx)
	g.SetLimit(8)

	for _, flow := range agent.Flows {
		flow := flow
		g.Go(func() error {
			return c.pullFlow(gCtx, client, customerIDN, projectSlug, project, agent, flow, &agentData, oldHashes, newHashes, customerType, customerIDNForPath, verbose, force, mu)
		})
	}

	if err := g.Wait(); err != nil {
		return err
	}

	mu.Lock()
	projectData.Agents[agent.IDN] = agentData
	mu.Unlock()
	return nil
}

func (c *PullCommand) pullFlow(
	ctx context.Context,
	client *platform.Client,
	customerIDN string,
	projectSlug string,
	project platform.Project,
	agent platform.Agent,
	flow platform.Flow,
	agentData *state.AgentData,
	oldHashes state.HashStore,
	newHashes state.HashStore,
	customerType string,
	customerIDNForPath string,
	verbose bool,
	force bool,
	mu *sync.Mutex,
) error {
	c.ensureConsole()
	if verbose {
		c.console.Info("    Flow %s (%s)", flow.Title, flow.IDN)
	}

	events, err := client.ListFlowEvents(ctx, flow.ID)
	if err != nil {
		if apiErr, ok := err.(*platform.APIError); ok && apiErr.Status == http.StatusNotFound {
			if verbose {
				c.console.Warn("Events missing for flow %s: %v", flow.IDN, err)
			}
		} else {
			return fmt.Errorf("list flow events: %w", err)
		}
		events = nil
	}
	states, err := client.ListFlowStates(ctx, flow.ID)
	if err != nil {
		if apiErr, ok := err.(*platform.APIError); ok && apiErr.Status == http.StatusNotFound {
			if verbose {
				c.console.Warn("States missing for flow %s: %v", flow.IDN, err)
			}
		} else {
			return fmt.Errorf("list flow states: %w", err)
		}
		states = nil
	}

	skills, err := client.ListFlowSkills(ctx, flow.ID)
	if err != nil {
		return fmt.Errorf("list flow skills: %w", err)
	}

	if err := c.exportFlowMetadata(customerType, customerIDN, projectSlug, agent.IDN, flow.IDN, flow, events, states, oldHashes, newHashes, force, mu); err != nil {
		return fmt.Errorf("export flow metadata %s: %w", flow.IDN, err)
	}

	flowData := state.FlowData{
		ID:          flow.ID,
		Title:       flow.Title,
		Description: flow.Description,
		RunnerType:  flow.DefaultRunnerType,
		Model: map[string]string{
			"model_idn":    flow.DefaultModel.ModelIDN,
			"provider_idn": flow.DefaultModel.ProviderIDN,
		},
		Skills:      map[string]state.SkillMetadataInfo{},
		Events:      convertFlowEvents(events),
		StateFields: convertFlowStates(states),
	}

	var g errgroup.Group
	g.SetLimit(16)

	for _, skill := range skills {
		skill := skill
		g.Go(func() error {
			if err := c.exportSkill(customerType, customerIDN, projectSlug, agent.IDN, flow.IDN, skill, oldHashes, newHashes, force, mu); err != nil {
				return fmt.Errorf("export skill script %s: %w", skill.IDN, err)
			}
			if err := c.exportSkillMetadata(customerType, customerIDN, projectSlug, agent.IDN, flow.IDN, skill, oldHashes, newHashes, force, mu); err != nil {
				return fmt.Errorf("export skill metadata %s: %w", skill.IDN, err)
			}

			fileName := skill.IDN + "." + platform.ScriptExtension(skill.RunnerType)

			mu.Lock()
			flowData.Skills[skill.IDN] = state.SkillMetadataInfo{
				ID:         skill.ID,
				IDN:        skill.IDN,
				Title:      skill.Title,
				RunnerType: skill.RunnerType,
				Model: map[string]string{
					"model_idn":    skill.Model.ModelIDN,
					"provider_idn": skill.Model.ProviderIDN,
				},
				Parameters: parametersForMap(skill),
				Path:       filepath.ToSlash(filepath.Join("flows", flow.IDN, fileName)),
				UpdatedAt:  skill.UpdatedAt,
			}
			mu.Unlock()
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		return err
	}

	mu.Lock()
	agentData.Flows[flow.IDN] = flowData
	mu.Unlock()
	return nil
}

func (c *PullCommand) exportFlowMetadata(
	customerType, customerIDN, projectSlug, agentIDN, flowIDN string,
	flow platform.Flow,
	events []platform.FlowEvent,
	states []platform.FlowState,
	oldHashes, newHashes state.HashStore,
	force bool,
	mu *sync.Mutex,
) error {
	type flowMetadataYAML struct {
		ID                string                `yaml:"id"`
		IDN               string                `yaml:"idn"`
		Title             string                `yaml:"title"`
		Description       string                `yaml:"description,omitempty"`
		DefaultRunnerType string                `yaml:"default_runner_type"`
		DefaultModel      map[string]string     `yaml:"default_model"`
		Events            []state.FlowEventInfo `yaml:"events"`
		StateFields       []state.FlowStateInfo `yaml:"state_fields"`
	}

	meta := flowMetadataYAML{
		ID:                flow.ID,
		IDN:               flow.IDN,
		Title:             flow.Title,
		Description:       flow.Description,
		DefaultRunnerType: flow.DefaultRunnerType,
		DefaultModel: map[string]string{
			"model_idn":    flow.DefaultModel.ModelIDN,
			"provider_idn": flow.DefaultModel.ProviderIDN,
		},
		Events:      convertFlowEvents(events),
		StateFields: convertFlowStates(states),
	}

	data, err := yaml.Marshal(meta)
	if err != nil {
		return fmt.Errorf("encode flow metadata: %w", err)
	}

	path := fsutil.ExportFlowMetadataPath(c.outputRoot, customerType, customerIDN, projectSlug, agentIDN, flowIDN)
	return c.writeFileWithHash(oldHashes, newHashes, path, data, force, mu)
}

func (c *PullCommand) exportSkillMetadata(customerType, customerIDN, projectSlug, agentIDN, flowIDN string, skill platform.Skill, oldHashes, newHashes state.HashStore, force bool, mu *sync.Mutex) error {
	data, err := serialize.SkillMetadata(skill)
	if err != nil {
		return err
	}
	path := fsutil.ExportSkillMetadataPath(c.outputRoot, customerType, customerIDN, projectSlug, agentIDN, flowIDN, skill.IDN)
	return c.writeFileWithHash(oldHashes, newHashes, path, data, force, mu)
}

func parametersForMap(skill platform.Skill) []map[string]any {
	if len(skill.Parameters) == 0 {
		return nil
	}
	result := make([]map[string]any, 0, len(skill.Parameters))
	for _, param := range skill.Parameters {
		result = append(result, map[string]any{
			"name":          param.Name,
			"default_value": param.DefaultValue,
		})
	}
	return result
}

func convertFlowEvents(events []platform.FlowEvent) []state.FlowEventInfo {
	if len(events) == 0 {
		return nil
	}
	converted := make([]state.FlowEventInfo, 0, len(events))
	for _, ev := range events {
		converted = append(converted, state.FlowEventInfo{
			IDN:            ev.IDN,
			Title:          "",
			Description:    ev.Description,
			SkillSelector:  ev.SkillSelector,
			SkillIDN:       ev.SkillIDN,
			StateIDN:       ev.StateIDN,
			IntegrationIDN: ev.IntegrationIDN,
			ConnectorIDN:   ev.ConnectorIDN,
			InterruptMode:  ev.InterruptMode,
		})
	}
	return converted
}

func convertFlowStates(states []platform.FlowState) []state.FlowStateInfo {
	if len(states) == 0 {
		return nil
	}
	converted := make([]state.FlowStateInfo, 0, len(states))
	for _, st := range states {
		converted = append(converted, state.FlowStateInfo{
			ID:           st.ID,
			IDN:          st.IDN,
			Title:        st.Title,
			DefaultValue: st.DefaultValue,
			Scope:        st.Scope,
		})
	}
	return converted
}

func (c *PullCommand) exportSkill(customerType, customerIDN, projectSlug, agentIDN, flowIDN string, skill platform.Skill, oldHashes, newHashes state.HashStore, force bool, mu *sync.Mutex) error {
	fileName := skill.IDN + "." + platform.ScriptExtension(skill.RunnerType)
	path := fsutil.ExportSkillScriptPath(c.outputRoot, customerType, customerIDN, projectSlug, agentIDN, flowIDN, fileName)
	return c.writeFileWithHash(oldHashes, newHashes, path, []byte(skill.PromptScript), force, mu)
}

func (c *PullCommand) writeProjectJSON(oldHashes, newHashes state.HashStore, customerType, customerIDN string, project platform.Project, slug string, force bool, mu *sync.Mutex) error {
	content := map[string]string{
		"customer_idn":  strings.ToLower(customerIDN),
		"project_id":    project.ID,
		"project_idn":   project.IDN,
		"project_title": project.Title,
	}
	data, err := json.MarshalIndent(content, "", "  ")
	if err != nil {
		return fmt.Errorf("encode project.json: %w", err)
	}
	return c.writeFileWithHash(oldHashes, newHashes, fsutil.ExportProjectJSONPath(c.outputRoot, customerType, customerIDN, slug), data, force, mu)
}

func (c *PullCommand) writeFlowsYAML(oldHashes, newHashes state.HashStore, customerType, customerIDN string, project platform.Project, projectData state.ProjectData, slug string, force bool, mu *sync.Mutex) error {
	data, err := serialize.GenerateFlowsYAML(project, projectData)
	if err != nil {
		return err
	}
	return c.writeFileWithHash(oldHashes, newHashes, fsutil.ExportFlowsYAMLPath(c.outputRoot, customerType, customerIDN, slug), data, force, mu)
}

func (c *PullCommand) exportAttributes(
	ctx context.Context,
	session *session.Session,
	projects map[string]state.ProjectData,
	oldHashes,
	newHashes state.HashStore,
	customerType string,
	customerIDN string,
	verbose bool,
	force bool,
	mu *sync.Mutex,
) {
	c.ensureConsole()
	resp, err := session.Client.GetCustomerAttributes(ctx, true)
	if err != nil {
		if verbose {
			c.console.Warn("Fetch attributes for %s: %v", session.IDN, err)
		}
		return
	}

	data, err := serialize.GenerateAttributesYAML(resp.Attributes)
	if err != nil {
		if verbose {
			c.console.Warn("Encode attributes for %s: %v", session.IDN, err)
		}
		return
	}

	for projectIDN, projectData := range projects {
		slug := strings.TrimSpace(projectData.Path)
		if slug == "" {
			slug = c.slugPrefix + strings.ToLower(projectIDN)
		}
		if err := c.writeFileWithHash(oldHashes, newHashes, fsutil.ExportAttributesPath(c.outputRoot, customerType, customerIDN, slug), data, force, mu); err != nil {
			if verbose {
				c.console.Warn("Write attributes for %s/%s: %v", session.IDN, projectIDN, err)
			}
		}
	}
}

func (c *PullCommand) confirmOverwrite(path string, lines []diff.Line) (bool, error) {
	c.promptMu.Lock()
	defer c.promptMu.Unlock()

	c.ensureConsole()
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

func (c *PullCommand) writeFileWithHash(oldHashes, newHashes state.HashStore, path string, content []byte, force bool, mu *sync.Mutex) error {
	if newHashes == nil {
		return fmt.Errorf("hash store not initialised")
	}

	c.ensureConsole()
	normalized := filepath.ToSlash(path)
	targetHash := util.SHA256Bytes(content)
	setHash := func(value string) {
		if mu != nil {
			mu.Lock()
			newHashes[normalized] = value
			mu.Unlock()
			return
		}
		newHashes[normalized] = value
	}

	fileExists := true
	existing, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			fileExists = false
		} else {
			return fmt.Errorf("read existing %s: %w", normalized, err)
		}
	}

	existingHash := util.SHA256Bytes(existing)

	// If content is unchanged, do nothing.
	if existingHash == targetHash {
		setHash(targetHash)
		return nil
	}

	// The file on disk is different from the content we are about to write.
	// Check for uncommitted local changes first.
	if oldHash, ok := oldHashes[normalized]; ok && oldHash != existingHash {
		if !force {
			c.console.Warn("Skipping %s: local changes detected (use --force to overwrite)", normalized)
			lines := diff.Generate(existing, content, 1)
			c.console.WriteErr(diff.Format(normalized, lines))
			// Preserve previous baseline so status/push still detect divergence.
			setHash(oldHash)
			return nil
		}
	}

	// If we are here, either there are no uncommitted changes, or --force is used.
	// Now we ask for confirmation to overwrite.
	if !force && fileExists {
		context := -1 // Full diff
		if !c.verboseOn {
			context = 3
		}
		lines := diff.Generate(existing, content, context)
		confirmed, err := c.confirmOverwrite(normalized, lines)
		if err != nil {
			return err
		}
		if !confirmed {
			// We didn't write the new content, so the hash is the existing one.
			setHash(existingHash)
			return nil
		}
	}

	if err := writeFile(path, content); err != nil {
		return err
	}

	setHash(targetHash)
	return nil
}

func writeFile(path string, content []byte) error {
	if err := fsutil.EnsureParentDir(path); err != nil {
		return err
	}
	return os.WriteFile(path, content, 0o644)
}

func uniqueStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, v := range values {
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	return out
}
