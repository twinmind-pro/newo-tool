package deploy

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/twinmind/newo-tool/internal/fsutil"
	"github.com/twinmind/newo-tool/internal/platform"
	"github.com/twinmind/newo-tool/internal/state"
)

// SourceConfig describes how to locate a local project for deployment.
type SourceConfig struct {
	OutputRoot   string
	CustomerType string
	CustomerIDN  string
	ProjectIDN   string
	SlugPrefix   string
}

// LoadSourceProject builds a deployment plan from the local filesystem and state metadata.
func LoadSourceProject(cfg SourceConfig) (ProjectPlan, error) {
	projectIDN := strings.TrimSpace(cfg.ProjectIDN)
	customerIDN := strings.TrimSpace(cfg.CustomerIDN)
	if projectIDN == "" {
		return ProjectPlan{}, fmt.Errorf("project idn is required")
	}
	if customerIDN == "" {
		return ProjectPlan{}, fmt.Errorf("customer idn is required")
	}

	projectMap, err := state.LoadProjectMap(customerIDN)
	if err != nil {
		return ProjectPlan{}, fmt.Errorf("load project map for %s: %w", customerIDN, err)
	}

	projectData, ok := projectMap.Projects[projectIDN]
	if !ok {
		return ProjectPlan{}, ErrProjectNotFound
	}

	slug := strings.TrimSpace(projectData.Path)
	if slug == "" {
		slug = strings.TrimSpace(projectIDN)
		if slug == "" {
			return ProjectPlan{}, fmt.Errorf("project %s missing slug in state map", projectIDN)
		}
		if cfg.SlugPrefix != "" {
			slug = cfg.SlugPrefix + strings.ToLower(slug)
		}
	}

	projectDir := filepath.Clean(fsutil.ExportProjectDir(cfg.OutputRoot, cfg.CustomerType, customerIDN, slug))
	if _, err := os.Stat(projectDir); err != nil {
		if os.IsNotExist(err) {
			return ProjectPlan{}, fmt.Errorf("%w: %s", ErrProjectDirMissing, projectDir)
		}
		return ProjectPlan{}, fmt.Errorf("stat project directory %s: %w", projectDir, err)
	}

	projectJSONPath := filepath.Clean(fsutil.ExportProjectJSONPath(cfg.OutputRoot, cfg.CustomerType, customerIDN, slug))
	projectJSON, err := readProjectJSON(projectJSONPath)
	if err != nil {
		if os.IsNotExist(err) {
			return ProjectPlan{}, fmt.Errorf("%w: %s", ErrProjectJSONMissing, projectJSONPath)
		}
		return ProjectPlan{}, err
	}

	plan := ProjectPlan{
		IDN:               projectIDN,
		Title:             fallback(strings.TrimSpace(projectJSON.ProjectTitle), projectIDN),
		Description:       "",
		Slug:              slug,
		RootDir:           projectDir,
		OriginalProjectID: strings.TrimSpace(projectData.ProjectID),
		ProjectJSONPath:   projectJSONPath,
		ProjectJSON:       projectJSON,
	}

	flowsPath := filepath.Clean(fsutil.ExportFlowsYAMLPath(cfg.OutputRoot, cfg.CustomerType, customerIDN, slug))
	if _, err := os.Stat(flowsPath); err == nil {
		plan.FlowsYAMLPath = flowsPath
	}

	attributesPath := filepath.Clean(fsutil.ExportAttributesPath(cfg.OutputRoot, cfg.CustomerType, customerIDN, slug))
	if _, err := os.Stat(attributesPath); err == nil {
		plan.AttributesPath = attributesPath
	}

	agentIDs := sortedKeys(projectData.Agents)
	plan.Agents = make([]AgentPlan, 0, len(agentIDs))
	for _, agentIDN := range agentIDs {
		agentData := projectData.Agents[agentIDN]
		agentPlan := AgentPlan{
			IDN:             agentIDN,
			Title:           fallback(agentData.Title, agentIDN),
			Description:     agentData.Description,
			OriginalAgentID: strings.TrimSpace(agentData.ID),
		}

		flowIDs := sortedFlowKeys(agentData.Flows)
		agentPlan.Flows = make([]FlowPlan, 0, len(flowIDs))
		for _, flowIDN := range flowIDs {
			flowData := agentData.Flows[flowIDN]
			flowPlan, err := buildFlowPlan(projectDir, flowIDN, flowData)
			if err != nil {
				return ProjectPlan{}, fmt.Errorf("build flow plan for %s/%s: %w", agentIDN, flowIDN, err)
			}
			agentPlan.Flows = append(agentPlan.Flows, flowPlan)
		}

		plan.Agents = append(plan.Agents, agentPlan)
	}

	return plan, nil
}

