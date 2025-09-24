package mods

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/iw2rmb/ploy/internal/cli/common"
	"github.com/iw2rmb/ploy/internal/git/provider"
)

// test helpers for runner healing integration
type testJobHelper struct{}

func (testJobHelper) SubmitPlannerJob(context.Context, *ModConfig, string, string) (*PlanResult, error) {
	return &PlanResult{PlanID: "p1", Options: []map[string]any{{"id": "llm1", "type": "llm-exec"}}}, nil
}
func (testJobHelper) SubmitReducerJob(context.Context, string, []BranchResult, *BranchResult, string) (*NextAction, error) {
	return &NextAction{Action: "stop", Notes: "healed"}, nil
}

type okHCLSubmitter struct{}

func (okHCLSubmitter) Validate(string) error                                  { return nil }
func (okHCLSubmitter) Submit(string, time.Duration) error                     { return nil }
func (okHCLSubmitter) SubmitCtx(context.Context, string, time.Duration) error { return nil }

// failingHCLSubmitter validates OK but fails on submit to avoid external Nomad dependency.
type failingHCLSubmitter struct{}

func (failingHCLSubmitter) Validate(string) error              { return nil }
func (failingHCLSubmitter) Submit(string, time.Duration) error { return fmt.Errorf("submit failed") }
func (failingHCLSubmitter) SubmitCtx(context.Context, string, time.Duration) error {
	return fmt.Errorf("submit failed")
}

// okHealer returns a completed winner without real job submission
type okHealer struct{}

func (okHealer) RunFanout(ctx context.Context, runCtx interface{}, branches []BranchSpec, maxParallel int) (BranchResult, []BranchResult, error) {
	if len(branches) == 0 {
		return BranchResult{}, nil, fmt.Errorf("no branches")
	}
	res := make([]BranchResult, len(branches))
	for i, b := range branches {
		res[i] = BranchResult{ID: b.ID, Status: "completed", StartedAt: time.Now(), FinishedAt: time.Now()}
	}
	return res[0], res, nil
}

// seqBuildChecker returns success on the first call and error on subsequent calls.
// Used to let the orw-apply apply+build pass, then trigger healing on the later build gate.
type seqBuildChecker struct{ calls int }

func (s *seqBuildChecker) CheckBuild(ctx context.Context, cfg common.DeployConfig) (*common.DeployResult, error) {
	s.calls++
	if s.calls == 1 {
		return &common.DeployResult{Success: true, Version: "mock-v1"}, nil
	}
	return nil, fmt.Errorf("compilation failed: undefined symbol")
}

type capturingHealer struct {
	branches []BranchSpec
}

func (h *capturingHealer) RunFanout(ctx context.Context, runCtx interface{}, branches []BranchSpec, maxParallel int) (BranchResult, []BranchResult, error) {
	if len(branches) == 0 {
		return BranchResult{}, nil, fmt.Errorf("no branches")
	}
	h.branches = append([]BranchSpec(nil), branches...)
	results := make([]BranchResult, len(branches))
	for i, b := range branches {
		results[i] = BranchResult{ID: b.ID, Status: "completed"}
	}
	return results[0], results, nil
}

// Mock job submission interfaces for testing - move these to a shared file
// to be available to all tests in the package
type MockJobSubmitter struct {
	SubmitError   error
	WaitError     error
	CollectError  error
	SubmitCalled  bool
	WaitCalled    bool
	CollectCalled bool
	SubmittedJobs []JobSpec
	JobResults    map[string]JobResult
	ArtifactPaths map[string]string
	JobDelays     map[string]time.Duration
	mu            sync.Mutex
}

func (m *MockJobSubmitter) SubmitAndWaitTerminal(ctx context.Context, spec JobSpec) (JobResult, error) {
	m.mu.Lock()
	m.SubmitCalled = true
	m.SubmittedJobs = append(m.SubmittedJobs, spec)
	delay := time.Duration(0)
	if m.JobDelays != nil {
		delay = m.JobDelays[spec.Name]
	}
	m.mu.Unlock()

	if m.SubmitError != nil {
		return JobResult{}, m.SubmitError
	}

	if delay > 0 {
		select {
		case <-ctx.Done():
			return JobResult{JobID: fmt.Sprintf("job-%s-%d", spec.Name, time.Now().Unix()), Status: "cancelled", Duration: 0, Output: ctx.Err().Error()}, nil
		case <-time.After(delay):
		}
	} else if ctx.Err() != nil {
		return JobResult{JobID: fmt.Sprintf("job-%s-%d", spec.Name, time.Now().Unix()), Status: "cancelled", Duration: 0, Output: ctx.Err().Error()}, nil
	}

	// Return mock result
	result := JobResult{
		JobID:    fmt.Sprintf("job-%s-%d", spec.Name, time.Now().Unix()),
		Status:   "completed",
		Duration: 30 * time.Second,
	}

	m.mu.Lock()
	if mockResult, exists := m.JobResults[spec.Name]; exists {
		result = mockResult
	}
	m.mu.Unlock()

	if ctx.Err() != nil {
		result.Status = "cancelled"
		if result.Output == "" {
			result.Output = ctx.Err().Error()
		}
		return result, nil
	}

	return result, m.WaitError
}

func (m *MockJobSubmitter) CollectArtifacts(ctx context.Context, jobID string, outputDir string) (map[string]string, error) {
	m.mu.Lock()
	m.CollectCalled = true
	m.mu.Unlock()

	if m.CollectError != nil {
		return nil, m.CollectError
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	if artifacts, exists := m.ArtifactPaths[jobID]; exists {
		return map[string]string{"plan.json": artifacts}, nil
	}

	return map[string]string{}, nil
}

// MockProductionBranchRunner for testing production branch execution
type MockProductionBranchRunner struct {
	LLMExecAssetsPath   string
	LLMExecAssetsError  error
	ORWApplyAssetsPath  string
	ORWApplyAssetsError error
	GitProviderMock     provider.GitProvider
	BuildCheckerMock    BuildCheckerInterface
	WorkspaceDir        string
	TargetRepo          string
}

func (m *MockProductionBranchRunner) RenderLLMExecAssets(optionID string) (string, error) {
	if m.LLMExecAssetsError != nil {
		return "", m.LLMExecAssetsError
	}
	return m.LLMExecAssetsPath, nil
}

func (m *MockProductionBranchRunner) RenderORWApplyAssets(optionID string) (string, error) {
	if m.ORWApplyAssetsError != nil {
		return "", m.ORWApplyAssetsError
	}
	return m.ORWApplyAssetsPath, nil
}

func (m *MockProductionBranchRunner) GetGitProvider() provider.GitProvider {
	return m.GitProviderMock
}

func (m *MockProductionBranchRunner) GetBuildChecker() BuildCheckerInterface {
	return m.BuildCheckerMock
}

func (m *MockProductionBranchRunner) GetWorkspaceDir() string {
	if m.WorkspaceDir == "" {
		return "/tmp/test-workspace"
	}
	return m.WorkspaceDir
}

func (m *MockProductionBranchRunner) GetTargetRepo() string {
	if m.TargetRepo == "" {
		return "https://gitlab.com/test/repo.git"
	}
	return m.TargetRepo
}

func (m *MockProductionBranchRunner) GetEventReporter() EventReporter { return nil }

func (m *MockProductionBranchRunner) GetArtifactUploader() ArtifactUploader { return nil }
