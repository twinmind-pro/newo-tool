package sync

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"golang.org/x/sync/errgroup"

	"github.com/twinmind/newo-tool/internal/diff"
	"github.com/twinmind/newo-tool/internal/fsutil"
	"github.com/twinmind/newo-tool/internal/platform"
	"github.com/twinmind/newo-tool/internal/serialize"
	"github.com/twinmind/newo-tool/internal/state"
	"github.com/twinmind/newo-tool/internal/util"
	"gopkg.in/yaml.v3"
)

const (
	defaultContextLines   = 3
	defaultConcurrencyCap = 4
)

// SkillSyncClient captures the subset of platform client functionality required for synchronisation.
type SkillSyncClient interface {
	UpdateSkill(ctx context.Context, skillID string, payload platform.UpdateSkillRequest) error
	CreateSkill(ctx context.Context, flowID string, payload platform.CreateSkillRequest) (platform.CreateSkillResponse, error)
	DeleteSkill(ctx context.Context, skillID string) error
	GetSkill(ctx context.Context, skillID string) (platform.Skill, error)
	ListFlowSkills(ctx context.Context, flowID string) ([]platform.Skill, error)
	PublishFlow(ctx context.Context, flowID string, payload platform.PublishFlowRequest) error
}

// Reporter provides logging hooks for the service.
type Reporter interface {
	Infof(format string, args ...any)
	Warnf(format string, args ...any)
	Successf(format string, args ...any)
}

// DiffGenerator abstracts diff computation to simplify testing.
type DiffGenerator interface {
	Generate(local, remote []byte, context int) []diff.Line
}

// DiffFunc adapts a plain function to DiffGenerator.
type DiffFunc func(local, remote []byte, context int) []diff.Line

// Generate implements DiffGenerator.
func (f DiffFunc) Generate(local, remote []byte, context int) []diff.Line {
	return f(local, remote, context)
}

// ConfirmPushRequest describes the data shown to the user before pushing an update.
type ConfirmPushRequest struct {
	Path       string
	Diff       []diff.Line
	Remote     []byte
	Local      []byte
	SkillIDN   string
	FlowIDN    string
	ProjectIDN string
}

// Decision captures a yes/no choice with optional "apply to all".
type Decision struct {
	Apply    bool
	ApplyAll bool
}

// ConfirmPushFunc prompts before updating a remote skill.
type ConfirmPushFunc func(req ConfirmPushRequest) (Decision, error)

// ConfirmDeletionFunc prompts before deleting a remote-only skill.
type ConfirmDeletionFunc func(path, skillIDN string) (Decision, error)

// ProjectSlugger provides canonical slugs for project directories.
type ProjectSlugger func(projectIDN string, data state.ProjectData) string

// SaveProjectMapFunc persists the project map for a customer.
type SaveProjectMapFunc func(customerIDN string, pm state.ProjectMap) error

// SaveHashesFunc persists the hash snapshot for a customer.
type SaveHashesFunc func(customerIDN string, hashes state.HashStore) error

// RegenerateFlowsFunc regenerates flows.yaml for a project.
type RegenerateFlowsFunc func(customerType, customerIDN, projectIDN, projectSlug string, projectData state.ProjectData, hashes state.HashStore) error

// SkillSyncRequest aggregates inputs for a synchronisation run.
type SkillSyncRequest struct {
	SessionIDN    string
	CustomerType  string
	OutputRoot    string
	ProjectMap    *state.ProjectMap
	Hashes        state.HashStore
	ShouldPublish bool
	Verbose       bool
	Force         bool

	Reporter         Reporter
	ProjectSlugger   ProjectSlugger
	ConfirmPush      ConfirmPushFunc
	ConfirmDeletion  ConfirmDeletionFunc
	SaveProjectMap   SaveProjectMapFunc
	SaveHashes       SaveHashesFunc
	RegenerateFlows  RegenerateFlowsFunc
	DiffContextLines int
}

// SkillSyncWarning records non-fatal issues encountered during sync.
type SkillSyncWarning struct {
	Message string
}

// SkillSyncResult summarises the changes performed by the service.
type SkillSyncResult struct {
	Updated            int
	Removed            int
	Created            int
	Published          int
	Force              bool
	Hashes             state.HashStore
	Warnings           []SkillSyncWarning
	SkippedPublication bool
}

