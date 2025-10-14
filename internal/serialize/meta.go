package serialize

import (
	"fmt"

	"gopkg.in/yaml.v3"

	"github.com/twinmind/newo-tool/internal/platform"
)

type projectMetadata struct {
	ID          string `yaml:"id"`
	IDN         string `yaml:"idn"`
	Title       string `yaml:"title"`
	Description string `yaml:"description"`
	CreatedAt   string `yaml:"created_at,omitempty"`
	UpdatedAt   string `yaml:"updated_at,omitempty"`
}

type agentMetadata struct {
	ID          string `yaml:"id"`
	IDN         string `yaml:"idn"`
	Title       string `yaml:"title"`
	Description string `yaml:"description"`
}

type flowMetadata struct {
	ID            string               `yaml:"id"`
	IDN           string               `yaml:"idn"`
	Title         string               `yaml:"title"`
	Description   string               `yaml:"description"`
	DefaultRunner string               `yaml:"default_runner_type"`
	DefaultModel  platform.ModelConfig `yaml:"default_model"`
	Events        []platform.FlowEvent `yaml:"events"`
	StateFields   []platform.FlowState `yaml:"state_fields"`
}

type parameterMetadata struct {
	Name         string `yaml:"name"`
	DefaultValue string `yaml:"default_value"`
}

type skillMetadata struct {
	ID         string               `yaml:"id"`
	IDN        string               `yaml:"idn"`
	Title      string               `yaml:"title"`
	RunnerType string               `yaml:"runner_type"`
	Model      platform.ModelConfig `yaml:"model"`
	Parameters []parameterMetadata  `yaml:"parameters"`
	Path       string               `yaml:"path,omitempty"`
}

// ProjectMetadata converts a project to YAML bytes.
func ProjectMetadata(project platform.Project) ([]byte, error) {
	payload := projectMetadata{
		ID:          project.ID,
		IDN:         project.IDN,
		Title:       project.Title,
		Description: project.Description,
		CreatedAt:   project.CreatedAt,
		UpdatedAt:   project.UpdatedAt,
	}
	return marshal(payload)
}

// AgentMetadata converts an agent to YAML bytes.
func AgentMetadata(agent platform.Agent) ([]byte, error) {
	payload := agentMetadata{
		ID:          agent.ID,
		IDN:         agent.IDN,
		Title:       agent.Title,
		Description: agent.Description,
	}
	return marshal(payload)
}

// FlowMetadata converts a flow plus event/state details to YAML bytes.
func FlowMetadata(flow platform.Flow, events []platform.FlowEvent, states []platform.FlowState) ([]byte, error) {
	payload := flowMetadata{
		ID:            flow.ID,
		IDN:           flow.IDN,
		Title:         flow.Title,
		Description:   flow.Description,
		DefaultRunner: flow.DefaultRunnerType,
		DefaultModel:  flow.DefaultModel,
		Events:        events,
		StateFields:   states,
	}
	return marshal(payload)
}

// SkillMetadata converts a skill to YAML bytes.
func SkillMetadata(skill platform.Skill) ([]byte, error) {
	params := make([]parameterMetadata, 0, len(skill.Parameters))
	for _, p := range skill.Parameters {
		params = append(params, parameterMetadata{
			Name:         p.Name,
			DefaultValue: p.DefaultValue,
		})
	}

	payload := skillMetadata{
		ID:         skill.ID,
		IDN:        skill.IDN,
		Title:      skill.Title,
		RunnerType: skill.RunnerType,
		Model:      skill.Model,
		Parameters: params,
		Path:       skill.Path,
	}
	return marshal(payload)
}

func marshal(value any) ([]byte, error) {
	data, err := yaml.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("marshal yaml: %w", err)
	}
	return data, nil
}
