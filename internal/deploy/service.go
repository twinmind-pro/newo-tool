package deploy

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/twinmind/newo-tool/internal/fsutil"
	"github.com/twinmind/newo-tool/internal/platform"
	"github.com/twinmind/newo-tool/internal/serialize"
	"github.com/twinmind/newo-tool/internal/state"
)

// DeployClient captures the platform API calls used by the deploy service.
type DeployClient interface {
	ListProjects(ctx context.Context) ([]platform.Project, error)
	CreateProject(ctx context.Context, payload platform.CreateProjectRequest) (platform.CreateProjectResponse, error)
	CreateAgent(ctx context.Context, projectID string, payload platform.CreateAgentRequest) (platform.CreateAgentResponse, error)
	CreateFlow(ctx context.Context, agentID string, payload platform.CreateFlowRequest) (platform.CreateFlowResponse, error)
	ListAgents(ctx context.Context, projectID string) ([]platform.Agent, error)
	CreateSkill(ctx context.Context, flowID string, payload platform.CreateSkillRequest) (platform.CreateSkillResponse, error)
	CreateFlowEvent(ctx context.Context, flowID string, payload platform.CreateFlowEventRequest) (platform.CreateFlowEventResponse, error)
	CreateFlowState(ctx context.Context, flowID string, payload platform.CreateFlowStateRequest) (platform.CreateFlowStateResponse, error)
}

// Reporter is used to surface progress information to callers.
type Reporter interface {
	Infof(format string, args ...any)
	Warnf(format string, args ...any)
	Successf(format string, args ...any)
}

// DeployRequest configures a deploy run.
type DeployRequest struct {
	Project            ProjectPlan
	TargetCustomerIDN  string
	TargetCustomerType string
	OutputRoot         string
	WorkspaceDir       string
	Reporter           Reporter
}

// DeployResult summarises the performed operations.
type DeployResult struct {
	ProjectID      string
	ProjectSlug    string
	AgentsCreated  int
	FlowsCreated   int
	SkillsCreated  int
	EventsCreated  int
	StatesCreated  int
	TargetRoot     string
	ProjectMap     state.ProjectMap
	Hashes         state.HashStore
	ProjectJSONRaw []byte
	FlowsYAMLRaw   []byte
}

// Service orchestrates deployment of local projects to a target customer.
type Service struct {
	client DeployClient
}

// NewService constructs a deployment service.
func NewService(client DeployClient) *Service {
	return &Service{client: client}
}