// SkillSyncService orchestrates skill synchronisation for push operations.
type SkillSyncService struct {
	client SkillSyncClient
	diff   DiffGenerator
}

// NewSkillSyncService constructs a SkillSyncService.
func NewSkillSyncService(client SkillSyncClient, diffGen DiffGenerator) *SkillSyncService {
	if diffGen == nil {
		diffGen = DiffFunc(diff.Generate)
	}
	return &SkillSyncService{
		client: client,
		diff:   diffGen,
	}
}

type publishTarget struct {
	projectIDN string
	agentIDN   string
	flowIDN    string
}

type skillSyncState struct {
	req                 SkillSyncRequest
	reporter            Reporter
	force               bool
	newHashes           state.HashStore
	flowsToPublish      map[string]publishTarget
	flowsToRegenerate   map[string]string
	updated             int
	removed             int
	created             int
	metadataChanged     bool
	warnings            []SkillSyncWarning
	diffContextLines    int
	flowSnapshotCache   map[string]*flowSnapshot
	flowSnapshotCacheMu sync.Mutex
}

// SyncCustomer performs the synchronisation and persists resulting state.
func (s *SkillSyncService) SyncCustomer(ctx context.Context, req SkillSyncRequest) (SkillSyncResult, error) {
	if req.ProjectMap == nil {
		return SkillSyncResult{}, fmt.Errorf("project map is required")
	}
	if req.Reporter == nil {
		req.Reporter = noopReporter{}
	}
	if req.ProjectSlugger == nil {
		req.ProjectSlugger = func(projectIDN string, data state.ProjectData) string {
			slug := strings.TrimSpace(data.Path)
			if slug != "" {
				return slug
			}
			base := strings.TrimSpace(projectIDN)
			if base == "" {
				base = "project"
			}
			return strings.ToLower(base)
		}
	}

	state := skillSyncState{
		req:               req,
		reporter:          req.Reporter,
		force:             req.Force,
		newHashes:         cloneHashes(req.Hashes),
		flowsToPublish:    map[string]publishTarget{},
		flowsToRegenerate: map[string]string{},
		diffContextLines:  effectiveContextLines(req.DiffContextLines),
	}

	if err := s.syncProjects(ctx, &state); err != nil {
		return SkillSyncResult{}, err
	}

	if state.updated == 0 && state.removed == 0 && state.created == 0 {
		return SkillSyncResult{
			Force:    state.force,
			Hashes:   state.newHashes,
			Warnings: state.warnings,
		}, nil
	}

	if err := s.persistState(&state); err != nil {
		return SkillSyncResult{}, err
	}

	published, err := s.publishFlows(ctx, &state)
	if err != nil {
		return SkillSyncResult{}, err
	}

	return SkillSyncResult{
		Updated:            state.updated,
		Removed:            state.removed,
		Created:            state.created,
		Published:          published,
		Force:              state.force,
		Hashes:             state.newHashes,
		Warnings:           state.warnings,
		SkippedPublication: !req.ShouldPublish,
	}, nil
}

func (s *SkillSyncService) syncProjects(ctx context.Context, st *skillSyncState) error {
	for projectIDN, projectData := range st.req.ProjectMap.Projects {
		projectSlug := st.req.ProjectSlugger(projectIDN, projectData)
		st.flowSnapshotCache = make(map[string]*flowSnapshot)
		for agentIDN, agentData := range projectData.Agents {
			for flowIDN, flowData := range agentData.Flows {
				if err := s.syncFlow(ctx, st, projectIDN, projectSlug, agentIDN, flowIDN, &flowData); err != nil {
					return err
				}
				agentData.Flows[flowIDN] = flowData
			}
			projectData.Agents[agentIDN] = agentData
		}
		st.req.ProjectMap.Projects[projectIDN] = projectData
	}
	return nil
}

