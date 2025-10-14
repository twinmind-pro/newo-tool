package status

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/twinmind/newo-tool/internal/state"
	"github.com/twinmind/newo-tool/internal/util"
)

func setupStatusWorkspace(t *testing.T, customer string) string {
	t.Helper()
	tmp := t.TempDir()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(tmp); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(wd)
	})
	if err := os.MkdirAll(filepath.Join(".newo", customer), 0o755); err != nil {
		t.Fatalf("mkdir .newo: %v", err)
	}
	return tmp
}

func writeProjectState(t *testing.T, customer string, project state.ProjectMap, hashes map[string]string) {
	mapPath := filepath.Join(".newo", customer, "map.json")
	data, err := json.MarshalIndent(project, "", "  ")
	if err != nil {
		t.Fatalf("marshal map: %v", err)
	}
	if err := os.WriteFile(mapPath, data, 0o644); err != nil {
		t.Fatalf("write map: %v", err)
	}

	hashPath := filepath.Join(".newo", customer, "hashes.json")
	data, err = json.MarshalIndent(hashes, "", "  ")
	if err != nil {
		t.Fatalf("marshal hashes: %v", err)
	}
	if err := os.WriteFile(hashPath, data, 0o644); err != nil {
		t.Fatalf("write hashes: %v", err)
	}
}

func TestRunCleanWorkspace(t *testing.T) {
	customer := "ACME"
	setupStatusWorkspace(t, customer)

	projectDir := filepath.Join("integrations", "calcom", "flows", "Flow")
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatalf("mkdir project: %v", err)
	}
	skillPath := filepath.Join(projectDir, "skill.nsl")
	if err := os.WriteFile(skillPath, []byte("content"), 0o644); err != nil {
		t.Fatalf("write skill: %v", err)
	}

	normalized := filepath.ToSlash(skillPath)
	project := state.ProjectMap{
		Projects: map[string]state.ProjectData{
			"calcom": {
				ProjectID:  "p",
				ProjectIDN: "calcom",
				Path:       "calcom",
				Agents: map[string]state.AgentData{
					"Agent": {
						ID:    "a",
						Title: "Agent",
						Flows: map[string]state.FlowData{
							"Flow": {
								ID:    "f",
								Title: "Flow",
								Skills: map[string]state.SkillMetadataInfo{
									"skill": {
										ID:   "s",
										IDN:  "skill",
										Path: filepath.ToSlash(filepath.Join("flows", "Flow", "skill.nsl")),
									},
								},
							},
						},
					},
				},
			},
		},
	}

	writeProjectState(t, customer, project, map[string]string{
		normalized: util.SHA256Bytes([]byte("content")),
	})

	var out bytes.Buffer
	dirty, err := Run(customer, "integrations", false, &out, &out)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if dirty != 0 {
		t.Fatalf("expected clean workspace, got %d dirty", dirty)
	}
	if got := out.String(); got != "No changes detected.\n" {
		t.Fatalf("unexpected output: %q", got)
	}
}
