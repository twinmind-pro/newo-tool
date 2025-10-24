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

// CreateProjectRequest represents the payload for creating a project.
type CreateProjectRequest struct {
	IDN                 string `json:"idn"`
	Title               string `json:"title"`
	Version             string `json:"version,omitempty"`
	Description         string `json:"description,omitempty"`
	IsAutoUpdateEnabled bool   `json:"is_auto_update_enabled,omitempty"`
	RegistryIDN         string `json:"registry_idn,omitempty"`
	RegistryItemIDN     string `json:"registry_item_idn,omitempty"`
	RegistryItemVersion string `json:"registry_item_version,omitempty"`
}

// CreateProjectResponse captures identifiers for a newly created project.
type CreateProjectResponse struct {
	ID string `json:"id"`
}

// Agent represents an agent belonging to a project.
type Agent struct {
	ID          string `json:"id"`
	IDN         string `json:"idn"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Flows       []Flow `json:"flows"`
}

// CreateAgentRequest represents the payload for creating an agent.
type CreateAgentRequest struct {
	IDN         string `json:"idn"`
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
	PersonaID   string `json:"persona_id,omitempty"`
}

// CreateAgentResponse captures the identifier assigned to a new agent.
type CreateAgentResponse struct {
	ID string `json:"id"`
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

// CreateFlowRequest represents the payload for creating a flow.
type CreateFlowRequest struct {
	IDN         string `json:"idn"`
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
}

// CreateFlowResponse captures the identifier assigned to a new flow.
type CreateFlowResponse struct {
	ID string `json:"id"`
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

// CreateFlowEventRequest represents payload to create a flow event.
type CreateFlowEventRequest struct {
	IDN            string `json:"idn"`
	Description    string `json:"description,omitempty"`
	SkillSelector  string `json:"skill_selector"`
	SkillIDN       string `json:"skill_idn,omitempty"`
	StateIDN       string `json:"state_idn,omitempty"`
	InterruptMode  string `json:"interrupt_mode"`
	IntegrationIDN string `json:"integration_idn"`
	ConnectorIDN   string `json:"connector_idn"`
}

// CreateFlowEventResponse captures identifier assigned to a new flow event.
type CreateFlowEventResponse struct {
	ID string `json:"id"`
}

// FlowState captures state fields for a flow.
type FlowState struct {
	ID           string `json:"id"`
	IDN          string `json:"idn"`
	Title        string `json:"title"`
	DefaultValue string `json:"default_value"`
	Scope        string `json:"scope"`
}

// CreateFlowStateRequest represents payload to create a flow state.
type CreateFlowStateRequest struct {
	Title        string `json:"title"`
	IDN          string `json:"idn"`
	DefaultValue string `json:"default_value,omitempty"`
	Scope        string `json:"scope"`
}

// CreateFlowStateResponse captures identifier assigned to a new flow state.
type CreateFlowStateResponse struct {
	ID string `json:"id"`
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

// CreateSkillRequest represents the payload for creating a new skill on a flow.
type CreateSkillRequest struct {
	IDN          string           `json:"idn"`
	Title        string           `json:"title"`
	PromptScript string           `json:"prompt_script"`
	RunnerType   string           `json:"runner_type"`
	Model        ModelConfig      `json:"model"`
	Path         string           `json:"path,omitempty"`
	Parameters   []SkillParameter `json:"parameters,omitempty"`
}

// CreateSkillResponse captures the identifier assigned to a newly created skill.
type CreateSkillResponse struct {
	ID string `json:"id"`
}

// CreateSkillParameterRequest represents payload to create a skill parameter.
type CreateSkillParameterRequest struct {
	Name         string `json:"name"`
	DefaultValue string `json:"default_value,omitempty"`
}

// CreateSkillParameterResponse captures identifier assigned to a new parameter.
type CreateSkillParameterResponse struct {
	ID string `json:"id"`
}

// PublishFlowRequest represents the payload used to publish a flow.
type PublishFlowRequest struct {
	Version     string `json:"version"`
	Description string `json:"description"`
	Type        string `json:"type"`
}