func (s *SkillSyncService) syncFlow(
	ctx context.Context,
	st *skillSyncState,
	projectIDN, projectSlug, agentIDN, flowIDN string,
	flowData *state.FlowData,
) error {
	for skillIDN, skillInfo := range flowData.Skills {
		if err := s.syncExistingSkill(ctx, st, projectIDN, projectSlug, agentIDN, flowIDN, skillIDN, &skillInfo, flowData); err != nil {
			return err
		}
		if _, exists := flowData.Skills[skillIDN]; exists {
			flowData.Skills[skillIDN] = skillInfo
		}
	}

	created, err := s.createMissing(ctx, st, projectIDN, projectSlug, agentIDN, flowIDN, flowData)
	if err != nil {
		return err
	}
	if created > 0 {
		st.created += created
		st.metadataChanged = true
		st.flowsToRegenerate[projectIDN] = projectSlug
	}
	return nil
}

func (s *SkillSyncService) syncExistingSkill(
	ctx context.Context,
	st *skillSyncState,
	projectIDN, projectSlug, agentIDN, flowIDN, skillIDN string,
	meta *state.SkillMetadataInfo,
	flowData *state.FlowData,
) error {
	ext := platform.ScriptExtension(meta.RunnerType)
	fileName := fmt.Sprintf("%s.%s", skillIDN, ext)
	scriptPath := fsutil.ExportSkillScriptPath(st.req.OutputRoot, st.req.CustomerType, st.req.SessionIDN, projectSlug, agentIDN, flowIDN, fileName)
	normalized := filepath.ToSlash(scriptPath)

	oldHash, tracked := st.req.Hashes[normalized]
	content, readErr := os.ReadFile(scriptPath)
	if readErr != nil {
		if errors.Is(readErr, os.ErrNotExist) {
			return s.handleMissingFile(ctx, st, projectIDN, projectSlug, flowIDN, skillIDN, normalized, meta, flowData)
		}
		return fmt.Errorf("read %s: %w", normalized, readErr)
	}

	if strings.TrimSpace(meta.ID) == "" {
		st.reporter.Warnf("Skipping %s: missing remote skill identifier; run `newo pull`", normalized)
		st.warnings = append(st.warnings, SkillSyncWarning{Message: fmt.Sprintf("missing remote identifier for %s", normalized)})
		return nil
	}

	remoteSkill, found, err := s.remoteSkillSnapshot(ctx, st, flowData.ID, *meta)
	if err != nil {
		return fmt.Errorf("verify remote skill %s: %w", normalized, err)
	}
	if !found {
		st.reporter.Warnf("Skipping %s: remote skill not found; run `newo pull`", normalized)
		st.warnings = append(st.warnings, SkillSyncWarning{Message: fmt.Sprintf("remote skill missing for %s", normalized)})
		return nil
	}

	remoteScript := remoteSkill.PromptScript
	remoteHash := util.SHA256String(remoteScript)

	if tracked && oldHash != "" && remoteHash != oldHash {
		st.reporter.Warnf("Skipping %s: remote version changed since last pull; run `newo pull`", normalized)
		st.warnings = append(st.warnings, SkillSyncWarning{Message: fmt.Sprintf("remote changed for %s", normalized)})
		return nil
	}

	currentHash := util.SHA256Bytes(content)
	if tracked && currentHash == oldHash {
		return nil
	}

	if !tracked {
		st.reporter.Warnf("Skipping %s: not tracked in hashes; run `newo pull` to refresh mapping", normalized)
		st.warnings = append(st.warnings, SkillSyncWarning{Message: fmt.Sprintf("untracked file %s", normalized)})
		return nil
	}

	if !st.force {
		if st.req.ConfirmPush == nil {
			return nil
		}
		diffLines := s.diff.Generate([]byte(remoteScript), content, st.diffContextLines)
		decision, err := st.req.ConfirmPush(ConfirmPushRequest{
			Path:       normalized,
			Diff:       diffLines,
			Remote:     []byte(remoteScript),
			Local:      content,
			SkillIDN:   skillIDN,
			FlowIDN:    flowIDN,
			ProjectIDN: projectIDN,
		})
		if err != nil {
			return fmt.Errorf("confirm push %s: %w", normalized, err)
		}
		if !decision.Apply {
			st.reporter.Infof("Skipping %s.", normalized)
			return nil
		}
		if decision.ApplyAll {
			st.force = true
		}
	}

	if st.req.Verbose {
		st.reporter.Infof("Updating skill %s/%s/%s", projectIDN, flowIDN, skillIDN)
	}

	if err := s.pushSkill(ctx, remoteSkill, *meta, string(content)); err != nil {
		return fmt.Errorf("push skill %s: %w", normalized, err)
	}

	st.newHashes[normalized] = currentHash
	st.updated++
	s.invalidateFlowSnapshot(st, flowData.ID)

	if st.req.ShouldPublish && strings.TrimSpace(flowData.ID) != "" {
		st.flowsToPublish[flowData.ID] = publishTarget{projectIDN: projectIDN, agentIDN: agentIDN, flowIDN: flowIDN}
	}

	return nil
}

