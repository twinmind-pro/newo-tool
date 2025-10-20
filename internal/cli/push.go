package cli

import (
	"bufio"
	"context"
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
	"github.com/twinmind/newo-tool/internal/ui/console"
	"github.com/twinmind/newo-tool/internal/util"
	"gopkg.in/yaml.v3"
)

type publishTarget struct {
	projectIDN string
	agentIDN   string
	flowIDN    string
}

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

	oldHashes, err := state.LoadHashes(session.IDN)
	if err != nil {
		return err
	}
	if len(oldHashes) == 0 {
		c.console.Info("No hash snapshot for %s. Run `newo pull --customer %s` to initialise tracking.", session.IDN, session.IDN)
		return nil
	}

	newHashes := make(state.HashStore, len(oldHashes))
	for path, hash := range oldHashes {
		newHashes[path] = hash
	}

	flowsToPublish := map[string]publishTarget{}
	flowsToRegenerate := map[string]string{}
	var errs []error
	updatedSkills := 0
	removedSkills := 0
	createdSkills := 0
	metadataChanged := false

	for projectIDN, projectData := range projectMap.Projects {
		projectSlug := c.projectSlug(projectIDN, projectData)

		flowCache := make(map[string]*flowSnapshot)
		for agentIDN, agentData := range projectData.Agents {
			for flowIDN, flowData := range agentData.Flows {
				for skillIDN, skillInfo := range flowData.Skills {
					entry := skillInfo
					flowData.Skills[skillIDN] = entry

					ext := platform.ScriptExtension(skillInfo.RunnerType)
					fileName := fmt.Sprintf("%s.%s", skillIDN, ext)
					scriptPath := fsutil.ExportSkillScriptPath(c.outputRoot, session.CustomerType, session.IDN, projectSlug, agentIDN, flowIDN, fileName)
					normalized := filepath.ToSlash(scriptPath)

					oldHash, tracked := oldHashes[normalized]
					content, readErr := os.ReadFile(scriptPath)
					if readErr != nil {
						if errors.Is(readErr, os.ErrNotExist) {
							if strings.TrimSpace(skillInfo.ID) == "" {
								c.console.Warn("Skipping %s: file missing and remote identifier unknown; run `newo pull`", normalized)
								continue
							}
							deleteRemote := force
							if !deleteRemote {
								confirmed, applyAll, confirmErr := c.confirmSkillDeletion(normalized, skillIDN)
								if confirmErr != nil {
									errs = append(errs, fmt.Errorf("confirm deletion %s: %w", normalized, confirmErr))
									continue
								}
								if applyAll && c.force != nil {
									*c.force = true
									force = true
								}
								deleteRemote = confirmed || applyAll
							}
							if !deleteRemote {
								continue
							}
							if verbose {
								c.console.Info("Deleting missing skill %s/%s/%s", projectIDN, flowIDN, skillIDN)
							}
							if err := session.Client.DeleteSkill(ctx, strings.TrimSpace(skillInfo.ID)); err != nil {
								errs = append(errs, fmt.Errorf("delete skill %s: %w", normalized, err))
								continue
							}
							delete(flowData.Skills, skillIDN)
							delete(newHashes, normalized)
							delete(oldHashes, normalized)
							removedSkills++
							metadataChanged = true
							flowsToRegenerate[projectIDN] = projectSlug
							c.console.Success("Deleted remote skill %s/%s/%s", projectIDN, flowIDN, skillIDN)
							continue
						}
						errs = append(errs, fmt.Errorf("read %s: %w", normalized, readErr))
						continue
					}

					if strings.TrimSpace(skillInfo.ID) == "" {
						c.console.Warn("Skipping %s: missing remote skill identifier; run `newo pull`", normalized)
						continue
					}

					localScript := string(content)

					remoteSkill, found, remoteErr := c.remoteSkillSnapshot(ctx, session.Client, flowData.ID, skillInfo, flowCache)
					if remoteErr != nil {
						errs = append(errs, fmt.Errorf("verify remote skill %s: %w", normalized, remoteErr))
						continue
					}
					if !found {
						c.console.Warn("Skipping %s: remote skill not found; run `newo pull`", normalized)
						errs = append(errs, fmt.Errorf("remote skill missing for %s", normalized))
						continue
					}

					remoteScript := remoteSkill.PromptScript
					remoteHash := util.SHA256String(remoteScript)

					if tracked && oldHash != "" && remoteHash != oldHash {
						c.console.Warn("Skipping %s: remote version changed since last pull; run `newo pull`", normalized)
						errs = append(errs, fmt.Errorf("remote changed for %s", normalized))
						continue
					}

					currentHash := util.SHA256Bytes(content)
					if tracked && currentHash == oldHash {
						continue
					}

					if !tracked {
						c.console.Warn("Skipping %s: not tracked in hashes; run `newo pull` to refresh mapping", normalized)
						continue
					}

					if !force {
						lines := diff.Generate([]byte(remoteScript), []byte(localScript), 3)
						c.console.Write(diff.Format(normalized, lines))

						c.console.Prompt("Push changes? [y/N]: ")
						reader := bufio.NewReader(os.Stdin)
						text, _ := reader.ReadString('\n')
						if strings.TrimSpace(strings.ToLower(text)) != "y" {
							c.console.Info("Skipping.")
							continue
						}
					}

					if verbose {
						c.console.Info("Updating skill %s/%s/%s", projectIDN, flowIDN, skillIDN)
					}

					if err := c.pushSkill(ctx, session.Client, remoteSkill, skillInfo, localScript); err != nil {
						errs = append(errs, fmt.Errorf("push skill %s: %w", normalized, err))
						continue
					}

					// Optimistically update the hash assuming the push was successful.
					newHashes[normalized] = currentHash
					updatedSkills++

					delete(flowCache, flowData.ID)

					if shouldPublish && strings.TrimSpace(flowData.ID) != "" {
						flowsToPublish[flowData.ID] = publishTarget{
							projectIDN: projectIDN,
							agentIDN:   agentIDN,
							flowIDN:    flowIDN,
						}
					}
				}

				flowDir := fsutil.ExportFlowDir(c.outputRoot, session.CustomerType, session.IDN, projectSlug, agentIDN, flowIDN)
				created, err := c.createMissingSkills(ctx, session, projectIDN, projectSlug, agentIDN, flowIDN, flowDir, &flowData, newHashes, verbose, flowsToPublish)
				if err != nil {
					errs = append(errs, fmt.Errorf("create skills for %s/%s/%s: %w", projectIDN, agentIDN, flowIDN, err))
				} else if created > 0 {
					createdSkills += created
					metadataChanged = true
					flowsToRegenerate[projectIDN] = projectSlug
				}
				agentData.Flows[flowIDN] = flowData
			}

			projectData.Agents[agentIDN] = agentData
		}

		projectMap.Projects[projectIDN] = projectData
	}

	if updatedSkills == 0 && removedSkills == 0 && createdSkills == 0 && len(errs) == 0 {
		c.console.Info("No changes to push for %s.", session.IDN)
		return nil
	}

	if updatedSkills > 0 {
		if verbose {
			c.console.Success("Updated %d skill(s) for %s", updatedSkills, session.IDN)
		} else {
			c.console.Success("Push complete for %s (%d skill(s) updated)", session.IDN, updatedSkills)
		}
	}
	if removedSkills > 0 {
		c.console.Success("Removed %d skill(s) for %s", removedSkills, session.IDN)
	}
	if createdSkills > 0 {
		c.console.Success("Created %d skill(s) for %s", createdSkills, session.IDN)
	}

	if updatedSkills > 0 || removedSkills > 0 || createdSkills > 0 {
		if err := state.SaveProjectMap(session.IDN, projectMap); err != nil {
			errs = append(errs, fmt.Errorf("save project map: %w", err))
		}
		if err := state.SaveHashes(session.IDN, newHashes); err != nil {
			errs = append(errs, fmt.Errorf("save hashes: %w", err))
		}
		if metadataChanged {
			for pid, slug := range flowsToRegenerate {
				data := projectMap.Projects[pid]
				if err := c.regenerateFlowsYAML(session.CustomerType, session.IDN, pid, slug, data, newHashes); err != nil {
					errs = append(errs, err)
				}
			}
		}
		if updatedSkills > 0 && shouldPublish && len(flowsToPublish) > 0 {
			if verbose {
				c.console.Info("Publishing %d flow(s) for %s", len(flowsToPublish), session.IDN)
			}
			for flowID, meta := range flowsToPublish {
				if err := session.Client.PublishFlow(ctx, flowID, defaultPublishRequest()); err != nil {
					errs = append(errs, fmt.Errorf("publish flow %s/%s/%s: %w", meta.projectIDN, meta.agentIDN, meta.flowIDN, err))
				} else if verbose {
					c.console.Info("  published %s/%s/%s", meta.projectIDN, meta.agentIDN, meta.flowIDN)
				}
			}
		}
	}

	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	return nil
}

