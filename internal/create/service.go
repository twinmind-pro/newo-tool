package create

import (
	"context"
	"fmt"
	"strings"

	"github.com/twinmind/newo-tool/internal/deploy"
)

// Request configures a create run.
type Request struct {
	ProjectName        string
	ProjectIDN         string
	Slug               string
	AgentIDN           string
	FlowIDNs           []string
	TargetCustomerIDN  string
	TargetCustomerType string
	OutputRoot         string
	WorkspaceDir       string
	Reporter           deploy.Reporter
	Attributes         []byte
}

// Result reuses DeployResult so callers have full context of created assets.
type Result = deploy.DeployResult

type deployExecutor interface {
	Deploy(ctx context.Context, req deploy.DeployRequest) (deploy.DeployResult, error)
}

// Service orchestrates project bootstrap via the deploy module.
type Service struct {
	deployer deployExecutor
}

// NewService constructs a creator backed by the platform deploy client.
func NewService(client deploy.DeployClient) *Service {
	return &Service{
		deployer: deploy.NewService(client),
	}
}

// newServiceWithExecutor exists for testing to bypass real API calls.
func newServiceWithExecutor(exec deployExecutor) *Service {
	return &Service{deployer: exec}
}

// Create provisions a project skeleton and materialises local artifacts.
func (s *Service) Create(ctx context.Context, req Request) (deploy.DeployResult, error) {
	if s == nil || s.deployer == nil {
		return deploy.DeployResult{}, fmt.Errorf("create service not configured")
	}

	validated, err := validateRequest(req)
	if err != nil {
		return deploy.DeployResult{}, err
	}

	plan := assemblePlan(validated)
	deployReq := deploy.DeployRequest{
		Project:            plan,
		TargetCustomerIDN:  validated.TargetCustomerIDN,
		TargetCustomerType: validated.TargetCustomerType,
		OutputRoot:         validated.OutputRoot,
		WorkspaceDir:       workspaceOrDefault(validated.WorkspaceDir),
		Reporter:           validated.Reporter,
		Attributes:         validated.Attributes,
	}

	return s.deployer.Deploy(ctx, deployReq)
}

func validateRequest(req Request) (Request, error) {
	req.ProjectName = strings.TrimSpace(req.ProjectName)
	if req.ProjectName == "" {
		return Request{}, fmt.Errorf("project name is required")
	}
	req.ProjectIDN = strings.TrimSpace(req.ProjectIDN)
	if req.ProjectIDN == "" {
		return Request{}, fmt.Errorf("project idn is required")
	}
	req.Slug = strings.TrimSpace(req.Slug)
	if req.Slug == "" {
		return Request{}, fmt.Errorf("project slug is required")
	}
	req.AgentIDN = strings.TrimSpace(req.AgentIDN)
	if req.AgentIDN == "" {
		return Request{}, fmt.Errorf("agent idn is required")
	}
	req.TargetCustomerIDN = strings.TrimSpace(req.TargetCustomerIDN)
	if req.TargetCustomerIDN == "" {
		return Request{}, fmt.Errorf("target customer idn is required")
	}
	req.TargetCustomerType = strings.TrimSpace(req.TargetCustomerType)
	if req.TargetCustomerType == "" {
		return Request{}, fmt.Errorf("target customer type is required")
	}
	req.OutputRoot = strings.TrimSpace(req.OutputRoot)
	if req.OutputRoot == "" {
		return Request{}, fmt.Errorf("output root is required")
	}
	if len(req.FlowIDNs) == 0 {
		return Request{}, fmt.Errorf("at least one flow is required")
	}

	seen := make(map[string]struct{}, len(req.FlowIDNs))
	flowIDNs := make([]string, 0, len(req.FlowIDNs))
	for _, flow := range req.FlowIDNs {
		flow = strings.TrimSpace(flow)
		if flow == "" {
			return Request{}, fmt.Errorf("flow idn cannot be empty")
		}
		key := strings.ToLower(flow)
		if _, exists := seen[key]; exists {
			return Request{}, fmt.Errorf("duplicate flow idn %s", flow)
		}
		seen[key] = struct{}{}
		flowIDNs = append(flowIDNs, flow)
	}
	req.FlowIDNs = flowIDNs

	return req, nil
}

func assemblePlan(req Request) deploy.ProjectPlan {
	agent := deploy.AgentPlan{
		IDN:         req.AgentIDN,
		Title:       req.AgentIDN,
		Description: "",
		Flows:       make([]deploy.FlowPlan, 0, len(req.FlowIDNs)),
	}
	for _, flowIDN := range req.FlowIDNs {
		agent.Flows = append(agent.Flows, deploy.FlowPlan{
			IDN:         flowIDN,
			Title:       flowIDN,
			Description: "",
		})
	}

	return deploy.ProjectPlan{
		IDN:         req.ProjectIDN,
		Title:       req.ProjectName,
		Description: "",
		Slug:        req.Slug,
		Agents:      []deploy.AgentPlan{agent},
	}
}

func workspaceOrDefault(workspace string) string {
	workspace = strings.TrimSpace(workspace)
	if workspace == "" {
		return "."
	}
	return workspace
}
