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

	"github.com/twinmind/newo-tool/internal/config"
	"github.com/twinmind/newo-tool/internal/customer"
	"github.com/twinmind/newo-tool/internal/diff"
	"github.com/twinmind/newo-tool/internal/fsutil"
	"github.com/twinmind/newo-tool/internal/platform"
	"github.com/twinmind/newo-tool/internal/serialize"
	"github.com/twinmind/newo-tool/internal/session"
	"github.com/twinmind/newo-tool/internal/state"
	"github.com/twinmind/newo-tool/internal/util"
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
	stdout     io.Writer
	stderr     io.Writer
	force      *bool
	verbose    *bool
	customer   *string
	project    *string
	outputRoot string
	slugPrefix string
	verboseOn  bool
}

// NewPullCommand constructs a pull command using provided output writers.
func NewPullCommand(stdout, stderr io.Writer) *PullCommand {
	return &PullCommand{stdout: stdout, stderr: stderr}
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
	c.project = fs.String("project", "", "restrict pull to a single project UUID")
}

func (c *PullCommand) Run(ctx context.Context, _ []string) error {
	force := c.force != nil && *c.force
	verbose := c.verbose != nil && *c.verbose
	c.verboseOn = verbose
	customerFilter := ""
	if c.customer != nil {
		customerFilter = strings.TrimSpace(*c.customer)
	}
	projectFilter := ""
	if c.project != nil {
		projectFilter = strings.TrimSpace(*c.project)
	}

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

	releaseLock, err := fsutil.AcquireLock("pull")
	if err != nil {
		if errors.Is(err, fsutil.ErrLocked) {
			return fmt.Errorf("another operation is already running; please retry later")
		}
		return err
	}
	defer func() {
		if err := releaseLock(); err != nil && verbose {
			_, _ = fmt.Fprintf(c.stderr, "warning: release lock: %v\n", err)
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
				_, _ = fmt.Fprintf(c.stdout, "Skipping customer %s, target is %s\n", session.IDN, customerFilter)
			}
			continue
		}

		if err := c.syncCustomer(ctx, session, projectFilter, verbose, force); err != nil {
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
		_, _ = fmt.Fprintln(c.stdout, "No customers matched the selection")
	}

	return nil
}

