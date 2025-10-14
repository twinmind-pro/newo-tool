package serialize

import (
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/twinmind/newo-tool/internal/platform"
)

func TestProjectMetadataYAML(t *testing.T) {
	project := platform.Project{ID: "id", IDN: "proj", Title: "Title", Description: "Desc"}
	data, err := ProjectMetadata(project)
	if err != nil {
		t.Fatalf("ProjectMetadata: %v", err)
	}
	var node map[string]any
	if err := yaml.Unmarshal(data, &node); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if node["idn"] != "proj" {
		t.Fatalf("unexpected idn: %#v", node)
	}
}