// Deploy executes the deployment workflow.
func (s *Service) Deploy(ctx context.Context, req DeployRequest) (DeployResult, error) {
	if s.client == nil {
		return DeployResult{}, fmt.Errorf("deploy client is required")
	}
	if strings.TrimSpace(req.TargetCustomerIDN) == "" {
		return DeployResult{}, fmt.Errorf("target customer idn is required")
	}
	if strings.TrimSpace(req.OutputRoot) == "" {
		return DeployResult{}, fmt.Errorf("output root is required")
	}
	if req.Project.IDN == "" {
		return DeployResult{}, fmt.Errorf("project idn is required")
	}
	if req.WorkspaceDir == "" {
		req.WorkspaceDir = "."
	}
	absWorkspace, err := filepath.Abs(req.WorkspaceDir)
	if err != nil {
		return DeployResult{}, fmt.Errorf("resolve workspace dir: %w", err)
	}

	reporter := req.Reporter
	if reporter == nil {
		reporter = noopReporter{}
	}

	reporter.Infof("Checking for existing project %q", req.Project.IDN)
	exists, err := s.projectExists(ctx, req.Project.IDN)
	if err != nil {
		return DeployResult{}, err
	}
	if exists {
		return DeployResult{}, fmt.Errorf("project %s already exists for target customer", req.Project.IDN)
	}

	reporter.Infof("Creating project %q", req.Project.IDN)
	createProjResp, err := s.client.CreateProject(ctx, platform.CreateProjectRequest{
		IDN:         req.Project.IDN,
		Title:       req.Project.Title,
		Description: req.Project.Description,
	})
	if err != nil {
		return DeployResult{}, fmt.Errorf("create project: %w", err)
	}
	projectID := strings.TrimSpace(createProjResp.ID)
	if projectID == "" {
		return DeployResult{}, fmt.Errorf("create project: empty project id returned")
	}

	targetRoot := fsutil.ExportProjectDir(req.OutputRoot, req.TargetCustomerType, req.TargetCustomerIDN, req.Project.Slug)
	if err := fsutil.EnsureDir(targetRoot); err != nil {
		return DeployResult{}, fmt.Errorf("ensure target directory %s: %w", targetRoot, err)
	}

	projectData := state.ProjectData{
		ProjectID:  projectID,
		ProjectIDN: req.Project.IDN,
		Path:       req.Project.Slug,
		Agents:     map[string]state.AgentData{},
	}

	result := DeployResult{
		ProjectID:   projectID,
		ProjectSlug: req.Project.Slug,
		TargetRoot:  targetRoot,
		Hashes:      state.HashStore{},
	}

	for _, agentPlan := range req.Project.Agents {
		reporter.Infof("Creating agent %q", agentPlan.IDN)
		agentResp, err := s.client.CreateAgent(ctx, projectID, platform.CreateAgentRequest{
			IDN:         agentPlan.IDN,
			Title:       agentPlan.Title,
			Description: agentPlan.Description,
		})
		if err != nil {
			return DeployResult{}, fmt.Errorf("create agent %s: %w", agentPlan.IDN, err)
		}
		agentID := strings.TrimSpace(agentResp.ID)
		if agentID == "" {
			return DeployResult{}, fmt.Errorf("agent %s: empty id", agentPlan.IDN)
		}
		result.AgentsCreated++

		agentData := state.AgentData{
			ID:          agentID,
			Title:       agentPlan.Title,
			Description: agentPlan.Description,
			Flows:       map[string]state.FlowData{},
		}

		// Create flows under this agent.
		for idx := range agentPlan.Flows {
			flowPlan := &agentPlan.Flows[idx]
			reporter.Infof("Creating flow %q", flowPlan.IDN)
			flowResp, err := s.client.CreateFlow(ctx, agentID, platform.CreateFlowRequest{
				IDN:   flowPlan.IDN,
				Title: flowPlan.Title,
			})
			if err != nil {
				return DeployResult{}, fmt.Errorf("create flow %s: %w", flowPlan.IDN, err)
			}
			flowPlan.CreatedFlowID = strings.TrimSpace(flowResp.ID)
			result.FlowsCreated++
		}

		// Resolve flow IDs if any were missing.
		if err := s.populateFlowIDs(ctx, projectID, agentPlan); err != nil {
			return DeployResult{}, err
		}

		for idx := range agentPlan.Flows {
			flowPlan := &agentPlan.Flows[idx]
			if flowPlan.CreatedFlowID == "" {
				return DeployResult{}, fmt.Errorf("flow %s: missing identifier after creation", flowPlan.IDN)
			}

			flowData := state.FlowData{
				ID:          flowPlan.CreatedFlowID,
				Title:       flowPlan.Title,
				Description: flowPlan.Description,
				RunnerType:  flowPlan.DefaultRunnerType,
				Model: map[string]string{
					"model_idn":    flowPlan.DefaultModel.ModelIDN,
					"provider_idn": flowPlan.DefaultModel.ProviderIDN,
				},
				Skills:      map[string]state.SkillMetadataInfo{},
				Events:      []state.FlowEventInfo{},
				StateFields: []state.FlowStateInfo{},
			}

			// Create skills
			for sidx := range flowPlan.Skills {
				skillPlan := &flowPlan.Skills[sidx]
				reporter.Infof("Creating skill %q/%q/%q", agentPlan.IDN, flowPlan.IDN, skillPlan.IDN)
				createReq := platform.CreateSkillRequest{
					IDN:          skillPlan.IDN,
					Title:        skillPlan.Title,
					PromptScript: string(skillPlan.Script),
					RunnerType:   skillPlan.RunnerType,
					Model: platform.ModelConfig{
						ModelIDN:    skillPlan.Model.ModelIDN,
						ProviderIDN: skillPlan.Model.ProviderIDN,
					},
					Path:       "",
					Parameters: convertParametersToPlatform(skillPlan.Parameters),
				}
				skillResp, err := s.client.CreateSkill(ctx, flowPlan.CreatedFlowID, createReq)
				if err != nil {
					return DeployResult{}, fmt.Errorf("create skill %s: %w", skillPlan.IDN, err)
				}
				skillPlan.CreatedSkillID = strings.TrimSpace(skillResp.ID)
				if skillPlan.CreatedSkillID == "" {
					return DeployResult{}, fmt.Errorf("skill %s: empty id returned", skillPlan.IDN)
				}
				result.SkillsCreated++

				flowData.Skills[skillPlan.IDN] = state.SkillMetadataInfo{
					ID:         skillPlan.CreatedSkillID,
					IDN:        skillPlan.IDN,
					Title:      skillPlan.Title,
					RunnerType: skillPlan.RunnerType,
					Model: map[string]string{
						"model_idn":    skillPlan.Model.ModelIDN,
						"provider_idn": skillPlan.Model.ProviderIDN,
					},
					Parameters: convertParametersToState(skillPlan.Parameters),
					Path:       skillPlan.ScriptRelPath,
				}
			}

			// Create flow events
			for eidx := range flowPlan.Events {
				eventPlan := &flowPlan.Events[eidx]
				reporter.Infof("Creating event %q on flow %q", eventPlan.IDN, flowPlan.IDN)
				resp, err := s.client.CreateFlowEvent(ctx, flowPlan.CreatedFlowID, platform.CreateFlowEventRequest{
					IDN:            eventPlan.IDN,
					Description:    eventPlan.Description,
					SkillSelector:  eventPlan.SkillSelector,
					SkillIDN:       eventPlan.SkillIDN,
					StateIDN:       eventPlan.StateIDN,
					InterruptMode:  eventPlan.InterruptMode,
					IntegrationIDN: eventPlan.IntegrationIDN,
					ConnectorIDN:   eventPlan.ConnectorIDN,
				})
				if err != nil {
					return DeployResult{}, fmt.Errorf("create event %s: %w", eventPlan.IDN, err)
				}
				eventPlan.CreatedID = strings.TrimSpace(resp.ID)
				result.EventsCreated++

				flowData.Events = append(flowData.Events, state.FlowEventInfo{
					IDN:            eventPlan.IDN,
					Title:          eventPlan.Title,
					Description:    eventPlan.Description,
					SkillSelector:  eventPlan.SkillSelector,
					SkillIDN:       eventPlan.SkillIDN,
					StateIDN:       eventPlan.StateIDN,
					IntegrationIDN: eventPlan.IntegrationIDN,
					ConnectorIDN:   eventPlan.ConnectorIDN,
					InterruptMode:  eventPlan.InterruptMode,
				})
			}

			// Create flow states
			for sidx := range flowPlan.States {
				statePlan := &flowPlan.States[sidx]
				reporter.Infof("Creating state %q on flow %q", statePlan.IDN, flowPlan.IDN)
				resp, err := s.client.CreateFlowState(ctx, flowPlan.CreatedFlowID, platform.CreateFlowStateRequest{
					Title:        statePlan.Title,
					IDN:          statePlan.IDN,
					DefaultValue: statePlan.DefaultValue,
					Scope:        statePlan.Scope,
				})
				if err != nil {
					return DeployResult{}, fmt.Errorf("create state %s: %w", statePlan.IDN, err)
				}
				statePlan.CreatedStateID = strings.TrimSpace(resp.ID)
				result.StatesCreated++

				flowData.StateFields = append(flowData.StateFields, state.FlowStateInfo{
					ID:           statePlan.CreatedStateID,
					IDN:          statePlan.IDN,
					Title:        statePlan.Title,
					DefaultValue: statePlan.DefaultValue,
					Scope:        statePlan.Scope,
				})
			}

			agentData.Flows[flowPlan.IDN] = flowData
		}

		projectData.Agents[agentPlan.IDN] = agentData
	}

	projectMap := state.ProjectMap{Projects: map[string]state.ProjectData{
		req.Project.IDN: projectData,
	}}

	result.ProjectMap = projectMap

	// Write local files and compute hashes.
	if err := s.writeLocalArtifacts(projectID, req, projectData, absWorkspace, &result); err != nil {
		return DeployResult{}, err
	}

	// Persist state.
	if err := fsutil.EnsureWorkspace(req.TargetCustomerIDN); err != nil {
		return DeployResult{}, fmt.Errorf("ensure workspace: %w", err)
	}
	if err := state.SaveProjectMap(req.TargetCustomerIDN, projectMap); err != nil {
		return DeployResult{}, fmt.Errorf("save project map: %w", err)
	}
	if err := state.SaveHashes(req.TargetCustomerIDN, result.Hashes); err != nil {
		return DeployResult{}, fmt.Errorf("save hashes: %w", err)
	}

	reporter.Successf("Deployment completed: project %s (%s)", req.Project.IDN, projectID)
	return result, nil
}

