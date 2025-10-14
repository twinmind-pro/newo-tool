package state

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/twinmind/newo-tool/internal/fsutil"
)

// ProjectMap mirrors the structure persisted by the legacy CLI to track IDs.
type ProjectMap struct {
	Projects map[string]ProjectData `json:"projects"`
}

// ProjectData keeps agent/flow/skill identifiers for a project.
type ProjectData struct {
	ProjectID  string               `json:"projectId"`
	ProjectIDN string               `json:"projectIdn"`
	Path       string               `json:"path"`
	Agents     map[string]AgentData `json:"agents"`
}

// AgentData keeps flow identifiers for an agent.
type AgentData struct {
	ID          string              `json:"id"`
	Title       string              `json:"title"`
	Description string              `json:"description"`
	Flows       map[string]FlowData `json:"flows"`
}

// FlowData keeps skill identifiers for a flow.
type FlowData struct {
	ID          string                       `json:"id"`
	Title       string                       `json:"title"`
	Description string                       `json:"description"`
	RunnerType  string                       `json:"runner_type"`
	Model       map[string]string            `json:"model"`
	Skills      map[string]SkillMetadataInfo `json:"skills"`
	Events      []FlowEventInfo              `json:"events"`
	StateFields []FlowStateInfo              `json:"state_fields"`
}

// SkillMetadataInfo mirrors metadata.yaml content for quick lookup.
type SkillMetadataInfo struct {
	ID         string            `json:"id"`
	IDN        string            `json:"idn"`
	Title      string            `json:"title"`
	RunnerType string            `json:"runner_type"`
	Model      map[string]string `json:"model"`
	Parameters []map[string]any  `json:"parameters"`
	Path       string            `json:"path,omitempty"`
	UpdatedAt  string            `json:"updated_at,omitempty"`
}

// FlowEventInfo captures event metadata used for flows.yaml generation.
type FlowEventInfo struct {
	IDN            string `json:"idn"`
	Title          string `json:"title"`
	Description    string `json:"description"`
	SkillSelector  string `json:"skill_selector"`
	SkillIDN       string `json:"skill_idn"`
	StateIDN       string `json:"state_idn"`
	IntegrationIDN string `json:"integration_idn"`
	ConnectorIDN   string `json:"connector_idn"`
	InterruptMode  string `json:"interrupt_mode"`
}

// FlowStateInfo captures state field metadata for flows.yaml.
type FlowStateInfo struct {
	ID           string `json:"id"`
	IDN          string `json:"idn"`
	Title        string `json:"title"`
	DefaultValue string `json:"default_value"`
	Scope        string `json:"scope"`
}

// LoadProjectMap returns the stored project map or an empty one.
func LoadProjectMap(customerIDN string) (ProjectMap, error) {
	path := fsutil.MapPath(customerIDN)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return ProjectMap{Projects: map[string]ProjectData{}}, nil
		}
		return ProjectMap{}, fmt.Errorf("read project map: %w", err)
	}

	var pm ProjectMap
	if err := json.Unmarshal(data, &pm); err != nil {
		return ProjectMap{}, fmt.Errorf("decode project map: %w", err)
	}

	if pm.Projects == nil {
		pm.Projects = map[string]ProjectData{}
	}
	return pm, nil
}

// SaveProjectMap persists the project map for the customer.
func SaveProjectMap(customerIDN string, pm ProjectMap) error {
	if pm.Projects == nil {
		pm.Projects = map[string]ProjectData{}
	}
	path := fsutil.MapPath(customerIDN)
	if err := fsutil.EnsureParentDir(path); err != nil {
		return err
	}

	data, err := json.MarshalIndent(pm, "", "  ")
	if err != nil {
		return fmt.Errorf("encode project map: %w", err)
	}

	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write project map: %w", err)
	}
	return nil
}
