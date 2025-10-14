package serialize

import (
	"testing"

	"github.com/twinmind/newo-tool/internal/platform"
	"github.com/twinmind/newo-tool/internal/state"
)

func TestGenerateFlowsYAML(t *testing.T) {
	project := platform.Project{ID: "pid", IDN: "proj"}
	data := state.ProjectData{
		ProjectID:  "pid",
		ProjectIDN: "proj",
		Agents: map[string]state.AgentData{
			"Agent": {
				Flows: map[string]state.FlowData{
					"Flow": {
						Title: "flow",
						Skills: map[string]state.SkillMetadataInfo{
							"Skill": {IDN: "Skill", Title: "Skill", Path: "flows/Flow/skill.nsl"},
						},
					},
				},
			},
		},
	}

	yaml, err := GenerateFlowsYAML(project, data)
	if err != nil {
		t.Fatalf("GenerateFlowsYAML: %v", err)
	}
	if len(yaml) == 0 {
		t.Fatalf("expected yaml output")
	}
}
