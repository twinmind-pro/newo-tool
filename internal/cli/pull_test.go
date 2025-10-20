package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/twinmind/newo-tool/internal/platform"
	"github.com/twinmind/newo-tool/internal/state"
	"github.com/twinmind/newo-tool/internal/testutil/httpmock"
	"github.com/twinmind/newo-tool/internal/util"
	"gopkg.in/yaml.v3"
)

func TestPullCommand_ProjectIDNFilter(t *testing.T) {
	// Mock server setup
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/auth/api-key/token":
			_ = json.NewEncoder(w).Encode(platform.TokenResponse{
				AccessToken:  "dummy-access-token",
				RefreshToken: "dummy-refresh-token",
			})
		case "/api/v1/customer/profile":
			_ = json.NewEncoder(w).Encode(platform.CustomerProfile{ID: "cust-123", IDN: "test-customer"})
		case "/api/v1/designer/projects":
			_ = json.NewEncoder(w).Encode([]platform.Project{
				{ID: "proj-uuid-a", IDN: "project-a", Title: "Project A"},
				{ID: "proj-uuid-b", IDN: "project-b", Title: "Project B"},
			})
		case "/api/v1/bff/agents/list":
			_ = json.NewEncoder(w).Encode([]platform.Agent{
				{
					ID:  "agent-uuid-1",
					IDN: "agent-a",
					Flows: []platform.Flow{
						{ID: "flow-uuid-1", IDN: "flow-a"},
					},
				},
			})
		case "/api/v1/designer/flows/flow-uuid-1/events":
			_ = json.NewEncoder(w).Encode([]platform.FlowEvent{
				{ID: "event-uuid-1", IDN: "user_message"},
			})
		case "/api/v1/designer/flows/flow-uuid-1/states":
			_ = json.NewEncoder(w).Encode([]platform.FlowState{})
		case "/api/v1/designer/flows/flow-uuid-1/skills":
			_ = json.NewEncoder(w).Encode([]platform.Skill{})
		case "/api/v1/bff/customer/attributes":
			_ = json.NewEncoder(w).Encode(platform.CustomerAttributesResponse{Attributes: []platform.CustomerAttribute{}})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})

	client, transport := httpmock.New(handler)
	t.Cleanup(platform.SetHTTPClientForTesting(client))
	t.Cleanup(platform.SetTransportForTesting(transport))
	baseURL := httpmock.BaseURL

	t.Run("pulls only the specified project_idn", func(t *testing.T) {
		tmp := t.TempDir()
		originalWD, _ := os.Getwd()
		if err := os.Chdir(tmp); err != nil {
			t.Fatal(err)
		}
		defer func() { _ = os.Chdir(originalWD) }()

		// Create newo.toml pointing to the mock server
		tomlContent := fmt.Sprintf(`
	[defaults]
	base_url = "%s"
	output_root = "."
	
	[[customers]]
	idn = "test-customer"
	api_key = "dummy-key"
	  [[customers.projects]]
	    idn = "project-a"
	`, baseURL)
		if err := os.WriteFile("newo.toml", []byte(tomlContent), 0o644); err != nil {
			t.Fatal(err)
		}

		cmd := NewPullCommand(&bytes.Buffer{}, &bytes.Buffer{})
		if err := cmd.Run(context.Background(), []string{}); err != nil {
			t.Fatalf("pull command failed: %v", err)
		}

		// Check that only project-a was downloaded
		if _, err := os.Stat(filepath.Join("test-customer", "project-a")); os.IsNotExist(err) {
			t.Errorf("expected 'test-customer/project-a' directory to be created, but it was not")
		}
		if _, err := os.Stat(filepath.Join("test-customer", "project-b")); !os.IsNotExist(err) {
			t.Errorf("expected 'test-customer/project-b' directory not to be created, but it was")
		}

		// Check that flow metadata was created and contains events
		flowMetaPath := filepath.Join("test-customer", "project-a", "agent-a", "flows", "flow-a", "metadata.yaml")
		if _, err := os.Stat(flowMetaPath); os.IsNotExist(err) {
			t.Fatalf("expected flow metadata file %q to be created, but it was not", flowMetaPath)
		}

		content, err := os.ReadFile(flowMetaPath)
		if err != nil {
			t.Fatalf("failed to read flow metadata file: %v", err)
		}

		var meta struct {
			Events []struct {
				IDN string `yaml:"idn"`
			} `yaml:"events"`
		}
		if err := yaml.Unmarshal(content, &meta); err != nil {
			t.Fatalf("failed to unmarshal flow metadata: %v", err)
		}

		if len(meta.Events) != 1 {
			t.Fatalf("expected 1 event, got %d", len(meta.Events))
		}
		if meta.Events[0].IDN != "user_message" {
			t.Errorf("expected event idn to be 'user_message', got %q", meta.Events[0].IDN)
		}
	})
	t.Run("returns error if project_idn not found", func(t *testing.T) {
		tmp := t.TempDir()
		originalWD, _ := os.Getwd()
		if err := os.Chdir(tmp); err != nil {
			t.Fatal(err)
		}
		defer func() { _ = os.Chdir(originalWD) }()

		// Create newo.toml with a non-existent project_idn
		tomlContent := fmt.Sprintf(`
[defaults]
base_url = "%s"
output_root = "."

[[customers]]
idn = "test-customer"
api_key = "dummy-key"
  [[customers.projects]]
  idn = "non-existent-project"
`, baseURL)
		if err := os.WriteFile("newo.toml", []byte(tomlContent), 0o644); err != nil {
			t.Fatal(err)
		}

		cmd := NewPullCommand(&bytes.Buffer{}, &bytes.Buffer{})
		err := cmd.Run(context.Background(), []string{})
		if err == nil {
			t.Fatal("expected pull command to fail, but it did not")
		}

		want := `project with idn "non-existent-project" not found for customer test-customer`
		if !strings.Contains(err.Error(), want) {
			t.Errorf("expected error message to contain %q, got %q", want, err.Error())
		}
	})
}