func (s *Service) projectExists(ctx context.Context, projectIDN string) (bool, error) {
	projects, err := s.client.ListProjects(ctx)
	if err != nil {
		return false, fmt.Errorf("list projects: %w", err)
	}
	for _, project := range projects {
		if strings.EqualFold(strings.TrimSpace(project.IDN), projectIDN) {
			return true, nil
		}
	}
	return false, nil
}

func (s *Service) populateFlowIDs(ctx context.Context, projectID string, agentPlan AgentPlan) error {
	needsResolution := false
	for _, flow := range agentPlan.Flows {
		if strings.TrimSpace(flow.CreatedFlowID) == "" {
			needsResolution = true
			break
		}
	}
	if !needsResolution {
		return nil
	}
	agents, err := s.client.ListAgents(ctx, projectID)
	if err != nil {
		return fmt.Errorf("list agents: %w", err)
	}

	var remoteAgent *platform.Agent
	for idx := range agents {
		if strings.EqualFold(strings.TrimSpace(agents[idx].IDN), agentPlan.IDN) {
			remoteAgent = &agents[idx]
			break
		}
	}
	if remoteAgent == nil {
		return fmt.Errorf("agent %s not found when resolving flow identifiers", agentPlan.IDN)
	}

	for idx := range agentPlan.Flows {
		flowPlan := &agentPlan.Flows[idx]
		if strings.TrimSpace(flowPlan.CreatedFlowID) != "" {
			continue
		}
		found := ""
		for _, remoteFlow := range remoteAgent.Flows {
			if strings.EqualFold(strings.TrimSpace(remoteFlow.IDN), flowPlan.IDN) {
				found = strings.TrimSpace(remoteFlow.ID)
				flowPlan.DefaultRunnerType = remoteFlow.DefaultRunnerType
				flowPlan.DefaultModel = remoteFlow.DefaultModel
				break
			}
		}
		if found == "" {
			return fmt.Errorf("flow %s not returned by ListAgents", flowPlan.IDN)
		}
		flowPlan.CreatedFlowID = found
	}
	return nil
}

