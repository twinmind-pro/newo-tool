package sync

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/twinmind/newo-tool/internal/fsutil"
	"github.com/twinmind/newo-tool/internal/platform"
	"github.com/twinmind/newo-tool/internal/state"
	"github.com/twinmind/newo-tool/internal/util"
)

func TestSkillSyncService_UpdateExistingSkill(t *testing.T) {
	t.Parallel()

	outputRoot := t.TempDir()
	client := newFakeSkillClient()

	projectIDN := "project"
	projectSlug := "project"
	agentIDN := "agent"
	flowIDN := "flow"
	skillIDN := "skill"

	remoteSkill := platform.Skill{
		ID:           "skill-id",
		IDN:          skillIDN,
		Title:        "Skill",
		PromptScript: "old script",
		RunnerType:   "nsl",
		Model: platform.ModelConfig{
			ModelIDN:    "m",
			ProviderIDN: "p",
		},
	}

	client.addFlowSkill("flow-id", remoteSkill)

	projectMap := state.ProjectMap{
		Projects: map[string]state.ProjectData{
			projectIDN: {
				ProjectID:  "proj-uuid",
				ProjectIDN: projectIDN,
				Path:       projectSlug,
				Agents: map[string]state.AgentData{
					agentIDN: {
						ID: "agent-id",
						Flows: map[string]state.FlowData{
							flowIDN: {
								ID: "flow-id",
								Skills: map[string]state.SkillMetadataInfo{
									skillIDN: {
										ID:         remoteSkill.ID,
										IDN:        remoteSkill.IDN,
										Title:      remoteSkill.Title,
										RunnerType: remoteSkill.RunnerType,
										Model: map[string]string{
											"model_idn":    remoteSkill.Model.ModelIDN,
											"provider_idn": remoteSkill.Model.ProviderIDN,
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	scriptPath := fsutil.ExportSkillScriptPath(
		outputRoot, "integration", "customer", projectSlug, agentIDN, flowIDN, skillIDN+"."+platform.ScriptExtension(remoteSkill.RunnerType),
	)
	if err := fsutil.EnsureParentDir(scriptPath); err != nil {
		t.Fatalf("ensure dir: %v", err)
	}
	const localScript = "new script"
	if err := os.WriteFile(scriptPath, []byte(localScript), fsutil.FilePerm); err != nil {
		t.Fatalf("write script: %v", err)
	}

	hashes := state.HashStore{
		filepath.ToSlash(scriptPath): util.SHA256String(remoteSkill.PromptScript),
	}

	var (
		savedHashes state.HashStore
		saveMu      sync.Mutex
	)

	req := SkillSyncRequest{
		SessionIDN:    "customer",
		CustomerType:  "integration",
		OutputRoot:    outputRoot,
		ProjectMap:    &projectMap,
		Hashes:        hashes,
		ShouldPublish: false,
		Verbose:       false,
		Force:         false,
		Reporter:      noopReporter{},
		ProjectSlugger: func(projectIDN string, data state.ProjectData) string {
			return data.Path
		},
		ConfirmPush: func(info ConfirmPushRequest) (Decision, error) {
			return Decision{Apply: true}, nil
		},
		ConfirmDeletion: func(string, string) (Decision, error) {
			return Decision{}, nil
		},
		SaveProjectMap: func(string, state.ProjectMap) error {
			return nil
		},
		SaveHashes: func(_ string, h state.HashStore) error {
			saveMu.Lock()
			defer saveMu.Unlock()
			savedHashes = cloneHashes(h)
			return nil
		},
	}

	service := NewSkillSyncService(client, nil)
	result, err := service.SyncCustomer(context.Background(), req)
	if err != nil {
		t.Fatalf("SyncCustomer: %v", err)
	}

	if result.Updated != 1 {
		t.Fatalf("expected 1 skill updated, got %d", result.Updated)
	}
	if len(client.updateCalls) != 1 {
		t.Fatalf("expected 1 UpdateSkill call, got %d", len(client.updateCalls))
	}
	if client.updateCalls[0].PromptScript != localScript {
		t.Fatalf("unexpected script: %q", client.updateCalls[0].PromptScript)
	}
	saveMu.Lock()
	defer saveMu.Unlock()
	if savedHashes == nil {
		t.Fatalf("hashes not persisted")
	}
	if savedHashes[filepath.ToSlash(scriptPath)] != util.SHA256String(localScript) {
		t.Fatalf("hash not updated")
	}
}

func TestSkillSyncService_DeleteMissingSkill(t *testing.T) {
	t.Parallel()

	outputRoot := t.TempDir()
	client := newFakeSkillClient()

	projectMap := state.ProjectMap{
		Projects: map[string]state.ProjectData{
			"project": {
				ProjectID: "proj-uuid",
				Path:      "project",
				Agents: map[string]state.AgentData{
					"agent": {
						ID: "agent-id",
						Flows: map[string]state.FlowData{
							"flow": {
								ID: "flow-id",
								Skills: map[string]state.SkillMetadataInfo{
									"skill": {
										ID:         "skill-id",
										IDN:        "skill",
										RunnerType: "nsl",
										Model: map[string]string{
											"model_idn":    "m",
											"provider_idn": "p",
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	scriptPath := fsutil.ExportSkillScriptPath(outputRoot, "integration", "customer", "project", "agent", "flow", "skill.nsl")
	hashes := state.HashStore{
		filepath.ToSlash(scriptPath): util.SHA256String("old"),
	}

	var (
		deleted []string
		mu      sync.Mutex
	)

	req := SkillSyncRequest{
		SessionIDN:    "customer",
		CustomerType:  "integration",
		OutputRoot:    outputRoot,
		ProjectMap:    &projectMap,
		Hashes:        hashes,
		ShouldPublish: false,
		Reporter:      noopReporter{},
		ProjectSlugger: func(projectIDN string, data state.ProjectData) string {
			return data.Path
		},
		ConfirmPush: func(info ConfirmPushRequest) (Decision, error) {
			return Decision{}, nil
		},
		ConfirmDeletion: func(string, string) (Decision, error) {
			return Decision{Apply: true}, nil
		},
		SaveProjectMap: func(string, state.ProjectMap) error {
			return nil
		},
		SaveHashes: func(string, state.HashStore) error {
			return nil
		},
		RegenerateFlows: func(string, string, string, string, state.ProjectData, state.HashStore) error {
			return nil
		},
	}

	client.deleteHook = func(id string) {
		mu.Lock()
		defer mu.Unlock()
		deleted = append(deleted, id)
	}

	service := NewSkillSyncService(client, nil)
	result, err := service.SyncCustomer(context.Background(), req)
	if err != nil {
		t.Fatalf("SyncCustomer: %v", err)
	}

	if result.Removed != 1 {
		t.Fatalf("expected 1 skill removed, got %d", result.Removed)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(deleted) != 1 || deleted[0] != "skill-id" {
		t.Fatalf("unexpected delete calls: %+v", deleted)
	}
	if _, ok := projectMap.Projects["project"].Agents["agent"].Flows["flow"].Skills["skill"]; ok {
		t.Fatalf("skill not removed from project map")
	}
	if _, ok := hashes[filepath.ToSlash(scriptPath)]; ok {
		t.Fatalf("hash entry not removed")
	}
}

func TestSkillSyncService_CreateMissingSkill(t *testing.T) {
	t.Parallel()

	outputRoot := t.TempDir()
	client := newFakeSkillClient()

	projectMap := state.ProjectMap{
		Projects: map[string]state.ProjectData{
			"project": {
				ProjectID: "proj",
				Path:      "project",
				Agents: map[string]state.AgentData{
					"agent": {
						ID: "agent-id",
						Flows: map[string]state.FlowData{
							"flow": {
								ID:     "flow-id",
								Skills: map[string]state.SkillMetadataInfo{},
							},
						},
					},
				},
			},
		},
	}

	flowDir := fsutil.ExportFlowDir(outputRoot, "integration", "customer", "project", "agent", "flow")
	if err := os.MkdirAll(flowDir, fsutil.DirPerm); err != nil {
		t.Fatalf("mkdir flow dir: %v", err)
	}

	meta := map[string]any{
		"idn":         "new_skill",
		"title":       "New Skill",
		"runner_type": "nsl",
		"model": map[string]string{
			"modelidn":    "m",
			"provideridn": "p",
		},
		"parameters": []map[string]any{
			{"name": "foo", "default_value": "bar"},
		},
	}
	metaBytes, err := yaml.Marshal(meta)
	if err != nil {
		t.Fatalf("marshal metadata: %v", err)
	}
	if err := os.WriteFile(filepath.Join(flowDir, "new_skill.meta.yaml"), metaBytes, fsutil.FilePerm); err != nil {
		t.Fatalf("write meta: %v", err)
	}
	if err := os.WriteFile(filepath.Join(flowDir, "new_skill.nsl"), []byte("content"), fsutil.FilePerm); err != nil {
		t.Fatalf("write script: %v", err)
	}

	var (
		createdSkills []platform.CreateSkillRequest
		createMu      sync.Mutex
	)
	client.createHook = func(req platform.CreateSkillRequest) string {
		createMu.Lock()
		defer createMu.Unlock()
		createdSkills = append(createdSkills, req)
		return "new-skill-id"
	}

	req := SkillSyncRequest{
		SessionIDN:    "customer",
		CustomerType:  "integration",
		OutputRoot:    outputRoot,
		ProjectMap:    &projectMap,
		Hashes:        state.HashStore{},
		ShouldPublish: false,
		Reporter:      noopReporter{},
		ProjectSlugger: func(projectIDN string, data state.ProjectData) string {
			return data.Path
		},
		ConfirmPush: func(info ConfirmPushRequest) (Decision, error) {
			return Decision{}, nil
		},
		ConfirmDeletion: func(string, string) (Decision, error) {
			return Decision{}, nil
		},
		SaveProjectMap: func(string, state.ProjectMap) error {
			return nil
		},
		SaveHashes: func(string, state.HashStore) error {
			return nil
		},
		RegenerateFlows: func(string, string, string, string, state.ProjectData, state.HashStore) error {
			return nil
		},
	}

	service := NewSkillSyncService(client, nil)
	result, err := service.SyncCustomer(context.Background(), req)
	if err != nil {
		t.Fatalf("SyncCustomer: %v", err)
	}

	if result.Created != 1 {
		t.Fatalf("expected 1 skill created, got %d", result.Created)
	}
	createMu.Lock()
	defer createMu.Unlock()
	if len(createdSkills) != 1 {
		t.Fatalf("expected 1 create request, got %d", len(createdSkills))
	}
	reqPayload := createdSkills[0]
	if reqPayload.IDN != "new_skill" || reqPayload.RunnerType != "nsl" || reqPayload.PromptScript != "content" {
		t.Fatalf("unexpected create payload: %+v", reqPayload)
	}
	skillMeta, ok := projectMap.Projects["project"].Agents["agent"].Flows["flow"].Skills["new_skill"]
	if !ok {
		t.Fatalf("skill metadata not recorded in project map")
	}
	if skillMeta.ID == "" {
		t.Fatalf("skill identifier not recorded")
	}
	slashScript := filepath.ToSlash(filepath.Join(flowDir, "new_skill.nsl"))
	if result.Hashes[slashScript] == "" {
		t.Fatalf("script hash missing from result")
	}
	metaPath := filepath.ToSlash(filepath.Join(flowDir, "new_skill.meta.yaml"))
	if result.Hashes[metaPath] == "" {
		t.Fatalf("metadata hash missing from result")
	}
}

// fakeSkillClient provides a thread-safe test double for SkillSyncClient.
type fakeSkillClient struct {
	mu           sync.Mutex
	flowSkills   map[string][]platform.Skill
	skillsByID   map[string]platform.Skill
	updateCalls  []platform.UpdateSkillRequest
	deleteCalls  []string
	publishCalls []string

	deleteHook func(skillID string)
	createHook func(req platform.CreateSkillRequest) string
}

func newFakeSkillClient() *fakeSkillClient {
	return &fakeSkillClient{
		flowSkills: make(map[string][]platform.Skill),
		skillsByID: make(map[string]platform.Skill),
	}
}

func (f *fakeSkillClient) addFlowSkill(flowID string, skill platform.Skill) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.flowSkills[flowID] = append(f.flowSkills[flowID], skill)
	if skill.ID != "" {
		f.skillsByID[skill.ID] = skill
	}
}

func (f *fakeSkillClient) UpdateSkill(_ context.Context, skillID string, payload platform.UpdateSkillRequest) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.updateCalls = append(f.updateCalls, payload)
	return nil
}

func (f *fakeSkillClient) CreateSkill(_ context.Context, flowID string, payload platform.CreateSkillRequest) (platform.CreateSkillResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	id := "generated-id"
	if f.createHook != nil {
		id = f.createHook(payload)
	}
	skill := platform.Skill{
		ID:           id,
		IDN:          payload.IDN,
		Title:        payload.Title,
		PromptScript: payload.PromptScript,
		RunnerType:   payload.RunnerType,
		Model:        payload.Model,
		Parameters:   payload.Parameters,
	}
	f.flowSkills[flowID] = append(f.flowSkills[flowID], skill)
	f.skillsByID[id] = skill
	return platform.CreateSkillResponse{ID: id}, nil
}

func (f *fakeSkillClient) DeleteSkill(_ context.Context, skillID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.deleteCalls = append(f.deleteCalls, skillID)
	if f.deleteHook != nil {
		f.deleteHook(skillID)
	}
	return nil
}

func (f *fakeSkillClient) GetSkill(_ context.Context, skillID string) (platform.Skill, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if skill, ok := f.skillsByID[skillID]; ok {
		return skill, nil
	}
	return platform.Skill{}, errors.New("not found")
}

func (f *fakeSkillClient) ListFlowSkills(_ context.Context, flowID string) ([]platform.Skill, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	skills := f.flowSkills[flowID]
	copied := make([]platform.Skill, len(skills))
	copy(copied, skills)
	return copied, nil
}

func (f *fakeSkillClient) PublishFlow(_ context.Context, flowID string, _ platform.PublishFlowRequest) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.publishCalls = append(f.publishCalls, flowID)
	return nil
}
