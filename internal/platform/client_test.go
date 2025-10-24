package platform

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
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

func TestClientCreateProject(t *testing.T) {
	t.Parallel()

	var received CreateProjectRequest
	client := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method: %s", r.Method)
		}
		if r.URL.Path != "/api/v1/designer/projects" {
			t.Fatalf("path: %s", r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
			t.Fatalf("decode: %v", err)
		}
		_ = json.NewEncoder(w).Encode(CreateProjectResponse{ID: "project-123"})
	}))

	resp, err := client.CreateProject(context.Background(), CreateProjectRequest{
		IDN:   "my-project",
		Title: "My Project",
	})
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	if resp.ID != "project-123" {
		t.Fatalf("unexpected id: %s", resp.ID)
	}
	if received.IDN != "my-project" || received.Title != "My Project" {
		t.Fatalf("unexpected payload: %#v", received)
	}
}

func TestClientDeleteProject(t *testing.T) {
	t.Parallel()

	client := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Fatalf("method: %s", r.Method)
		}
		if r.URL.Path != "/api/v1/designer/projects/project-123" {
			t.Fatalf("path: %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusNoContent)
	}))

	if err := client.DeleteProject(context.Background(), "project-123"); err != nil {
		t.Fatalf("DeleteProject: %v", err)
	}
}

func TestClientCreateAgent(t *testing.T) {
	t.Parallel()

	var received CreateAgentRequest
	client := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method: %s", r.Method)
		}
		if r.URL.Path != "/api/v2/designer/proj-1/agents" {
			t.Fatalf("path: %s", r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
			t.Fatalf("decode: %v", err)
		}
		_ = json.NewEncoder(w).Encode(CreateAgentResponse{ID: "agent-1"})
	}))

	resp, err := client.CreateAgent(context.Background(), "proj-1", CreateAgentRequest{
		IDN:   "agent-idn",
		Title: "Agent Title",
	})
	if err != nil {
		t.Fatalf("CreateAgent: %v", err)
	}
	if resp.ID != "agent-1" {
		t.Fatalf("unexpected id: %s", resp.ID)
	}
	if received.IDN != "agent-idn" {
		t.Fatalf("unexpected payload: %#v", received)
	}
}

func TestClientCreateFlow(t *testing.T) {
	t.Parallel()

	client := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method: %s", r.Method)
		}
		if r.URL.Path != "/api/v1/designer/agent-1/flows/empty" {
			t.Fatalf("path: %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusCreated)
	}))

	resp, err := client.CreateFlow(context.Background(), "agent-1", CreateFlowRequest{
		IDN:   "flow-idn",
		Title: "Flow Title",
	})
	if err != nil {
		t.Fatalf("CreateFlow: %v", err)
	}
	if resp.ID != "" {
		t.Fatalf("expected empty id for empty response body, got %q", resp.ID)
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

func TestClientCreateSkill(t *testing.T) {
	t.Parallel()

	client := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method: %s", r.Method)
		}
		if !strings.HasSuffix(r.URL.Path, "/api/v1/designer/flows/flow-123/skills") {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(CreateSkillResponse{ID: "skill-abc"})
	}))

	resp, err := client.CreateSkill(context.Background(), "flow-123", CreateSkillRequest{
		IDN:        "new_skill",
		Title:      "New Skill",
		RunnerType: "nsl",
		Model:      ModelConfig{ModelIDN: "gpt4o", ProviderIDN: "openai"},
	})
	if err != nil {
		t.Fatalf("CreateSkill: %v", err)
	}
	if resp.ID != "skill-abc" {
		t.Fatalf("unexpected id: %s", resp.ID)
	}
}

func TestClientDeleteSkill(t *testing.T) {
	client := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Fatalf("method: %s", r.Method)
		}
		if !strings.HasSuffix(r.URL.Path, "/api/v1/designer/flows/skills/skill-123") {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusNoContent)
	}))

	if err := client.DeleteSkill(context.Background(), "skill-123"); err != nil {
		t.Fatalf("DeleteSkill: %v", err)
	}
}