func (s *Service) writeLocalArtifacts(projectID string, req DeployRequest, projectData state.ProjectData, workspace string, result *DeployResult) error {
	targetRoot := result.TargetRoot

	if err := fsutil.EnsureDir(targetRoot); err != nil {
		return fmt.Errorf("ensure target dir %s: %w", targetRoot, err)
	}

	// Write project.json
	projectJSON := ProjectJSON{
		CustomerIDN:  strings.ToLower(req.TargetCustomerIDN),
		ProjectID:    projectID,
		ProjectIDN:   req.Project.IDN,
		ProjectTitle: req.Project.Title,
	}
	projectJSONBytes, err := json.MarshalIndent(projectJSON, "", "  ")
	if err != nil {
		return fmt.Errorf("encode project json: %w", err)
	}
	projectJSONPath := filepath.Join(targetRoot, fsutil.ProjectJSON)
	if err := writeFile(projectJSONPath, projectJSONBytes); err != nil {
		return fmt.Errorf("write project.json: %w", err)
	}
	hashKey, err := workspaceRelative(workspace, projectJSONPath)
	if err != nil {
		return err
	}
	result.Hashes[hashKey] = hashBytes(projectJSONBytes)
	result.ProjectJSONRaw = projectJSONBytes

	// Write flow directories, scripts, metadata.
	for agentIDN, agent := range projectData.Agents {
		for flowIDN, flow := range agent.Flows {
			flowDir := fsutil.ExportFlowDir(req.OutputRoot, req.TargetCustomerType, req.TargetCustomerIDN, req.Project.Slug, agentIDN, flowIDN)
			if err := fsutil.EnsureDir(flowDir); err != nil {
				return fmt.Errorf("ensure flow dir %s: %w", flowDir, err)
			}

			flowMetaBytes, err := serialize.FlowMetadata(platform.Flow{
				ID:                flow.ID,
				IDN:               flowIDN,
				Title:             flow.Title,
				Description:       flow.Description,
				DefaultRunnerType: flow.RunnerType,
				DefaultModel: platform.ModelConfig{
					ModelIDN:    flow.Model["model_idn"],
					ProviderIDN: flow.Model["provider_idn"],
				},
			}, convertEventsToPlatform(flow.Events), convertStatesToPlatform(flow.StateFields))
			if err != nil {
				return fmt.Errorf("serialize flow metadata for %s: %w", flowIDN, err)
			}

			flowMetaPath := filepath.Join(flowDir, fsutil.MetadataYAML)
			if err := writeFile(flowMetaPath, flowMetaBytes); err != nil {
				return fmt.Errorf("write flow metadata %s: %w", flowMetaPath, err)
			}
			hashKey, err = workspaceRelative(workspace, flowMetaPath)
			if err != nil {
				return err
			}
			result.Hashes[hashKey] = hashBytes(flowMetaBytes)

			for skillIDN, skill := range flow.Skills {
				var scriptBytes []byte
				var metaBytes []byte

				// Retrieve script bytes from source plan (project plan) if available.
				planSkill := findSkillPlan(req.Project, agentIDN, flowIDN, skillIDN)
				if planSkill == nil {
					return fmt.Errorf("skill plan %s not found when writing files", skillIDN)
				}
				scriptBytes = planSkill.Script

				ext := scriptExtension(planSkill.ScriptRelPath)
				scriptPath := filepath.Join(flowDir, fmt.Sprintf("%s.%s", skillIDN, ext))
				if err := writeFile(scriptPath, scriptBytes); err != nil {
					return fmt.Errorf("write script %s: %w", scriptPath, err)
				}
				hashKey, err = workspaceRelative(workspace, scriptPath)
				if err != nil {
					return err
				}
				result.Hashes[hashKey] = hashBytes(scriptBytes)

				metaBytes, err = serialize.SkillMetadata(platform.Skill{
					ID:         skill.ID,
					IDN:        skill.IDN,
					Title:      skill.Title,
					RunnerType: skill.RunnerType,
					Model: platform.ModelConfig{
						ModelIDN:    skill.Model["model_idn"],
						ProviderIDN: skill.Model["provider_idn"],
					},
					Parameters: convertParametersInfo(skill.Parameters),
					Path:       planSkill.ScriptRelPath,
				})
				if err != nil {
					return fmt.Errorf("serialize skill metadata for %s: %w", skillIDN, err)
				}
				metaPath := filepath.Join(flowDir, fmt.Sprintf("%s%s", skillIDN, fsutil.SkillMetaFileExt))
				if err := writeFile(metaPath, metaBytes); err != nil {
					return fmt.Errorf("write skill metadata %s: %w", metaPath, err)
				}
				hashKey, err = workspaceRelative(workspace, metaPath)
				if err != nil {
					return err
				}
				result.Hashes[hashKey] = hashBytes(metaBytes)
			}
		}
	}

	// Generate flows.yaml
	project := platform.Project{
		ID:          projectID,
		IDN:         req.Project.IDN,
		Title:       req.Project.Title,
		Description: req.Project.Description,
	}
	flowsYAML, err := serialize.GenerateFlowsYAML(project, projectData)
	if err != nil {
		return fmt.Errorf("generate flows.yaml: %w", err)
	}
	flowsPath := filepath.Join(targetRoot, fsutil.FlowsYAML)
	if err := writeFile(flowsPath, flowsYAML); err != nil {
		return fmt.Errorf("write flows.yaml: %w", err)
	}
	hashKey, err = workspaceRelative(workspace, flowsPath)
	if err != nil {
		return err
	}
	result.Hashes[hashKey] = hashBytes(flowsYAML)
	result.FlowsYAMLRaw = flowsYAML

	return nil
}