func TestWriteFileWithHashConflictKeepsBaseline(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "conflict.txt")

	if err := os.WriteFile(path, []byte("local-content"), 0o644); err != nil {
		t.Fatalf("write local file: %v", err)
	}

	normalized := filepath.ToSlash(path)
	oldHashes := state.HashStore{
		normalized: util.SHA256Bytes([]byte("previous-remote")),
	}
	newHashes := state.HashStore{}

	cmd := &PullCommand{stderr: &bytes.Buffer{}}

	if err := cmd.writeFileWithHash(oldHashes, newHashes, path, []byte("new-remote"), false, nil); err != nil {
		t.Fatalf("writeFileWithHash conflict: %v", err)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	if string(content) != "local-content" {
		t.Fatalf("file content overwritten, got %q", string(content))
	}

	if got, ok := newHashes[normalized]; !ok {
		t.Fatalf("expected hash entry preserved")
	} else if want := oldHashes[normalized]; got != want {
		t.Fatalf("hash changed in conflict path, want %q got %q", want, got)
	}
}

func TestWriteFileWithHashUpdatesOnSuccess(t *testing.T) {
	oldStdin := os.Stdin
	defer func() { os.Stdin = oldStdin }()
	r, w, _ := os.Pipe()
	os.Stdin = r
	_, _ = w.WriteString("y\n")
	_ = w.Close()

	tmp := t.TempDir()
	path := filepath.Join(tmp, "sync.txt")
	if err := os.WriteFile(path, []byte("old"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	normalized := filepath.ToSlash(path)
	oldHashes := state.HashStore{
		normalized: util.SHA256Bytes([]byte("old")),
	}
	newHashes := state.HashStore{}

	cmd := &PullCommand{stdout: &bytes.Buffer{}, stderr: &bytes.Buffer{}}

	if err := cmd.writeFileWithHash(oldHashes, newHashes, path, []byte("remote"), false, nil); err != nil {
		t.Fatalf("writeFileWithHash: %v", err)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	if string(content) != "remote" {
		t.Fatalf("expected remote content written, got %q", string(content))
	}

	if got, ok := newHashes[normalized]; !ok {
		t.Fatalf("missing hash entry")
	} else if want := util.SHA256Bytes([]byte("remote")); got != want {
		t.Fatalf("unexpected hash, want %q got %q", want, got)
	}
}

func TestWriteFileWithHashSkipsOnDecline(t *testing.T) {
	oldStdin := os.Stdin
	defer func() { os.Stdin = oldStdin }()
	r, w, _ := os.Pipe()
	os.Stdin = r
	_, _ = w.WriteString("n\n")
	_ = w.Close()

	tmp := t.TempDir()
	path := filepath.Join(tmp, "skip.txt")
	if err := os.WriteFile(path, []byte("keep-local"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	normalized := filepath.ToSlash(path)
	existingHash := util.SHA256Bytes([]byte("keep-local"))
	oldHashes := state.HashStore{
		normalized: existingHash,
	}
	newHashes := state.HashStore{}

	out := &bytes.Buffer{}
	cmd := &PullCommand{stdout: out, stderr: &bytes.Buffer{}}

	if err := cmd.writeFileWithHash(oldHashes, newHashes, path, []byte("remote"), false, nil); err != nil {
		t.Fatalf("writeFileWithHash: %v", err)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	if string(content) != "keep-local" {
		t.Fatalf("expected local content preserved, got %q", string(content))
	}

	if got, ok := newHashes[normalized]; !ok {
		t.Fatalf("expected hash entry recorded")
	} else if got != existingHash {
		t.Fatalf("expected hash %q, got %q", existingHash, got)
	}

	if !strings.Contains(out.String(), "Skipping overwrite.") {
		t.Fatalf("expected skip message in stdout, got %q", out.String())
	}
}