func (c *PushCommand) pushSkill(ctx context.Context, client *platform.Client, remote platform.Skill, meta state.SkillMetadataInfo, script string) error {
	request := platform.UpdateSkillRequest{
		ID:           remote.ID,
		IDN:          choose(meta.IDN, remote.IDN),
		Title:        choose(meta.Title, remote.Title),
		PromptScript: script,
		RunnerType:   choose(meta.RunnerType, remote.RunnerType),
		Model:        mergeModel(remote.Model, meta.Model),
		Parameters:   mergeParameters(remote.Parameters, meta.Parameters),
		Path:         remote.Path,
	}
	return client.UpdateSkill(ctx, remote.ID, request)
}

func (c *PushCommand) createMissingSkills(
	ctx context.Context,
	session *session.Session,
	projectIDN, projectSlug, agentIDN, flowIDN, flowDir string,
	flowData *state.FlowData,
	newHashes state.HashStore,
	verbose bool,
	flowsToPublish map[string]publishTarget,
) (int, error) {
	entries, err := os.ReadDir(flowDir)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, fmt.Errorf("read flow directory: %w", err)
	}
	created := 0
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(name, ".meta.yaml") || name == "metadata.yaml" {
			continue
		}
		skillIDN := strings.TrimSuffix(name, ".meta.yaml")
		if _, exists := flowData.Skills[skillIDN]; exists {
			continue
		}
		metadataPath := filepath.Join(flowDir, name)
		metadataBytes, err := os.ReadFile(metadataPath)
		if err != nil {
			return created, fmt.Errorf("read metadata %s: %w", metadataPath, err)
		}
		metaDoc, err := parseSkillMetadataYAML(metadataBytes)
		if err != nil {
			return created, fmt.Errorf("decode metadata %s: %w", metadataPath, err)
		}
		if metaDoc.IDN == "" {
			metaDoc.IDN = skillIDN
		}
		title := metaDoc.Title
		if strings.TrimSpace(title) == "" {
			title = metaDoc.IDN
		}
		ext := platform.ScriptExtension(metaDoc.RunnerType)
		scriptPath := filepath.Join(flowDir, fmt.Sprintf("%s.%s", skillIDN, ext))
		scriptBytes, err := os.ReadFile(scriptPath)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return created, fmt.Errorf("read script %s: %w", scriptPath, err)
		}
		if errors.Is(err, os.ErrNotExist) {
			scriptBytes = []byte{}
		}
		if verbose {
			c.console.Info("Creating new skill %s/%s/%s", projectIDN, flowIDN, skillIDN)
		}
		if strings.TrimSpace(flowData.ID) == "" {
			c.console.Warn("Skipping %s/%s/%s: missing flow identifier", projectIDN, flowIDN, skillIDN)
			continue
		}
		createReq := platform.CreateSkillRequest{
			IDN:          metaDoc.IDN,
			Title:        title,
			PromptScript: string(scriptBytes),
			RunnerType:   metaDoc.RunnerType,
			Model: platform.ModelConfig{
				ModelIDN:    metaDoc.Model.ModelIDN,
				ProviderIDN: metaDoc.Model.ProviderIDN,
			},
			Parameters: convertParametersForAPI(metaDoc.Parameters),
		}
		resp, err := session.Client.CreateSkill(ctx, flowData.ID, createReq)
		if err != nil {
			return created, fmt.Errorf("create skill %s: %w", skillIDN, err)
		}
		created++
		platformSkill := platform.Skill{
			ID:           resp.ID,
			IDN:          metaDoc.IDN,
			Title:        title,
			PromptScript: string(scriptBytes),
			RunnerType:   metaDoc.RunnerType,
			Model:        createReq.Model,
			Parameters:   createReq.Parameters,
		}
		metaBytes, err := serialize.SkillMetadata(platformSkill)
		if err != nil {
			return created, fmt.Errorf("serialize metadata %s: %w", skillIDN, err)
		}
		if err := fsutil.EnsureParentDir(metadataPath); err != nil {
			return created, err
		}
		if err := os.WriteFile(metadataPath, metaBytes, fsutil.FilePerm); err != nil {
			return created, fmt.Errorf("write metadata %s: %w", metadataPath, err)
		}
		if err := fsutil.EnsureParentDir(scriptPath); err != nil {
			return created, err
		}
		if err := os.WriteFile(scriptPath, scriptBytes, fsutil.FilePerm); err != nil {
			return created, fmt.Errorf("write script %s: %w", scriptPath, err)
		}
		flowData.Skills[skillIDN] = state.SkillMetadataInfo{
			ID:         resp.ID,
			IDN:        metaDoc.IDN,
			Title:      title,
			RunnerType: metaDoc.RunnerType,
			Model: map[string]string{
				"model_idn":    metaDoc.Model.ModelIDN,
				"provider_idn": metaDoc.Model.ProviderIDN,
			},
			Parameters: convertParametersForState(metaDoc.Parameters),
		}
		scriptHash := util.SHA256Bytes(scriptBytes)
		metadataHash := util.SHA256Bytes(metaBytes)
		newHashes[filepath.ToSlash(scriptPath)] = scriptHash
		newHashes[filepath.ToSlash(metadataPath)] = metadataHash
		if strings.TrimSpace(flowData.ID) != "" {
			flowsToPublish[flowData.ID] = publishTarget{projectIDN: projectIDN, agentIDN: agentIDN, flowIDN: flowIDN}
		}
		c.console.Success("Created skill %s/%s/%s", projectIDN, flowIDN, skillIDN)
	}
	return created, nil
}