func convertParametersToPlatform(params []SkillParameterPlan) []platform.SkillParameter {
	if len(params) == 0 {
		return nil
	}
	result := make([]platform.SkillParameter, 0, len(params))
	for _, p := range params {
		result = append(result, platform.SkillParameter{
			Name:         p.Name,
			DefaultValue: p.DefaultValue,
		})
	}
	return result
}

func convertParametersToState(params []SkillParameterPlan) []map[string]any {
	if len(params) == 0 {
		return nil
	}
	result := make([]map[string]any, 0, len(params))
	for _, p := range params {
		result = append(result, map[string]any{
			"name":          p.Name,
			"default_value": p.DefaultValue,
		})
	}
	return result
}

func convertParametersInfo(params []map[string]any) []platform.SkillParameter {
	if len(params) == 0 {
		return nil
	}
	result := make([]platform.SkillParameter, 0, len(params))
	for _, p := range params {
		name, _ := p["name"].(string)
		value := ""
		if raw, ok := p["default_value"]; ok && raw != nil {
			value = fmt.Sprint(raw)
		}
		result = append(result, platform.SkillParameter{
			Name:         name,
			DefaultValue: value,
		})
	}
	return result
}

func convertEventsToPlatform(events []state.FlowEventInfo) []platform.FlowEvent {
	if len(events) == 0 {
		return nil
	}
	result := make([]platform.FlowEvent, 0, len(events))
	for _, ev := range events {
		result = append(result, platform.FlowEvent{
			IDN:            ev.IDN,
			Description:    ev.Description,
			SkillSelector:  ev.SkillSelector,
			SkillIDN:       ev.SkillIDN,
			StateIDN:       ev.StateIDN,
			IntegrationIDN: ev.IntegrationIDN,
			ConnectorIDN:   ev.ConnectorIDN,
			InterruptMode:  ev.InterruptMode,
		})
	}
	return result
}