func (s *SkillSyncService) handleMissingFile(
	ctx context.Context,
	st *skillSyncState,
	projectIDN, projectSlug, flowIDN, skillIDN, normalized string,
	meta *state.SkillMetadataInfo,
	flowData *state.FlowData,
) error {
	if strings.TrimSpace(meta.ID) == "" {
		st.reporter.Warnf("Skipping %s: file missing and remote identifier unknown; run `newo pull`", normalized)
		st.warnings = append(st.warnings, SkillSyncWarning{Message: fmt.Sprintf("cannot delete unknown remote skill %s", normalized)})
		return nil
	}

	if !st.force {
		if st.req.ConfirmDeletion == nil {
			return nil
		}
		decision, err := st.req.ConfirmDeletion(normalized, skillIDN)
		if err != nil {
			return fmt.Errorf("confirm deletion %s: %w", normalized, err)
		}
		if !decision.Apply {
			return nil
		}
		if decision.ApplyAll {
			st.force = true
		}
	}

	if st.req.Verbose {
		st.reporter.Infof("Deleting missing skill %s/%s/%s", projectIDN, flowIDN, skillIDN)
	}
	if err := s.client.DeleteSkill(ctx, strings.TrimSpace(meta.ID)); err != nil {
		return fmt.Errorf("delete skill %s: %w", normalized, err)
	}

	delete(flowData.Skills, skillIDN)
	delete(st.newHashes, normalized)
	delete(st.req.Hashes, normalized)
	st.removed++
	st.metadataChanged = true
	st.flowsToRegenerate[projectIDN] = projectSlug
	st.reporter.Successf("Deleted remote skill %s/%s/%s", projectIDN, flowIDN, skillIDN)

	return nil
}

func (s *SkillSyncService) createMissing(
	ctx context.Context,
	st *skillSyncState,
	projectIDN, projectSlug, agentIDN, flowIDN string,
	flowData *state.FlowData,
) (int, error) {
	flowDir := fsutil.ExportFlowDir(st.req.OutputRoot, st.req.CustomerType, st.req.SessionIDN, projectSlug, agentIDN, flowIDN)
	entries, err := os.ReadDir(flowDir)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, fmt.Errorf("read flow directory: %w", err)
	}

	created := 0
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(name, fsutil.SkillMetaFileExt) || name == fsutil.MetadataYAML {
			continue
		}
		skillIDN := strings.TrimSuffix(name, fsutil.SkillMetaFileExt)
		if _, exists := flowData.Skills[skillIDN]; exists {
			continue
		}

		metadataPath := filepath.Join(flowDir, name)
		metaDoc, err := readSkillMetadata(metadataPath)
		if err != nil {
			return created, fmt.Errorf("decode metadata %s: %w", metadataPath, err)
		}

		if strings.TrimSpace(metaDoc.IDN) == "" {
			metaDoc.IDN = skillIDN
		}

		title := strings.TrimSpace(metaDoc.Title)
		if title == "" {
			title = metaDoc.IDN
		}

		ext := platform.ScriptExtension(metaDoc.RunnerType)
		scriptPath := filepath.Join(flowDir, fmt.Sprintf("%s.%s", skillIDN, ext))
		scriptBytes, err := os.ReadFile(scriptPath)
		if err != nil {
			if !errors.Is(err, os.ErrNotExist) {
				return created, fmt.Errorf("read script %s: %w", scriptPath, err)
			}
			scriptBytes = []byte{}
		}

		if st.req.Verbose {
			st.reporter.Infof("Creating new skill %s/%s/%s", projectIDN, flowIDN, skillIDN)
		}
		if strings.TrimSpace(flowData.ID) == "" {
			st.reporter.Warnf("Skipping %s/%s/%s: missing flow identifier", projectIDN, flowIDN, skillIDN)
			st.warnings = append(st.warnings, SkillSyncWarning{Message: fmt.Sprintf("missing flow identifier for %s/%s/%s", projectIDN, flowIDN, skillIDN)})
			continue
		}

		createReq := platform.CreateSkillRequest{
			IDN:          metaDoc.IDN,
			Title:        title,
			PromptScript: string(scriptBytes),
			RunnerType:   metaDoc.RunnerType,
			Model: platform.ModelConfig{
				ModelIDN:    metaDoc.Model.ModelIDN,
				ProviderIDN: metaDoc.Model.ProviderIDN,
			},
			Parameters: convertParametersForAPI(metaDoc.Parameters),
		}

		resp, err := s.client.CreateSkill(ctx, flowData.ID, createReq)
		if err != nil {
			return created, fmt.Errorf("create skill %s: %w", skillIDN, err)
		}
		created++

		if err := s.persistMetadata(flowDir, projectIDN, agentIDN, flowIDN, skillIDN, metaDoc, title, scriptBytes, resp.ID, flowData, st); err != nil {
			return created, err
		}
	}
	return created, nil
}