func (c *PushCommand) confirmSkillDeletion(path, skillIDN string) (bool, bool, error) {
	c.ensureConsole()
	c.console.Prompt("Skill %s missing locally. Delete remote version %s? [y/N/a]: ", skillIDN, path)
	reader := bufio.NewReader(os.Stdin)
	text, err := reader.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return false, false, err
	}
	switch strings.TrimSpace(strings.ToLower(text)) {
	case "y":
		return true, false, nil
	case "a":
		return true, true, nil
	default:
		c.console.Info("Keeping remote skill.")
		return false, false, nil
	}
}

func (c *PushCommand) regenerateFlowsYAML(customerType, customerIDN, projectIDN, projectSlug string, projectData state.ProjectData, hashes state.HashStore) error {
	project := platform.Project{ID: projectData.ProjectID, IDN: projectIDN, Title: projectIDN}
	content, err := serialize.GenerateFlowsYAML(project, projectData)
	if err != nil {
		return fmt.Errorf("generate flows.yaml: %w", err)
	}
	path := fsutil.ExportFlowsYAMLPath(c.outputRoot, customerType, customerIDN, projectSlug)
	if err := fsutil.EnsureParentDir(path); err != nil {
		return err
	}
	if err := os.WriteFile(path, content, fsutil.FilePerm); err != nil {
		return fmt.Errorf("write flows.yaml: %w", err)
	}
	hashes[filepath.ToSlash(path)] = util.SHA256Bytes(content)
	return nil
}