func TestClientCreateSkillParameter(t *testing.T) {
	t.Parallel()

	var received CreateSkillParameterRequest
	client := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method: %s", r.Method)
		}
		if r.URL.Path != "/api/v1/designer/flows/skills/skill-1/parameters" {
			t.Fatalf("path: %s", r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
			t.Fatalf("decode: %v", err)
		}
		_ = json.NewEncoder(w).Encode(CreateSkillParameterResponse{ID: "param-1"})
	}))

	resp, err := client.CreateSkillParameter(context.Background(), "skill-1", CreateSkillParameterRequest{
		Name:         "param",
		DefaultValue: "value",
	})
	if err != nil {
		t.Fatalf("CreateSkillParameter: %v", err)
	}
	if resp.ID != "param-1" {
		t.Fatalf("unexpected id: %s", resp.ID)
	}
	if received.Name != "param" || received.DefaultValue != "value" {
		t.Fatalf("unexpected payload: %#v", received)
	}
}

func TestClientCreateFlowEvent(t *testing.T) {
	t.Parallel()

	var received CreateFlowEventRequest
	client := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method: %s", r.Method)
		}
		if r.URL.Path != "/api/v1/designer/flows/flow-1/events" {
			t.Fatalf("path: %s", r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
			t.Fatalf("decode: %v", err)
		}
		_ = json.NewEncoder(w).Encode(CreateFlowEventResponse{ID: "event-1"})
	}))

	resp, err := client.CreateFlowEvent(context.Background(), "flow-1", CreateFlowEventRequest{
		IDN:            "event-idn",
		SkillSelector:  "skill_idn",
		SkillIDN:       "skill-1",
		InterruptMode:  "queue",
		IntegrationIDN: "integration",
		ConnectorIDN:   "connector",
	})
	if err != nil {
		t.Fatalf("CreateFlowEvent: %v", err)
	}
	if resp.ID != "event-1" {
		t.Fatalf("unexpected id: %s", resp.ID)
	}
	if received.IDN != "event-idn" {
		t.Fatalf("unexpected payload: %#v", received)
	}
}

func TestClientDeleteFlowEvent(t *testing.T) {
	t.Parallel()

	client := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Fatalf("method: %s", r.Method)
		}
		if r.URL.Path != "/api/v1/designer/flows/events/event-1" {
			t.Fatalf("path: %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusNoContent)
	}))

	if err := client.DeleteFlowEvent(context.Background(), "event-1"); err != nil {
		t.Fatalf("DeleteFlowEvent: %v", err)
	}
}

func TestClientCreateFlowState(t *testing.T) {
	t.Parallel()

	var received CreateFlowStateRequest
	client := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method: %s", r.Method)
		}
		if r.URL.Path != "/api/v1/designer/flows/flow-1/states" {
			t.Fatalf("path: %s", r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
			t.Fatalf("decode: %v", err)
		}
		_ = json.NewEncoder(w).Encode(CreateFlowStateResponse{ID: "state-1"})
	}))

	resp, err := client.CreateFlowState(context.Background(), "flow-1", CreateFlowStateRequest{
		Title: "State Title",
		IDN:   "state-idn",
		Scope: "flow",
	})
	if err != nil {
		t.Fatalf("CreateFlowState: %v", err)
	}
	if resp.ID != "state-1" {
		t.Fatalf("unexpected id: %s", resp.ID)
	}
	if received.IDN != "state-idn" {
		t.Fatalf("unexpected payload: %#v", received)
	}
}

func TestClientDeleteFlowState(t *testing.T) {
	t.Parallel()

	client := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Fatalf("method: %s", r.Method)
		}
		if r.URL.Path != "/api/v1/designer/flows/states/state-1" {
			t.Fatalf("path: %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusNoContent)
	}))

	if err := client.DeleteFlowState(context.Background(), "state-1"); err != nil {
		t.Fatalf("DeleteFlowState: %v", err)
	}
}