func (s *SkillSyncService) pushSkill(ctx context.Context, remote platform.Skill, meta state.SkillMetadataInfo, script string) error {
	request := platform.UpdateSkillRequest{
		ID:           remote.ID,
		IDN:          choose(meta.IDN, remote.IDN),
		Title:        choose(meta.Title, remote.Title),
		PromptScript: script,
		RunnerType:   choose(meta.RunnerType, remote.RunnerType),
		Model:        mergeModel(remote.Model, meta.Model),
		Parameters:   mergeParameters(remote.Parameters, meta.Parameters),
		Path:         remote.Path,
	}
	return s.client.UpdateSkill(ctx, remote.ID, request)
}

func (s *SkillSyncService) persistMetadata(
	flowDir, projectIDN, agentIDN, flowIDN, skillIDN string,
	metaDoc skillMetadataDocument,
	title string,
	scriptBytes []byte,
	remoteID string,
	flowData *state.FlowData,
	st *skillSyncState,
) error {
	metaBytes, err := serialize.SkillMetadata(platform.Skill{
		ID:           remoteID,
		IDN:          metaDoc.IDN,
		Title:        title,
		PromptScript: string(scriptBytes),
		RunnerType:   metaDoc.RunnerType,
		Model: platform.ModelConfig{
			ModelIDN:    metaDoc.Model.ModelIDN,
			ProviderIDN: metaDoc.Model.ProviderIDN,
		},
		Parameters: convertParametersForAPI(metaDoc.Parameters),
	})
	if err != nil {
		return fmt.Errorf("serialize metadata %s: %w", skillIDN, err)
	}

	metadataPath := filepath.Join(flowDir, fmt.Sprintf("%s%s", skillIDN, fsutil.SkillMetaFileExt))
	if err := fsutil.EnsureParentDir(metadataPath); err != nil {
		return err
	}
	if err := os.WriteFile(metadataPath, metaBytes, fsutil.FilePerm); err != nil {
		return fmt.Errorf("write metadata %s: %w", metadataPath, err)
	}

	ext := platform.ScriptExtension(metaDoc.RunnerType)
	scriptPath := filepath.Join(flowDir, fmt.Sprintf("%s.%s", skillIDN, ext))
	if err := fsutil.EnsureParentDir(scriptPath); err != nil {
		return err
	}
	if err := os.WriteFile(scriptPath, scriptBytes, fsutil.FilePerm); err != nil {
		return fmt.Errorf("write script %s: %w", scriptPath, err)
	}

	flowData.Skills[skillIDN] = state.SkillMetadataInfo{
		ID:         remoteID,
		IDN:        metaDoc.IDN,
		Title:      title,
		RunnerType: metaDoc.RunnerType,
		Model: map[string]string{
			"model_idn":    metaDoc.Model.ModelIDN,
			"provider_idn": metaDoc.Model.ProviderIDN,
		},
		Parameters: convertParametersForState(metaDoc.Parameters),
	}

	scriptHash := util.SHA256Bytes(scriptBytes)
	metadataHash := util.SHA256Bytes(metaBytes)
	st.newHashes[filepath.ToSlash(scriptPath)] = scriptHash
	st.newHashes[filepath.ToSlash(metadataPath)] = metadataHash

	if strings.TrimSpace(flowData.ID) != "" {
		st.flowsToPublish[flowData.ID] = publishTarget{
			projectIDN: projectIDN,
			agentIDN:   agentIDN,
			flowIDN:    flowIDN,
		}
	}
	return nil
}