func buildFlowPlan(projectDir, flowIDN string, data state.FlowData) (FlowPlan, error) {
	model := platform.ModelConfig{
		ModelIDN:    data.Model["model_idn"],
		ProviderIDN: data.Model["provider_idn"],
	}

	flowDirRel := filepath.Join(fsutil.FlowsDir, flowIDN)
	flowDir := filepath.Join(projectDir, flowDirRel)
	flowPlan := FlowPlan{
		IDN:               flowIDN,
		Title:             fallback(data.Title, flowIDN),
		Description:       data.Description,
		DefaultRunnerType: data.RunnerType,
		DefaultModel:      model,
		OriginalFlowID:    strings.TrimSpace(data.ID),
		FlowDir:           flowDir,
		FlowDirRel:        filepath.ToSlash(flowDirRel),
		MetadataPath:      filepath.Join(flowDir, fsutil.MetadataYAML),
		MetadataRelPath:   filepath.ToSlash(filepath.Join(flowDirRel, fsutil.MetadataYAML)),
	}

	skillIDs := sortedSkillKeys(data.Skills)
	flowPlan.Skills = make([]SkillPlan, 0, len(skillIDs))
	for _, skillIDN := range skillIDs {
		meta := data.Skills[skillIDN]
		skillPlan, err := buildSkillPlan(projectDir, flowIDN, meta)
		if err != nil {
			return FlowPlan{}, fmt.Errorf("build skill plan for %s: %w", skillIDN, err)
		}
		flowPlan.Skills = append(flowPlan.Skills, skillPlan)
	}

	if len(data.Events) > 0 {
		flowPlan.Events = make([]FlowEventPlan, 0, len(data.Events))
		for _, ev := range data.Events {
			flowPlan.Events = append(flowPlan.Events, FlowEventPlan{
				IDN:            ev.IDN,
				Title:          ev.Title,
				Description:    ev.Description,
				SkillSelector:  ev.SkillSelector,
				SkillIDN:       ev.SkillIDN,
				StateIDN:       ev.StateIDN,
				IntegrationIDN: ev.IntegrationIDN,
				ConnectorIDN:   ev.ConnectorIDN,
				InterruptMode:  ev.InterruptMode,
			})
		}
	}

	if len(data.StateFields) > 0 {
		flowPlan.States = make([]FlowStatePlan, 0, len(data.StateFields))
		for _, st := range data.StateFields {
			flowPlan.States = append(flowPlan.States, FlowStatePlan{
				OriginalStateID: strings.TrimSpace(st.ID),
				IDN:             st.IDN,
				Title:           st.Title,
				DefaultValue:    st.DefaultValue,
				Scope:           st.Scope,
			})
		}
	}

	return flowPlan, nil
}

func buildSkillPlan(projectDir, flowIDN string, meta state.SkillMetadataInfo) (SkillPlan, error) {
	model := platform.ModelConfig{
		ModelIDN:    meta.Model["model_idn"],
		ProviderIDN: meta.Model["provider_idn"],
	}

	scriptRel := strings.TrimSpace(meta.Path)
	if scriptRel == "" {
		return SkillPlan{}, fmt.Errorf("skill %s missing script path", meta.IDN)
	}
	scriptPath := filepath.Join(projectDir, filepath.FromSlash(scriptRel))
	scriptBytes, err := os.ReadFile(scriptPath)
	if err != nil {
		if os.IsNotExist(err) {
			return SkillPlan{}, fmt.Errorf("%w: %s", ErrSkillScriptMissing, scriptPath)
		}
		return SkillPlan{}, fmt.Errorf("read script %s: %w", scriptPath, err)
	}

	metaRel := filepath.ToSlash(filepath.Join(fsutil.FlowsDir, flowIDN, fmt.Sprintf("%s%s", meta.IDN, fsutil.SkillMetaFileExt)))
	plan := SkillPlan{
		IDN:             meta.IDN,
		Title:           fallback(meta.Title, meta.IDN),
		RunnerType:      meta.RunnerType,
		Model:           model,
		Parameters:      convertParameters(meta.Parameters),
		OriginalSkillID: strings.TrimSpace(meta.ID),
		ScriptPath:      scriptPath,
		ScriptRelPath:   filepath.ToSlash(scriptRel),
		MetadataPath:    filepath.Join(projectDir, filepath.FromSlash(metaRel)),
		MetadataRelPath: metaRel,
		Script:          scriptBytes,
	}

	return plan, nil
}

func readProjectJSON(path string) (ProjectJSON, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return ProjectJSON{}, err
	}
	var project ProjectJSON
	if err := json.Unmarshal(data, &project); err != nil {
		return ProjectJSON{}, fmt.Errorf("parse project json %s: %w", path, err)
	}
	return project, nil
}

func convertParameters(params []map[string]any) []SkillParameterPlan {
	if len(params) == 0 {
		return nil
	}
	result := make([]SkillParameterPlan, 0, len(params))
	for _, p := range params {
		name, _ := p["name"].(string)
		value := ""
		if v, ok := p["default_value"]; ok && v != nil {
			value = fmt.Sprint(v)
		}
		result = append(result, SkillParameterPlan{
			Name:         name,
			DefaultValue: value,
		})
	}
	return result
}

func fallback(value, defaultValue string) string {
	v := strings.TrimSpace(value)
	if v != "" {
		return v
	}
	return defaultValue
}

func sortedKeys[T any](m map[string]T) []string {
	if len(m) == 0 {
		return nil
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func sortedFlowKeys(m map[string]state.FlowData) []string {
	return sortedKeys(m)
}

func sortedSkillKeys(m map[string]state.SkillMetadataInfo) []string {
	return sortedKeys(m)
}