func convertStatesToPlatform(states []state.FlowStateInfo) []platform.FlowState {
	if len(states) == 0 {
		return nil
	}
	result := make([]platform.FlowState, 0, len(states))
	for _, st := range states {
		result = append(result, platform.FlowState{
			IDN:          st.IDN,
			Title:        st.Title,
			DefaultValue: st.DefaultValue,
			Scope:        st.Scope,
		})
	}
	return result
}

func findSkillPlan(plan ProjectPlan, agentIDN, flowIDN, skillIDN string) *SkillPlan {
	for idx := range plan.Agents {
		if plan.Agents[idx].IDN != agentIDN {
			continue
		}
		for fidx := range plan.Agents[idx].Flows {
			if plan.Agents[idx].Flows[fidx].IDN != flowIDN {
				continue
			}
			for sidx := range plan.Agents[idx].Flows[fidx].Skills {
				if plan.Agents[idx].Flows[fidx].Skills[sidx].IDN == skillIDN {
					return &plan.Agents[idx].Flows[fidx].Skills[sidx]
				}
			}
		}
	}
	return nil
}

func scriptExtension(relPath string) string {
	base := filepath.Base(relPath)
	if idx := strings.LastIndex(base, "."); idx != -1 && idx < len(base)-1 {
		return base[idx+1:]
	}
	return "txt"
}

func workspaceRelative(workspace, path string) (string, error) {
	rel, err := filepath.Rel(workspace, path)
	if err != nil {
		return "", fmt.Errorf("compute relative path for %s: %w", path, err)
	}
	return filepath.ToSlash(rel), nil
}

func writeFile(path string, data []byte) error {
	if err := fsutil.EnsureParentDir(path); err != nil {
		return err
	}
	return os.WriteFile(path, data, fsutil.FilePerm)
}

func hashBytes(data []byte) string {
	sum := sha256.Sum256(data)
	return fmt.Sprintf("%x", sum[:])
}

type noopReporter struct{}

func (noopReporter) Infof(string, ...any)    {}
func (noopReporter) Warnf(string, ...any)    {}
func (noopReporter) Successf(string, ...any) {}
