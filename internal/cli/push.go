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
	"github.com/twinmind/newo-tool/internal/session"
	"github.com/twinmind/newo-tool/internal/state"
	"github.com/twinmind/newo-tool/internal/util"
)

// PushCommand uploads local script changes to the NEWO platform.
type PushCommand struct {
	stdout    io.Writer
	stderr    io.Writer
	verbose   *bool
	customer  *string
	noPublish *bool
	force     *bool

	outputRoot string
	slugPrefix string
}

// NewPushCommand constructs a push command.
func NewPushCommand(stdout, stderr io.Writer) *PushCommand {
	return &PushCommand{stdout: stdout, stderr: stderr}
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
			_, _ = fmt.Fprintf(c.stderr, "warning: release lock: %v\n", err)
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
		_, _ = fmt.Fprintln(c.stdout, "No customers matched the selection. Run `newo pull` first to initialise state.")
	}

	if registryDirty {
		if err := registry.Save(); err != nil {
			return err
		}
	}

	return nil
}

func (c *PushCommand) pushCustomer(ctx context.Context, session *session.Session, shouldPublish bool, verbose bool, force bool) error {
	projectMap, err := state.LoadProjectMap(session.IDN)
	if err != nil {
		return err
	}
	if len(projectMap.Projects) == 0 {
		_, _ = fmt.Fprintf(c.stdout, "No project map for %s. Run `newo pull --customer %s` first.\n", session.IDN, session.IDN)
		return nil
	}

	oldHashes, err := state.LoadHashes(session.IDN)
	if err != nil {
		return err
	}
	if len(oldHashes) == 0 {
		_, _ = fmt.Fprintf(c.stdout, "No hash snapshot for %s. Run `newo pull --customer %s` to initialise tracking.\n", session.IDN, session.IDN)
		return nil
	}

	newHashes := make(state.HashStore, len(oldHashes))
	for path, hash := range oldHashes {
		newHashes[path] = hash
	}

	type publishTarget struct {
		projectIDN string
		agentIDN   string
		flowIDN    string
	}

	flowsToPublish := map[string]publishTarget{}
	var errs []error
	updatedSkills := 0

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
							_, _ = fmt.Fprintf(c.stderr, "skipping %s: file not found; run `newo pull` to resynchronise\n", normalized)
							continue
						}
						errs = append(errs, fmt.Errorf("read %s: %w", normalized, readErr))
						continue
					}

					if strings.TrimSpace(skillInfo.ID) == "" {
						_, _ = fmt.Fprintf(c.stderr, "skipping %s: missing remote skill identifier; run `newo pull`\n", normalized)
						continue
					}

					localScript := string(content)

					remoteSkill, found, remoteErr := c.remoteSkillSnapshot(ctx, session.Client, flowData.ID, skillInfo, flowCache)
					if remoteErr != nil {
						errs = append(errs, fmt.Errorf("verify remote skill %s: %w", normalized, remoteErr))
						continue
					}
					if !found {
						_, _ = fmt.Fprintf(c.stderr, "skipping %s: remote skill not found; run `newo pull`\n", normalized)
						errs = append(errs, fmt.Errorf("remote skill missing for %s", normalized))
						continue
					}

					remoteScript := remoteSkill.PromptScript
					remoteHash := util.SHA256String(remoteScript)

					if tracked && oldHash != "" && remoteHash != oldHash {
						_, _ = fmt.Fprintf(c.stderr, "skipping %s: remote version changed since last pull; run `newo pull`\n", normalized)
						errs = append(errs, fmt.Errorf("remote changed for %s", normalized))
						continue
					}

					currentHash := util.SHA256Bytes(content)
					if tracked && currentHash == oldHash {
						continue
					}

					if !tracked {
						_, _ = fmt.Fprintf(c.stderr, "skipping %s: not tracked in hashes; run `newo pull` to refresh mapping\n", normalized)
						continue
					}

					if !force {
						lines := diff.Generate([]byte(remoteScript), []byte(localScript), 3)
						_, _ = fmt.Fprint(c.stdout, diff.Format(normalized, lines))

						_, _ = fmt.Fprintf(c.stdout, "Push changes? [y/N]: ")
						reader := bufio.NewReader(os.Stdin)
						text, _ := reader.ReadString('\n')
						if strings.TrimSpace(strings.ToLower(text)) != "y" {
							_, _ = fmt.Fprintf(c.stdout, "Skipping.\n")
							continue
						}
					}

					if verbose {
						_, _ = fmt.Fprintf(c.stdout, "→ Updating skill %s/%s/%s\n", projectIDN, flowIDN, skillIDN)
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

				agentData.Flows[flowIDN] = flowData
			}

			projectData.Agents[agentIDN] = agentData
		}

		projectMap.Projects[projectIDN] = projectData
	}

	if updatedSkills == 0 && len(errs) == 0 {
		_, _ = fmt.Fprintf(c.stdout, "No changes to push for %s.\n", session.IDN)
		return nil
	}

	if updatedSkills > 0 {
		if verbose {
			_, _ = fmt.Fprintf(c.stdout, "Updated %d skill(s) for %s\n", updatedSkills, session.IDN)
		} else {
			_, _ = fmt.Fprintf(c.stdout, "Push complete for %s (%d skill(s) updated)\n", session.IDN, updatedSkills)
		}

		if err := state.SaveProjectMap(session.IDN, projectMap); err != nil {
			errs = append(errs, fmt.Errorf("save project map: %w", err))
		}

		if err := state.SaveHashes(session.IDN, newHashes); err != nil {
			errs = append(errs, fmt.Errorf("save hashes: %w", err))
		}

		if shouldPublish && len(flowsToPublish) > 0 {
			if verbose {
				_, _ = fmt.Fprintf(c.stdout, "Publishing %d flow(s) for %s\n", len(flowsToPublish), session.IDN)
			}
			for flowID, meta := range flowsToPublish {
				if err := session.Client.PublishFlow(ctx, flowID, defaultPublishRequest()); err != nil {
					errs = append(errs, fmt.Errorf("publish flow %s/%s/%s: %w", meta.projectIDN, meta.agentIDN, meta.flowIDN, err))
				} else if verbose {
					_, _ = fmt.Fprintf(c.stdout, "   published %s/%s/%s\n", meta.projectIDN, meta.agentIDN, meta.flowIDN)
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