func (s *SkillSyncService) persistState(st *skillSyncState) error {
	saveProjectMap := st.req.SaveProjectMap
	if saveProjectMap == nil {
		saveProjectMap = state.SaveProjectMap
	}
	saveHashes := st.req.SaveHashes
	if saveHashes == nil {
		saveHashes = state.SaveHashes
	}
	regenerateFlows := st.req.RegenerateFlows
	if regenerateFlows == nil {
		regenerateFlows = func(customerType, customerIDN, projectIDN, projectSlug string, projectData state.ProjectData, hashes state.HashStore) error {
			return regenerateFlowsYAML(st.req.OutputRoot, customerType, customerIDN, projectIDN, projectSlug, projectData, hashes)
		}
	}

	var errs []error
	if err := saveProjectMap(st.req.SessionIDN, *st.req.ProjectMap); err != nil {
		errs = append(errs, fmt.Errorf("save project map: %w", err))
	}
	if err := saveHashes(st.req.SessionIDN, st.newHashes); err != nil {
		errs = append(errs, fmt.Errorf("save hashes: %w", err))
	}
	if st.metadataChanged {
		if err := s.regenerateFlows(regenerateFlows, st); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func (s *SkillSyncService) regenerateFlows(regen RegenerateFlowsFunc, st *skillSyncState) error {
	if len(st.flowsToRegenerate) == 0 {
		return nil
	}

	maxConcurrency := min(len(st.flowsToRegenerate), concurrencyCap())
	var g errgroup.Group
	sem := make(chan struct{}, maxConcurrency)

	for projectIDN, slug := range st.flowsToRegenerate {
		pid := projectIDN
		projectSlug := slug
		sem <- struct{}{}
		g.Go(func() error {
			defer func() { <-sem }()
			data := st.req.ProjectMap.Projects[pid]
			return regen(st.req.CustomerType, st.req.SessionIDN, pid, projectSlug, data, st.newHashes)
		})
	}

	return g.Wait()
}

func (s *SkillSyncService) publishFlows(ctx context.Context, st *skillSyncState) (int, error) {
	if !st.req.ShouldPublish || len(st.flowsToPublish) == 0 {
		return 0, nil
	}

	maxConcurrency := min(len(st.flowsToPublish), concurrencyCap())
	g, gctx := errgroup.WithContext(ctx)
	sem := make(chan struct{}, maxConcurrency)

	publishedMu := sync.Mutex{}
	published := 0
	var errs []error
	var errsMu sync.Mutex

	for flowID, meta := range st.flowsToPublish {
		flowID := flowID
		meta := meta
		sem <- struct{}{}
		g.Go(func() error {
			defer func() { <-sem }()
			if err := s.client.PublishFlow(gctx, flowID, defaultPublishRequest()); err != nil {
				errsMu.Lock()
				errs = append(errs, fmt.Errorf("publish flow %s/%s/%s: %w", meta.projectIDN, meta.agentIDN, meta.flowIDN, err))
				errsMu.Unlock()
				return nil
			}
			if st.req.Verbose {
				st.reporter.Infof("Published %s/%s/%s", meta.projectIDN, meta.agentIDN, meta.flowIDN)
			}
			publishedMu.Lock()
			published++
			publishedMu.Unlock()
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		return published, err
	}
	if len(errs) > 0 {
		return published, errors.Join(errs...)
	}
	return published, nil
}

func (s *SkillSyncService) remoteSkillSnapshot(ctx context.Context, st *skillSyncState, flowID string, info state.SkillMetadataInfo) (platform.Skill, bool, error) {
	flowID = strings.TrimSpace(flowID)
	id := strings.TrimSpace(info.ID)

	if flowID != "" {
		snap, err := s.loadFlowSnapshot(ctx, st, flowID)
		if err != nil {
			return platform.Skill{}, false, err
		}
		if skill, found := snap.lookup(info); found {
			return skill, true, nil
		}
	}

	if id == "" {
		return platform.Skill{}, false, nil
	}

	skill, err := s.client.GetSkill(ctx, id)
	if err != nil {
		var apiErr *platform.APIError
		if errors.As(err, &apiErr) && apiErr.Status == 404 {
			return platform.Skill{}, false, nil
		}
		return platform.Skill{}, false, err
	}

	if flowID != "" {
		s.storeSkillSnapshot(st, flowID, skill)
	}
	return skill, true, nil
}

func (s *SkillSyncService) loadFlowSnapshot(ctx context.Context, st *skillSyncState, flowID string) (*flowSnapshot, error) {
	st.flowSnapshotCacheMu.Lock()
	if st.flowSnapshotCache == nil {
		st.flowSnapshotCache = make(map[string]*flowSnapshot)
	}
	if snap, ok := st.flowSnapshotCache[flowID]; ok {
		st.flowSnapshotCacheMu.Unlock()
		return snap, nil
	}
	st.flowSnapshotCacheMu.Unlock()

	skills, err := s.client.ListFlowSkills(ctx, flowID)
	if err != nil {
		return nil, err
	}
	snap := newFlowSnapshot(skills)

	st.flowSnapshotCacheMu.Lock()
	st.flowSnapshotCache[flowID] = snap
	st.flowSnapshotCacheMu.Unlock()
	return snap, nil
}

func (s *SkillSyncService) storeSkillSnapshot(st *skillSyncState, flowID string, skill platform.Skill) {
	if strings.TrimSpace(flowID) == "" {
		return
	}
	st.flowSnapshotCacheMu.Lock()
	if st.flowSnapshotCache == nil {
		st.flowSnapshotCache = make(map[string]*flowSnapshot)
	}
	snap, ok := st.flowSnapshotCache[flowID]
	if !ok {
		snap = newFlowSnapshot(nil)
		st.flowSnapshotCache[flowID] = snap
	}
	st.flowSnapshotCacheMu.Unlock()
	snap.store(skill)
}

func (s *SkillSyncService) invalidateFlowSnapshot(st *skillSyncState, flowID string) {
	if strings.TrimSpace(flowID) == "" {
		return
	}
	st.flowSnapshotCacheMu.Lock()
	delete(st.flowSnapshotCache, flowID)
	st.flowSnapshotCacheMu.Unlock()
}

type flowSnapshot struct {
	byID  map[string]platform.Skill
	byIDN map[string]platform.Skill
}

func newFlowSnapshot(skills []platform.Skill) *flowSnapshot {
	snap := &flowSnapshot{
		byID:  make(map[string]platform.Skill),
		byIDN: make(map[string]platform.Skill),
	}
	for _, s := range skills {
		if s.ID != "" {
			snap.byID[s.ID] = s
		}
		if s.IDN != "" {
			snap.byIDN[strings.ToLower(s.IDN)] = s
		}
	}
	return snap
}

func (s *flowSnapshot) lookup(info state.SkillMetadataInfo) (platform.Skill, bool) {
	if s == nil {
		return platform.Skill{}, false
	}
	if info.ID != "" {
		if skill, ok := s.byID[info.ID]; ok {
			return skill, true
		}
	}
	if info.IDN != "" {
		if skill, ok := s.byIDN[strings.ToLower(info.IDN)]; ok {
			return skill, true
		}
	}
	return platform.Skill{}, false
}

func (s *flowSnapshot) store(skill platform.Skill) {
	if s == nil {
		return
	}
	if skill.ID != "" {
		s.byID[skill.ID] = skill
	}
	if skill.IDN != "" {
		s.byIDN[strings.ToLower(skill.IDN)] = skill
	}
}

func readSkillMetadata(path string) (skillMetadataDocument, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return skillMetadataDocument{}, err
	}
	var doc skillMetadataDocument
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return skillMetadataDocument{}, err
	}
	return doc, nil
}

type skillMetadataDocument struct {
	ID         string                   `yaml:"id"`
	IDN        string                   `yaml:"idn"`
	Title      string                   `yaml:"title"`
	RunnerType string                   `yaml:"runner_type"`
	Model      skillMetadataModel       `yaml:"model"`
	Parameters []skillParameterMetadata `yaml:"parameters"`
}

type skillMetadataModel struct {
	ModelIDN    string `yaml:"modelidn"`
	ProviderIDN string `yaml:"provideridn"`
}

type skillParameterMetadata struct {
	Name         string      `yaml:"name"`
	DefaultValue interface{} `yaml:"default_value"`
}

func convertParametersForAPI(params []skillParameterMetadata) []platform.SkillParameter {
	if len(params) == 0 {
		return nil
	}
	result := make([]platform.SkillParameter, 0, len(params))
	for _, p := range params {
		result = append(result, platform.SkillParameter{
			Name:         p.Name,
			DefaultValue: fmt.Sprint(p.DefaultValue),
		})
	}
	return result
}

func convertParametersForState(params []skillParameterMetadata) []map[string]any {
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

func convertParameters(params []map[string]any) []platform.SkillParameter {
	if len(params) == 0 {
		return nil
	}
	result := make([]platform.SkillParameter, 0, len(params))
	for _, raw := range params {
		name, _ := raw["name"].(string)
		if strings.TrimSpace(name) == "" {
			continue
		}
		value := ""
		if v, ok := raw["default_value"]; ok && v != nil {
			value = fmt.Sprint(v)
		}
		result = append(result, platform.SkillParameter{
			Name:         name,
			DefaultValue: value,
		})
	}
	return result
}

func defaultPublishRequest() platform.PublishFlowRequest {
	return platform.PublishFlowRequest{
		Version:     "1.0",
		Description: "Published via newo-go CLI",
		Type:        "public",
	}
}

func choose(primary, fallback string) string {
	if v := strings.TrimSpace(primary); v != "" {
		return v
	}
	return fallback
}

func mergeModel(remote platform.ModelConfig, override map[string]string) platform.ModelConfig {
	result := remote
	if override == nil {
		return result
	}
	if v := strings.TrimSpace(override["model_idn"]); v != "" {
		result.ModelIDN = v
	}
	if v := strings.TrimSpace(override["provider_idn"]); v != "" {
		result.ProviderIDN = v
	}
	return result
}

func mergeParameters(remote []platform.SkillParameter, override []map[string]any) []platform.SkillParameter {
	if len(override) > 0 {
		return convertParameters(override)
	}
	return remote
}

func regenerateFlowsYAML(outputRoot, customerType, customerIDN, projectIDN, projectSlug string, projectData state.ProjectData, hashes state.HashStore) error {
	project := platform.Project{ID: projectData.ProjectID, IDN: projectIDN, Title: projectIDN}
	content, err := serialize.GenerateFlowsYAML(project, projectData)
	if err != nil {
		return fmt.Errorf("generate flows.yaml: %w", err)
	}
	path := fsutil.ExportFlowsYAMLPath(outputRoot, customerType, customerIDN, projectSlug)
	if err := fsutil.EnsureParentDir(path); err != nil {
		return err
	}
	if err := os.WriteFile(path, content, fsutil.FilePerm); err != nil {
		return fmt.Errorf("write flows.yaml: %w", err)
	}
	hashes[filepath.ToSlash(path)] = util.SHA256Bytes(content)
	return nil
}

func cloneHashes(hashes state.HashStore) state.HashStore {
	if hashes == nil {
		return state.HashStore{}
	}
	copied := make(state.HashStore, len(hashes))
	for k, v := range hashes {
		copied[k] = v
	}
	return copied
}

func effectiveContextLines(requested int) int {
	if requested > 0 {
		return requested
	}
	return defaultContextLines
}

type noopReporter struct{}

func (noopReporter) Infof(string, ...any)    {}
func (noopReporter) Warnf(string, ...any)    {}
func (noopReporter) Successf(string, ...any) {}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func concurrencyCap() int {
	capacity := defaultConcurrencyCap
	if cpu := runtime.NumCPU(); cpu > 0 && cpu < capacity {
		capacity = cpu
	}
	if capacity < 1 {
		return 1
	}
	return capacity
}
