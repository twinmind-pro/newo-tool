package deploy

import "github.com/twinmind/newo-tool/internal/platform"

// ProjectJSON captures the contents of project.json.
type ProjectJSON struct {
	CustomerIDN  string `json:"customer_idn"`
	ProjectID    string `json:"project_id"`
	ProjectIDN   string `json:"project_idn"`
	ProjectTitle string `json:"project_title"`
}

// ProjectPlan describes a local project prepared for deployment.
type ProjectPlan struct {
	IDN               string
	Title             string
	Description       string
	Slug              string
	RootDir           string
	OriginalProjectID string

	ProjectJSONPath string
	FlowsYAMLPath   string
	AttributesPath  string

	ProjectJSON ProjectJSON
	Agents      []AgentPlan
}

// AgentPlan describes an agent within a project.
type AgentPlan struct {
	IDN             string
	Title           string
	Description     string
	OriginalAgentID string
	Flows           []FlowPlan
}

// FlowPlan describes a flow and the assets required to recreate it remotely.
type FlowPlan struct {
	IDN               string
	Title             string
	Description       string
	DefaultRunnerType string
	DefaultModel      platform.ModelConfig
	OriginalFlowID    string
	CreatedFlowID     string
	FlowDir           string
	FlowDirRel        string
	MetadataPath      string
	MetadataRelPath   string
	Skills            []SkillPlan
	Events            []FlowEventPlan
	States            []FlowStatePlan
}

// SkillPlan describes a skill including metadata and script content.
type SkillPlan struct {
	IDN             string
	Title           string
	RunnerType      string
	Model           platform.ModelConfig
	Parameters      []SkillParameterPlan
	OriginalSkillID string
	CreatedSkillID  string

	ScriptPath      string
	ScriptRelPath   string
	MetadataPath    string
	MetadataRelPath string
	Script          []byte
}

// SkillParameterPlan represents a parameter defined on a skill.
type SkillParameterPlan struct {
	Name         string
	DefaultValue string
}

// FlowEventPlan captures the configuration of a flow event.
type FlowEventPlan struct {
	CreatedID      string
	IDN            string
	Title          string
	Description    string
	SkillSelector  string
	SkillIDN       string
	StateIDN       string
	IntegrationIDN string
	ConnectorIDN   string
	InterruptMode  string
}

// FlowStatePlan captures the configuration of a flow state field.
type FlowStatePlan struct {
	OriginalStateID string
	CreatedStateID  string
	IDN             string
	Title           string
	DefaultValue    string
	Scope           string
}