type skillMetadataDocument struct {
	ID         string                   `yaml:"id"`
	IDN        string                   `yaml:"idn"`
	Title      string                   `yaml:"title"`
	RunnerType string                   `yaml:"runner_type"`
	Model      skillMetadataModel       `yaml:"model"`
	Parameters []skillParameterMetadata `yaml:"parameters"`
}

type skillMetadataModel struct {
	ModelIDN    string `yaml:"modelidn"`
	ProviderIDN string `yaml:"provideridn"`
}

type skillParameterMetadata struct {
	Name         string      `yaml:"name"`
	DefaultValue interface{} `yaml:"default_value"`
}

func parseSkillMetadataYAML(data []byte) (skillMetadataDocument, error) {
	var doc skillMetadataDocument
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return skillMetadataDocument{}, err
	}
	return doc, nil
}

func convertParametersForAPI(params []skillParameterMetadata) []platform.SkillParameter {
	if len(params) == 0 {
		return nil
	}
	result := make([]platform.SkillParameter, 0, len(params))
	for _, p := range params {
		result = append(result, platform.SkillParameter{
			Name:         p.Name,
			DefaultValue: fmt.Sprint(p.DefaultValue),
		})
	}
	return result
}

func convertParametersForState(params []skillParameterMetadata) []map[string]any {
	if len(params) == 0 {
		return nil
	}
	result := make([]map[string]any, 0, len(params))
	for _, p := range params {
		result = append(result, map[string]any{
			"name":          p.Name,
			"default_value": p.DefaultValue,
		})
	}
	return result
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

func (c *PushCommand) remoteSkillSnapshot(ctx context.Context, client *platform.Client, flowID string, info state.SkillMetadataInfo, cache map[string]*flowSnapshot) (platform.Skill, bool, error) {
	flowID = strings.TrimSpace(flowID)
	id := strings.TrimSpace(info.ID)

	if flowID != "" {
		if snap, ok := cache[flowID]; ok && snap != nil {
			if skill, ok := snap.lookup(info); ok {
				return skill, true, nil
			}
		} else if flowID != "" {
			snap, err := buildFlowSnapshot(ctx, client, flowID)
			if err != nil {
				return platform.Skill{}, false, err
			}
			cache[flowID] = snap
			if skill, ok := snap.lookup(info); ok {
				return skill, true, nil
			}
		}
	}

	if id == "" {
		return platform.Skill{}, false, nil
	}

	skill, err := client.GetSkill(ctx, id)
	if err != nil {
		var apiErr *platform.APIError
		if errors.As(err, &apiErr) && apiErr.Status == http.StatusNotFound {
			return platform.Skill{}, false, nil
		}
		return platform.Skill{}, false, err
	}

	if flowID != "" {
		if snap, ok := cache[flowID]; ok && snap != nil {
			snap.store(skill)
		}
	}
	return skill, true, nil
}

func convertParameters(params []map[string]any) []platform.SkillParameter {
	if len(params) == 0 {
		return nil
	}
	result := make([]platform.SkillParameter, 0, len(params))
	for _, raw := range params {
		name, _ := raw["name"].(string)
		if strings.TrimSpace(name) == "" {
			continue
		}
		value := ""
		if v, ok := raw["default_value"]; ok && v != nil {
			value = fmt.Sprint(v)
		}
		result = append(result, platform.SkillParameter{
			Name:         name,
			DefaultValue: value,
		})
	}
	return result
}

func defaultPublishRequest() platform.PublishFlowRequest {
	return platform.PublishFlowRequest{
		Version:     "1.0",
		Description: "Published via newo-go CLI",
		Type:        "public",
	}
}

func choose(primary, fallback string) string {
	if v := strings.TrimSpace(primary); v != "" {
		return v
	}
	return fallback
}

func mergeModel(remote platform.ModelConfig, override map[string]string) platform.ModelConfig {
	result := remote
	if override == nil {
		return result
	}
	if v := strings.TrimSpace(override["model_idn"]); v != "" {
		result.ModelIDN = v
	}
	if v := strings.TrimSpace(override["provider_idn"]); v != "" {
		result.ProviderIDN = v
	}
	return result
}

func mergeParameters(remote []platform.SkillParameter, override []map[string]any) []platform.SkillParameter {
	if len(override) > 0 {
		return convertParameters(override)
	}
	return remote
}

type flowSnapshot struct {
	byID  map[string]platform.Skill
	byIDN map[string]platform.Skill
}

func buildFlowSnapshot(ctx context.Context, client *platform.Client, flowID string) (*flowSnapshot, error) {
	skills, err := client.ListFlowSkills(ctx, flowID)
	if err != nil {
		return nil, err
	}
	snap := &flowSnapshot{
		byID:  make(map[string]platform.Skill, len(skills)),
		byIDN: make(map[string]platform.Skill, len(skills)),
	}
	for _, s := range skills {
		if s.ID != "" {
			snap.byID[s.ID] = s
		}
		if s.IDN != "" {
			snap.byIDN[strings.ToLower(s.IDN)] = s
		}
	}
	return snap, nil
}

func (s *flowSnapshot) lookup(info state.SkillMetadataInfo) (platform.Skill, bool) {
	if s == nil {
		return platform.Skill{}, false
	}
	if info.ID != "" {
		if skill, ok := s.byID[info.ID]; ok {
			return skill, true
		}
	}
	if info.IDN != "" {
		if skill, ok := s.byIDN[strings.ToLower(info.IDN)]; ok {
			return skill, true
		}
	}
	return platform.Skill{}, false
}

func (s *flowSnapshot) store(skill platform.Skill) {
	if s == nil {
		return
	}
	if skill.ID != "" {
		s.byID[skill.ID] = skill
	}
	if skill.IDN != "" {
		s.byIDN[strings.ToLower(skill.IDN)] = skill
	}
}
