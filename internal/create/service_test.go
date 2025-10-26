package create

import (
	"context"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/twinmind/newo-tool/internal/deploy"
)

type fakeDeployer struct {
	req    deploy.DeployRequest
	result deploy.DeployResult
	err    error
	called bool
}

func (f *fakeDeployer) Deploy(ctx context.Context, req deploy.DeployRequest) (deploy.DeployResult, error) {
	f.called = true
	f.req = req
	return f.result, f.err
}

func TestServiceCreateBuildsPlan(t *testing.T) {
	exec := &fakeDeployer{
		result: deploy.DeployResult{ProjectID: "proj-123"},
	}
	svc := newServiceWithExecutor(exec)

	req := Request{
		ProjectName:        "Cal Com",
		ProjectIDN:         "calCom",
		Slug:               "cal-com",
		AgentIDN:           "CalComAgent",
		FlowIDNs:           []string{"SetupFlow", "BookingFlow"},
		TargetCustomerIDN:  "neyjadizwc",
		TargetCustomerType: "integration",
		OutputRoot:         "integrations",
		WorkspaceDir:       ".",
		Attributes:         []byte("attributes: []\n"),
	}

	result, err := svc.Create(context.Background(), req)
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	if result.ProjectID != "proj-123" {
		t.Fatalf("unexpected project id %s", result.ProjectID)
	}
	if !exec.called {
		t.Fatalf("deployer was not invoked")
	}

	if exec.req.TargetCustomerIDN != "neyjadizwc" {
		t.Fatalf("expected target IDN neyjadizwc, got %s", exec.req.TargetCustomerIDN)
	}
	if diff := cmp.Diff([]string{"SetupFlow", "BookingFlow"}, extractFlowIDs(exec.req.Project)); diff != "" {
		t.Fatalf("flows mismatch (-want +got):\n%s", diff)
	}
	if string(exec.req.Attributes) != "attributes: []\n" {
		t.Fatalf("attributes payload mismatch")
	}
}

func TestServiceCreateValidatesInput(t *testing.T) {
	svc := newServiceWithExecutor(&fakeDeployer{})
	_, err := svc.Create(context.Background(), Request{})
	if err == nil {
		t.Fatalf("expected validation error")
	}
}

func extractFlowIDs(plan deploy.ProjectPlan) []string {
	if len(plan.Agents) == 0 {
		return nil
	}
	ids := make([]string, 0, len(plan.Agents[0].Flows))
	for _, flow := range plan.Agents[0].Flows {
		ids = append(ids, flow.IDN)
	}
	return ids
}
