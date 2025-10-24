package deploy

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadSourceProject(t *testing.T) {
	t.Parallel()

	chdirToRepoRoot(t)

	cfg := SourceConfig{
		OutputRoot:   "integrations",
		CustomerType: "integration",
		CustomerIDN:  "neyjadizwc",
		ProjectIDN:   "gohighlevel",
	}

	project, err := LoadSourceProject(cfg)
	if err != nil {
		t.Fatalf("LoadSourceProject: %v", err)
	}

	if project.IDN != "gohighlevel" {
		t.Fatalf("unexpected project idn: %s", project.IDN)
	}
	if project.ProjectJSON.ProjectTitle != "GoHighLevel Integration" {
		t.Fatalf("unexpected project title: %s", project.ProjectJSON.ProjectTitle)
	}
	if project.ProjectJSON.ProjectID == "" {
		t.Fatalf("expected original project id")
	}

	agent := findAgent(project, "GoHighLevelIntegration")
	if agent == nil {
		t.Fatalf("agent GoHighLevelIntegration not found")
	}

	flow := findFlow(*agent, "AvailabilityFlow")
	if flow == nil {
		t.Fatalf("flow AvailabilityFlow not found")
	}
	if flow.MetadataPath == "" {
		t.Fatalf("flow metadata path not set")
	}

	skill := findSkill(*flow, "CheckAvailabilitySkill")
	if skill == nil {
		t.Fatalf("skill CheckAvailabilitySkill not found")
	}
	if len(skill.Script) == 0 {
		t.Fatalf("skill script not loaded")
	}
	if skill.MetadataPath == "" {
		t.Fatalf("skill metadata path not set")
	}
	if skill.ScriptPath == "" {
		t.Fatalf("skill script path not set")
	}

	if len(flow.Events) == 0 {
		t.Fatalf("expected events in flow")
	}
}

func TestLoadSourceProjectMissingProject(t *testing.T) {
	t.Parallel()

	chdirToRepoRoot(t)

	_, err := LoadSourceProject(SourceConfig{
		OutputRoot:   "integrations",
		CustomerType: "integration",
		CustomerIDN:  "neyjadizwc",
		ProjectIDN:   "missing",
	})
	if !errors.Is(err, ErrProjectNotFound) {
		t.Fatalf("expected ErrProjectNotFound, got %v", err)
	}
}

func TestLoadSourceProjectMissingDirectory(t *testing.T) {
	t.Parallel()

	chdirToRepoRoot(t)

	_, err := LoadSourceProject(SourceConfig{
		OutputRoot:   "missing-root",
		CustomerType: "integration",
		CustomerIDN:  "neyjadizwc",
		ProjectIDN:   "gohighlevel",
	})
	if !errors.Is(err, ErrProjectDirMissing) {
		t.Fatalf("expected ErrProjectDirMissing, got %v", err)
	}
}

func findAgent(project ProjectPlan, idn string) *AgentPlan {
	for i := range project.Agents {
		if project.Agents[i].IDN == idn {
			return &project.Agents[i]
		}
	}
	return nil
}

func findFlow(agent AgentPlan, idn string) *FlowPlan {
	for i := range agent.Flows {
		if agent.Flows[i].IDN == idn {
			return &agent.Flows[i]
		}
	}
	return nil
}

func findSkill(flow FlowPlan, idn string) *SkillPlan {
	for i := range flow.Skills {
		if flow.Skills[i].IDN == idn {
			return &flow.Skills[i]
		}
	}
	return nil
}

func chdirToRepoRoot(t *testing.T) {
	t.Helper()

	orig, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}

	dir := orig
	for {
		if _, statErr := os.Stat(filepath.Join(dir, "go.mod")); statErr == nil {
			break
		} else if os.IsNotExist(statErr) {
			parent := filepath.Dir(dir)
			if parent == dir {
				t.Fatalf("go.mod not found in hierarchy starting from %s", orig)
			}
			dir = parent
			continue
		} else {
			t.Fatalf("stat go.mod: %v", statErr)
		}
	}

	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir to repo root: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(orig)
	})
}
