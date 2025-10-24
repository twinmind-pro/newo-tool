package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadToml(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.toml")
	content := `
[defaults]
output_root = "integrations"

[[customers]]
idn = "A"

  [[customers.projects]]
  idn = "proj1"

[[customers]]
idn = "B"

  [[customers.projects]]
  idn = "proj2"
`
	if err := os.WriteFile(path, []byte(strings.TrimSpace(content)), 0o644); err != nil {
		t.Fatalf("write toml: %v", err)
	}

	cfg, err := LoadToml(path)
	if err != nil {
		t.Fatalf("LoadToml: %v", err)
	}
	if len(cfg.Customers) != 2 {
		t.Fatalf("expected 2 customers, got %d", len(cfg.Customers))
	}
}

func TestSaveToml(t *testing.T) {
	cfg := TomlFile{
		Customers: []FileCustomerWritable{
			{IDN: "B", Projects: []Project{{IDN: "proj2"}}},
			{IDN: "A", Projects: []Project{{IDN: "proj1"}}},
		},
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "out.toml")
	if err := SaveToml(path, cfg); err != nil {
		t.Fatalf("SaveToml: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	content := string(data)
	firstA := strings.Index(content, "idn = \"A\"")
	firstB := strings.Index(content, "idn = \"B\"")
	if firstA == -1 || firstB == -1 || firstA > firstB {
		t.Fatalf("customers not sorted: %s", content)
	}
}

func TestAddProjectToToml(t *testing.T) {
	content := `[[customers]]
	idn = "cust"

  [[customers.projects]]
  idn = "existing"
`
	dir := t.TempDir()
	path := filepath.Join(dir, "cfg.toml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	if err := AddProjectToToml(path, "cust", "new-project", "uuid-123"); err != nil {
		t.Fatalf("AddProjectToToml: %v", err)
	}

	cfg, err := LoadToml(path)
	if err != nil {
		t.Fatalf("LoadToml: %v", err)
	}
	if len(cfg.Customers[0].Projects) != 2 {
		t.Fatalf("expected 2 projects, got %d", len(cfg.Customers[0].Projects))
	}
	if cfg.Customers[0].Projects[1].ID != "uuid-123" {
		t.Fatalf("missing project id: %#v", cfg.Customers[0].Projects[1])
	}

	// Update existing.
	if err := AddProjectToToml(path, "cust", "existing", "uuid-existing"); err != nil {
		t.Fatalf("AddProjectToToml update: %v", err)
	}
	cfg, err = LoadToml(path)
	if err != nil {
		t.Fatalf("LoadToml: %v", err)
	}
	if cfg.Customers[0].Projects[0].ID != "uuid-existing" {
		t.Fatalf("expected updated ID, got %#v", cfg.Customers[0].Projects[0])
	}
}
