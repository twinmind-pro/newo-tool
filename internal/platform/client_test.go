package platform

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/twinmind/newo-tool/internal/testutil/httpmock"
)

func testClient(t *testing.T, handler http.Handler) *Client {
	t.Helper()
	stubClient, _ := httpmock.New(handler)
	client, err := NewClient(httpmock.BaseURL, "token", WithHTTPClient(stubClient))
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	return client
}

func TestClientListProjects(t *testing.T) {
	t.Parallel()

	client := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/designer/projects" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer token" {
			t.Fatalf("missing auth header: %q", got)
		}
		_ = json.NewEncoder(w).Encode([]Project{{ID: "1", IDN: "proj"}})
	}))

	projects, err := client.ListProjects(context.Background())
	if err != nil {
		t.Fatalf("ListProjects: %v", err)
	}
	if len(projects) != 1 || projects[0].IDN != "proj" {
		t.Fatalf("unexpected projects: %#v", projects)
	}
}

func TestClientUpdateSkill(t *testing.T) {
	t.Parallel()

	called := false
	client := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Fatalf("method: %s", r.Method)
		}
		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Fatalf("content-type: %s", ct)
		}
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	payload := UpdateSkillRequest{ID: "id", PromptScript: "script"}
	if err := client.UpdateSkill(context.Background(), "id", payload); err != nil {
		t.Fatalf("UpdateSkill: %v", err)
	}
	if !called {
		t.Fatalf("expected handler to be called")
	}
}