func (c *PullCommand) syncCustomer(
	ctx context.Context,
	session *session.Session,
	projectOverride string,
	verbose bool,
	force bool,
) error {
	if verbose {
		_, _ = fmt.Fprintf(c.stdout, "→ Working with customer %s (%s)\n", session.Profile.IDN, session.Profile.ID)
	}

	if err := fsutil.EnsureWorkspace(session.IDN); err != nil {
		return fmt.Errorf("prepare workspace: %w", err)
	}

	projectScope := strings.TrimSpace(projectOverride)
	if projectScope == "" {
		projectScope = strings.TrimSpace(session.ProjectID)
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
	if projectScope != "" {
		project, err := session.Client.GetProject(ctx, projectScope)
		if err != nil {
			return fmt.Errorf("fetch project %s: %w", projectScope, err)
		}
		projects = []platform.Project{project}
	} else {
		projects, err = session.Client.ListProjects(ctx)
		if err != nil {
			return fmt.Errorf("list projects: %w", err)
		}
	}

	if len(projects) == 0 {
		if verbose {
			_, _ = fmt.Fprintf(c.stdout, "No projects found for %s\n", session.IDN)
		}
		return nil
	}

	if verbose {
		_, _ = fmt.Fprintf(c.stdout, "→ Pulling %d project(s) for %s\n", len(projects), session.IDN)
	}

	for _, project := range projects {
		if err := c.pullProject(ctx, session.Client, session.IDN, project, projectMap, hashes, newHashes, verbose, force); err != nil {
			return err
		}
		pulledProjectIDs = append(pulledProjectIDs, strings.TrimSpace(project.IDN))
	}

	c.exportAttributes(ctx, session, projectMap.Projects, hashes, newHashes, verbose, force)

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
	_, _ = fmt.Fprintf(c.stdout, "Pull complete for %s (%s)\n", projectLabel, session.IDN)
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
	verbose bool,
	force bool,
) error {
	if verbose {
		_, _ = fmt.Fprintf(c.stdout, "→ Project %s (%s)\n", project.Title, project.IDN)
	}

	slug := c.projectSlug(project)
	if err := os.MkdirAll(fsutil.ExportProjectDir(c.outputRoot, slug), fsutil.DirPerm); err != nil {
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

	for _, agent := range agents {
		if err := c.pullAgent(ctx, client, customerIDN, slug, project, agent, &projectData, oldHashes, newHashes, verbose, force); err != nil {
			return err
		}
	}

	if err := c.writeProjectJSON(oldHashes, newHashes, customerIDN, project, slug, force); err != nil {
		return err
	}

	if err := c.writeFlowsYAML(oldHashes, newHashes, project, projectData, slug, force); err != nil {
		return err
	}

	if projectMap.Projects == nil {
		projectMap.Projects = map[string]state.ProjectData{}
	}
	projectMap.Projects[project.IDN] = projectData
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
	verbose bool,
	force bool,
) error {
	if verbose {
		_, _ = fmt.Fprintf(c.stdout, "   → Agent %s (%s)\n", agent.Title, agent.IDN)
	}

	agentData := state.AgentData{
		ID:          agent.ID,
		Title:       agent.Title,
		Description: agent.Description,
		Flows:       map[string]state.FlowData{},
	}

	for _, flow := range agent.Flows {
		if err := c.pullFlow(ctx, client, customerIDN, projectSlug, project, agent, flow, &agentData, oldHashes, newHashes, verbose, force); err != nil {
			return err
		}
	}

	projectData.Agents[agent.IDN] = agentData
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
	verbose bool,
	force bool,
) error {
	if verbose {
		_, _ = fmt.Fprintf(c.stdout, "      → Flow %s (%s)\n", flow.Title, flow.IDN)
	}

	events, err := client.ListFlowEvents(ctx, flow.ID)
	if err != nil {
		if apiErr, ok := err.(*platform.APIError); ok && apiErr.Status == http.StatusNotFound {
			if verbose {
				_, _ = fmt.Fprintf(c.stderr, "warning: events missing for flow %s: %v\n", flow.IDN, err)
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
				_, _ = fmt.Fprintf(c.stderr, "warning: states missing for flow %s: %v\n", flow.IDN, err)
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

	for _, skill := range skills {
		if err := c.exportSkill(projectSlug, flow.IDN, skill, oldHashes, newHashes, force); err != nil {
			return err
		}

		fileName := skill.IDN + "." + platform.ScriptExtension(skill.RunnerType)
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
	}

	agentData.Flows[flow.IDN] = flowData
	return nil
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

func (c *PullCommand) exportSkill(projectSlug, flowIDN string, skill platform.Skill, oldHashes, newHashes state.HashStore, force bool) error {
	fileName := skill.IDN + "." + platform.ScriptExtension(skill.RunnerType)
	path := fsutil.ExportSkillScriptPath(c.outputRoot, projectSlug, flowIDN, fileName)
	return c.writeFileWithHash(oldHashes, newHashes, path, []byte(skill.PromptScript), force)
}

func (c *PullCommand) writeProjectJSON(oldHashes, newHashes state.HashStore, customerIDN string, project platform.Project, slug string, force bool) error {
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
	return c.writeFileWithHash(oldHashes, newHashes, fsutil.ExportProjectJSONPath(c.outputRoot, slug), data, force)
}

func (c *PullCommand) writeFlowsYAML(oldHashes, newHashes state.HashStore, project platform.Project, projectData state.ProjectData, slug string, force bool) error {
	data, err := serialize.GenerateFlowsYAML(project, projectData)
	if err != nil {
		return err
	}
	return c.writeFileWithHash(oldHashes, newHashes, fsutil.ExportFlowsYAMLPath(c.outputRoot, slug), data, force)
}

func (c *PullCommand) exportAttributes(ctx context.Context, session *session.Session, projects map[string]state.ProjectData, oldHashes, newHashes state.HashStore, verbose bool, force bool) {
	resp, err := session.Client.GetCustomerAttributes(ctx, true)
	if err != nil {
		if verbose {
			_, _ = fmt.Fprintf(c.stderr, "warning: fetch attributes for %s: %v\n", session.IDN, err)
		}
		return
	}

	data, err := serialize.GenerateAttributesYAML(resp.Attributes)
	if err != nil {
		if verbose {
			_, _ = fmt.Fprintf(c.stderr, "warning: encode attributes for %s: %v\n", session.IDN, err)
		}
		return
	}

	for projectIDN, projectData := range projects {
		slug := strings.TrimSpace(projectData.Path)
		if slug == "" {
			slug = c.slugPrefix + strings.ToLower(projectIDN)
		}
		if err := c.writeFileWithHash(oldHashes, newHashes, fsutil.ExportAttributesPath(c.outputRoot, slug), data, force); err != nil {
			if verbose {
				_, _ = fmt.Fprintf(c.stderr, "warning: write attributes for %s/%s: %v\n", session.IDN, projectIDN, err)
			}
		}
	}
}

func (c *PullCommand) writeFileWithHash(oldHashes, newHashes state.HashStore, path string, content []byte, force bool) error {
	if newHashes == nil {
		return fmt.Errorf("hash store not initialised")
	}

	normalized := filepath.ToSlash(path)
	targetHash := util.SHA256Bytes(content)

	existing, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("read existing %s: %w", normalized, err)
	}

	existingHash := util.SHA256Bytes(existing)

	// If content is unchanged, do nothing.
	if existingHash == targetHash {
		newHashes[normalized] = targetHash
		return nil
	}

	// The file on disk is different from the content we are about to write.
	// Check for uncommitted local changes first.
	if oldHash, ok := oldHashes[normalized]; ok && oldHash != existingHash {
		if !force {
			_, _ = fmt.Fprintf(c.stderr, "skipping %s: local changes detected (use --force to overwrite)\n", normalized)
			lines := diff.Generate(existing, content, 1)
			_, _ = fmt.Fprint(c.stderr, diff.Format(normalized, lines))
			// Preserve previous baseline so status/push still detect divergence.
			newHashes[normalized] = oldHash
			return nil
		}
	}

	// If we are here, either there are no uncommitted changes, or --force is used.
	// Now we ask for confirmation to overwrite.
	if !force {
		context := -1 // Full diff
		if !c.verboseOn {
			context = 3
		}
		lines := diff.Generate(existing, content, context)
		_, _ = fmt.Fprint(c.stdout, diff.Format(normalized, lines))

		_, _ = fmt.Fprintf(c.stdout, "Overwrite local file %s? [y/N]: ", normalized)
		reader := bufio.NewReader(os.Stdin)
		text, _ := reader.ReadString('\n')
		if strings.TrimSpace(strings.ToLower(text)) != "y" {
			_, _ = fmt.Fprintf(c.stdout, "Skipping overwrite.\n")
			// We didn't write the new content, so the hash is the existing one.
			newHashes[normalized] = existingHash
			return nil
		}
	}

	if err := writeFile(path, content); err != nil {
		return err
	}

	newHashes[normalized] = targetHash
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
