package platform

// Project represents high-level project metadata.
type Project struct {
	ID          string `json:"id"`
	IDN         string `json:"idn"`
	Title       string `json:"title"`
	Description string `json:"description"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
}

// Agent represents an agent belonging to a project.
type Agent struct {
	ID          string `json:"id"`
	IDN         string `json:"idn"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Flows       []Flow `json:"flows"`
}

// Flow describes a flow attached to an agent.
type Flow struct {
	ID                string      `json:"id"`
	IDN               string      `json:"idn"`
	Title             string      `json:"title"`
	Description       string      `json:"description"`
	DefaultRunnerType string      `json:"default_runner_type"`
	DefaultModel      ModelConfig `json:"default_model"`
}

// ModelConfig contains model identifiers.
type ModelConfig struct {
	ModelIDN    string `json:"model_idn"`
	ProviderIDN string `json:"provider_idn"`
}

// SkillParameter describes a named parameter on a skill.
type SkillParameter struct {
	Name         string `json:"name"`
	DefaultValue string `json:"default_value"`
}

// Skill represents a skill returned by listFlowSkills.
type Skill struct {
	ID           string           `json:"id"`
	IDN          string           `json:"idn"`
	Title        string           `json:"title"`
	PromptScript string           `json:"prompt_script"`
	RunnerType   string           `json:"runner_type"`
	Model        ModelConfig      `json:"model"`
	Parameters   []SkillParameter `json:"parameters"`
	Path         string           `json:"path"`
	UpdatedAt    string           `json:"updated_at"`
}

// FlowEvent contains metadata for flow events.
type FlowEvent struct {
	ID             string `json:"id"`
	IDN            string `json:"idn"`
	Description    string `json:"description"`
	SkillSelector  string `json:"skill_selector"`
	SkillIDN       string `json:"skill_idn"`
	StateIDN       string `json:"state_idn"`
	IntegrationIDN string `json:"integration_idn"`
	ConnectorIDN   string `json:"connector_idn"`
	InterruptMode  string `json:"interrupt_mode"`
}

// FlowState captures state fields for a flow.
type FlowState struct {
	ID           string `json:"id"`
	IDN          string `json:"idn"`
	Title        string `json:"title"`
	DefaultValue string `json:"default_value"`
	Scope        string `json:"scope"`
}

// CustomerProfile describes a NEWO customer.
type CustomerProfile struct {
	ID           string `json:"id"`
	IDN          string `json:"idn"`
	Organization string `json:"organization_name"`
	Email        string `json:"email"`
}

// CustomerAttribute describes a customer attribute entry.
type CustomerAttribute struct {
	ID             string      `json:"id"`
	IDN            string      `json:"idn"`
	Value          interface{} `json:"value"`
	Title          string      `json:"title"`
	Description    string      `json:"description"`
	Group          string      `json:"group"`
	IsHidden       bool        `json:"is_hidden"`
	PossibleValues []string    `json:"possible_values"`
	ValueType      string      `json:"value_type"`
}

// CustomerAttributesResponse wraps attributes payload from the API.
type CustomerAttributesResponse struct {
	Attributes []CustomerAttribute `json:"attributes"`
}

// UpdateSkillRequest represents the payload for updating a skill.
type UpdateSkillRequest struct {
	ID           string           `json:"id"`
	IDN          string           `json:"idn"`
	Title        string           `json:"title"`
	PromptScript string           `json:"prompt_script"`
	RunnerType   string           `json:"runner_type"`
	Model        ModelConfig      `json:"model"`
	Parameters   []SkillParameter `json:"parameters"`
	Path         string           `json:"path,omitempty"`
}

// PublishFlowRequest represents the payload used to publish a flow.
type PublishFlowRequest struct {
	Version     string `json:"version"`
	Description string `json:"description"`
	Type        string `json:"type"`
}
