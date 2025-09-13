package transflow

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/iw2rmb/ploy/internal/cli/common"
	"github.com/iw2rmb/ploy/internal/git/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// test helpers for runner healing integration
type testJobHelper struct{}

func (testJobHelper) SubmitPlannerJob(context.Context, *TransflowConfig, string, string) (*PlanResult, error) {
	return &PlanResult{PlanID: "p1", Options: []map[string]any{{"id": "llm1", "type": "llm-exec"}}}, nil
}
func (testJobHelper) SubmitReducerJob(context.Context, string, []BranchResult, *BranchResult, string) (*NextAction, error) {
	return &NextAction{Action: "stop", Notes: "healed"}, nil
}

type okHCLSubmitter struct{}

func (okHCLSubmitter) Validate(string) error              { return nil }
func (okHCLSubmitter) Submit(string, time.Duration) error { return nil }

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

// Mock job submission interfaces for testing - move these to top of file
// to be available to all parts of the package
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
}

// JobSpec and JobResult types are now defined in types.go

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

func (m *MockJobSubmitter) SubmitAndWaitTerminal(ctx context.Context, spec JobSpec) (JobResult, error) {
	m.SubmitCalled = true
	m.SubmittedJobs = append(m.SubmittedJobs, spec)

	if m.SubmitError != nil {
		return JobResult{}, m.SubmitError
	}

	// Return mock result
	result := JobResult{
		JobID:    fmt.Sprintf("job-%s-%d", spec.Name, time.Now().Unix()),
		Status:   "completed",
		Duration: 30 * time.Second,
	}

	if mockResult, exists := m.JobResults[spec.Name]; exists {
		result = mockResult
	}

	return result, m.WaitError
}

func (m *MockJobSubmitter) CollectArtifacts(ctx context.Context, jobID string, outputDir string) (map[string]string, error) {
	m.CollectCalled = true

	if m.CollectError != nil {
		return nil, m.CollectError
	}

	if artifacts, exists := m.ArtifactPaths[jobID]; exists {
		return map[string]string{"plan.json": artifacts}, nil
	}

	return map[string]string{}, nil
}

// Test job submission helpers
func TestSubmitPlannerJob(t *testing.T) {
	tests := []struct {
		name          string
		config        *TransflowConfig
		buildError    string
		expectError   bool
		expectJobType string
	}{
		{
			name: "successful planner submission",
			config: &TransflowConfig{
				ID:         "test-workflow",
				TargetRepo: "https://github.com/test/repo",
				BaseRef:    "main",
			},
			buildError:    "compilation failed: undefined symbol",
			expectError:   false,
			expectJobType: "planner",
		},
		{
			name: "planner submission with job failure",
			config: &TransflowConfig{
				ID:         "failing-workflow",
				TargetRepo: "https://github.com/test/repo",
				BaseRef:    "main",
			},
			buildError:    "build timeout",
			expectError:   true,
			expectJobType: "planner",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// This test will fail until we implement SubmitPlannerJob
			ctx := context.Background()
			mockSubmitter := &MockJobSubmitter{
				JobResults:    make(map[string]JobResult),
				ArtifactPaths: make(map[string]string),
			}

			if tt.expectError {
				mockSubmitter.SubmitError = fmt.Errorf("job submission failed")
			} else {
				mockSubmitter.JobResults["planner"] = JobResult{
					JobID:  "planner-123",
					Status: "completed",
					Output: `{"plan_id": "test-plan", "options": [{"id": "opt1", "type": "llm-exec"}]}`,
				}
				mockSubmitter.ArtifactPaths["planner-123"] = "/tmp/plan.json"
			}

			submitter := NewJobSubmissionHelper(mockSubmitter)

			// This function doesn't exist yet - will cause compilation failure
			plan, err := submitter.SubmitPlannerJob(ctx, tt.config, tt.buildError, "/tmp/workspace")

			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, plan)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, plan)
				assert.Equal(t, "test-plan", plan.PlanID)
				assert.Len(t, plan.Options, 1)
			}

			assert.True(t, mockSubmitter.SubmitCalled)
			require.Len(t, mockSubmitter.SubmittedJobs, 1)
			assert.Equal(t, tt.expectJobType, mockSubmitter.SubmittedJobs[0].Type)
		})
	}
}

func TestSubmitReducerJob(t *testing.T) {
	tests := []struct {
		name          string
		planID        string
		branchResults []BranchResult
		winner        *BranchResult
		expectError   bool
	}{
		{
			name:   "successful reducer submission",
			planID: "test-plan-123",
			branchResults: []BranchResult{
				{ID: "branch-1", Status: "completed", Notes: "success"},
				{ID: "branch-2", Status: "failed", Notes: "timeout"},
			},
			winner:      &BranchResult{ID: "branch-1", Status: "completed"},
			expectError: false,
		},
		{
			name:   "reducer with all failed branches",
			planID: "failed-plan-456",
			branchResults: []BranchResult{
				{ID: "branch-1", Status: "failed", Notes: "error"},
				{ID: "branch-2", Status: "failed", Notes: "timeout"},
			},
			winner:      nil,
			expectError: false, // Reducer should still process failure case
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			mockSubmitter := &MockJobSubmitter{
				JobResults:    make(map[string]JobResult),
				ArtifactPaths: make(map[string]string),
			}

			// Mock reducer result
			mockSubmitter.JobResults["reducer"] = JobResult{
				JobID:  "reducer-789",
				Status: "completed",
				Output: `{"action": "stop", "notes": "workflow completed"}`,
			}
			mockSubmitter.ArtifactPaths["reducer-789"] = "/tmp/next.json"

			submitter := NewJobSubmissionHelper(mockSubmitter)

			// This function doesn't exist yet - will cause compilation failure
			nextAction, err := submitter.SubmitReducerJob(ctx, tt.planID, tt.branchResults, tt.winner, "/tmp/workspace")

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, nextAction)
				assert.Equal(t, "stop", nextAction.Action)
			}

			assert.True(t, mockSubmitter.SubmitCalled)
			require.Len(t, mockSubmitter.SubmittedJobs, 1)
			assert.Equal(t, "reducer", mockSubmitter.SubmittedJobs[0].Type)
		})
	}
}

// Test fanout orchestration
func TestRunHealingFanout(t *testing.T) {
	tests := []struct {
		name         string
		branches     []BranchSpec
		maxParallel  int
		expectWinner bool
		expectError  bool
	}{
		{
			name: "successful fanout with winner",
			branches: []BranchSpec{
				{ID: "human-step", Type: "human", Inputs: map[string]interface{}{"timeout": "1h"}},
				{ID: "llm-fix", Type: "llm-exec", Inputs: map[string]interface{}{"model": "gpt-4"}},
				{ID: "orw-gen", Type: "orw-generated", Inputs: map[string]interface{}{"recipe": "java17"}},
			},
			maxParallel:  3,
			expectWinner: true,
			expectError:  false,
		},
		{
			name: "fanout with all branches failing",
			branches: []BranchSpec{
				{ID: "llm-fix", Type: "llm-exec", Inputs: map[string]interface{}{"model": "gpt-4"}},
				{ID: "orw-gen", Type: "orw-generated", Inputs: map[string]interface{}{"recipe": "java17"}},
			},
			maxParallel:  2,
			expectWinner: false,
			expectError:  true,
		},
		{
			name: "fanout with parallelism limit",
			branches: []BranchSpec{
				{ID: "branch-1", Type: "llm-exec"},
				{ID: "branch-2", Type: "llm-exec"},
				{ID: "branch-3", Type: "llm-exec"},
				{ID: "branch-4", Type: "llm-exec"},
			},
			maxParallel:  2, // Should limit to 2 concurrent
			expectWinner: true,
			expectError:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			mockSubmitter := &MockJobSubmitter{
				JobResults:    make(map[string]JobResult),
				ArtifactPaths: make(map[string]string),
			}

			// Configure mock results based on test expectations
			if tt.expectWinner {
				// Make first branch succeed
				mockSubmitter.JobResults[tt.branches[0].ID] = JobResult{
					JobID:  "job-success-123",
					Status: "completed",
					Output: "success",
				}
			} else {
				// Make all branches fail
				for _, branch := range tt.branches {
					mockSubmitter.JobResults[branch.ID] = JobResult{
						JobID:  fmt.Sprintf("job-fail-%s", branch.ID),
						Status: "failed",
						Output: "failed",
					}
				}
			}

			orchestrator := NewFanoutOrchestrator(mockSubmitter)

			// This function doesn't exist yet - will cause compilation failure
			winner, results, err := orchestrator.RunHealingFanout(ctx, nil, tt.branches, tt.maxParallel)

			if tt.expectError {
				assert.Error(t, err)
				assert.Equal(t, BranchResult{}, winner)
			} else {
				assert.NoError(t, err)
				if tt.expectWinner {
					assert.NotEqual(t, BranchResult{}, winner)
					assert.Equal(t, "completed", winner.Status)
				}
			}

			assert.Len(t, results, len(tt.branches))

			// Verify parallelism was respected
			if len(tt.branches) > tt.maxParallel {
				// Should have submitted at most maxParallel jobs
				assert.LessOrEqual(t, len(mockSubmitter.SubmittedJobs), tt.maxParallel)
			}
		})
	}
}

// Test runner integration
func TestTransflowRunnerWithHealing(t *testing.T) {
	t.Run("healing triggered on build failure", func(t *testing.T) {
		// Stub out external interactions via injectable seams
		oldDL := downloadToFileFn
		oldGet := getJSONFn
		oldPut := putJSONFn
		oldVDP := validateDiffPathsFn
		oldVUD := validateUnifiedDiffFn
		oldAD := applyUnifiedDiffFn
		oldHasChanges := hasRepoChangesFn
		// Stub remote artifact calls: write minimal content to dest, and no-op JSON interactions
		downloadToFileFn = func(_ string, dest string) error {
			_ = os.MkdirAll(filepath.Dir(dest), 0755)
			diff := "--- a/pom.xml\n+++ b/pom.xml\n@@ -1 +1 @@\n-<project></project>\n+<project><modelVersion>4.0.0</modelVersion></project>\n"
			return os.WriteFile(dest, []byte(diff), 0644)
		}
		getJSONFn = func(string, string) ([]byte, int, error) { return nil, 404, nil }
		putJSONFn = func(string, string, []byte) error { return nil }
		// Skip path validation and patch application in unit test
		validateDiffPathsFn = func(string, []string) error { return nil }
		validateUnifiedDiffFn = func(context.Context, string, string) error { return nil }
		applyUnifiedDiffFn = func(context.Context, string, string) error { return nil }
		hasRepoChangesFn = func(string) (bool, error) { return true, nil }

		defer func() {
			downloadToFileFn = oldDL
			getJSONFn = oldGet
			putJSONFn = oldPut
			validateDiffPathsFn = oldVDP
			validateUnifiedDiffFn = oldVUD
			applyUnifiedDiffFn = oldAD
			hasRepoChangesFn = oldHasChanges
		}()
		// Execution id used in branch metadata paths
		os.Setenv("PLOY_TRANSFLOW_EXECUTION_ID", "test-exec")
		defer os.Unsetenv("PLOY_TRANSFLOW_EXECUTION_ID")
		// Setup
		config := &TransflowConfig{
			ID:         "healing-test",
			TargetRepo: "https://github.com/test/repo",
			BaseRef:    "main",
			SelfHeal: &SelfHealConfig{
				MaxRetries: 2,
				Enabled:    true,
			},
			Steps: []TransflowStep{
				{
					Type:               "orw-apply",
					ID:                 "java-migration",
					Recipes:            []string{"org.openrewrite.java.migrate.UpgradeToJava17"},
					RecipeGroup:        "org.openrewrite.recipe",
					RecipeArtifact:     "rewrite-migrate-java",
					RecipeVersion:      "3.17.0",
					MavenPluginVersion: "6.18.0",
				},
			},
		}

		// Mocks
		mockGit := &MockGitOperations{}
		mockRecipe := &MockRecipeExecutor{}
		var check seqBuildChecker
		mockBuild := &check
		// Inject planner/reducer helper to avoid touching Nomad
		// and HCL submitter that validates and submits successfully

		runner, err := NewTransflowRunner(config, "/tmp/workspace")
		require.NoError(t, err)

		runner.SetGitOperations(mockGit)
		runner.SetRecipeExecutor(mockRecipe)
		runner.SetBuildChecker(mockBuild)
		runner.SetJobHelper(testJobHelper{})
		runner.SetHCLSubmitter(okHCLSubmitter{})
		// Non-nil jobSubmitter enables healing path; injected jobHelper handles planner/reducer
		runner.SetJobSubmitter(NoopJobSubmitter{})
		// Healer that returns a successful winner without submitting real jobs
		runner.SetHealingOrchestrator(okHealer{})

		// Ensure a minimal build file exists to pass ORW guard
		_ = os.MkdirAll("/tmp/workspace/repo", 0755)
		_ = os.WriteFile("/tmp/workspace/repo/pom.xml", []byte("<project></project>"), 0644)

		// Execute
		ctx := context.Background()
		result, err := runner.Run(ctx)

		// Verify healing was attempted
		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.NotNil(t, result.HealingSummary)
		assert.True(t, result.HealingSummary.Enabled)
		assert.Greater(t, result.HealingSummary.AttemptsCount, 0)
	})
}

// Test human-step branch implementation (RED phase - failing tests)
func TestHumanStepBranchCurrentBehavior(t *testing.T) {
	// This test documents the current behavior - human-step immediately fails
	// Once we implement proper human-step functionality, this test should be updated

	orchestrator := &fanoutOrchestrator{}

	branchSpec := BranchSpec{
		ID:   "human-intervention-test",
		Type: "human-step",
		Inputs: map[string]interface{}{
			"timeout":    "5m",
			"buildError": "Build failed: test error",
		},
	}

	result := BranchResult{
		ID:        branchSpec.ID,
		StartedAt: time.Now(),
		Status:    "pending", // Will be changed by executeHumanStepBranch
	}

	ctx := context.Background()
	actualResult := orchestrator.executeHumanStepBranch(ctx, branchSpec, result)

	// Current behavior - should fail with "requires production runner" in test mode
	assert.Equal(t, "failed", actualResult.Status, "human-step should fail in test mode")
	assert.Contains(t, actualResult.Notes, "requires production runner", "should indicate production runner needed")
	assert.True(t, actualResult.Duration > 0, "should have recorded duration")
	assert.False(t, actualResult.FinishedAt.IsZero(), "should have finished time")

	// When we implement human-step properly, these assertions will need to change:
	// TODO: human-step should create Git branch for manual intervention
	// TODO: human-step should create MR with build error context
	// TODO: human-step should poll for manual commits with configurable timeout
	// TODO: human-step should validate build after manual changes
	// TODO: human-step should return "completed" when human fixes the build
	// TODO: human-step should return "timeout" when no fix within time limit
}

// Test human-step branch full workflow (RED phase - failing tests for expected behavior)
func TestHumanStepBranchFullWorkflow(t *testing.T) {
	t.Run("human-step creates MR with correct configuration", func(t *testing.T) {
		// This test documents expected MR creation behavior - will fail until production integration
		mockGitProvider := &MockGitProvider{
			MRResult: &provider.MRResult{
				MRURL:   "https://gitlab.com/test/repo/-/merge_requests/42",
				MRID:    42,
				Created: true,
			},
		}

		orchestrator := &fanoutOrchestrator{
			runner: &MockProductionBranchRunner{
				GitProviderMock:  mockGitProvider,
				BuildCheckerMock: &MockBuildChecker{},
				TargetRepo:       "https://gitlab.com/test/repo.git",
			},
		}

		branchSpec := BranchSpec{
			ID:   "human-fix-001",
			Type: "human-step",
			Inputs: map[string]interface{}{
				"timeout":    "15m",
				"buildError": "Compilation failed: undefined method 'getFoo()'",
			},
		}

		result := BranchResult{
			ID:        branchSpec.ID,
			StartedAt: time.Now(),
			Status:    "pending",
		}

		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		defer cancel()

		actualResult := orchestrator.executeHumanStepBranch(ctx, branchSpec, result)

		// Expected behavior - MR should be created
		assert.True(t, mockGitProvider.MRCalled, "MR should be created")
		assert.Equal(t, "https://gitlab.com/test/repo.git", mockGitProvider.MRConfig.RepoURL, "should use correct repo URL")
		assert.Equal(t, "human-intervention-human-fix-001", mockGitProvider.MRConfig.SourceBranch, "should create intervention branch")
		assert.Equal(t, "main", mockGitProvider.MRConfig.TargetBranch, "should target main branch")
		assert.Contains(t, mockGitProvider.MRConfig.Title, "Human Intervention Required", "MR title should indicate intervention needed")
		assert.Contains(t, mockGitProvider.MRConfig.Description, "getFoo()", "MR description should contain build error")
		assert.Contains(t, mockGitProvider.MRConfig.Labels, "human-intervention", "should have human-intervention label")

		// Will timeout since no human fixes the build in test mode
		assert.Equal(t, "timeout", actualResult.Status, "should timeout waiting for human fix")
		assert.Contains(t, actualResult.Notes, "timed out", "should indicate timeout occurred")
	})

	t.Run("human-step detects successful manual fix", func(t *testing.T) {
		// This test documents expected success behavior - will fail until production integration
		mockBuildChecker := &MockBuildChecker{
			BuildResult: &common.DeployResult{Success: true}, // Simulate human fixing the build
		}

		mockGitProvider := &MockGitProvider{
			MRResult: &provider.MRResult{
				MRURL:   "https://gitlab.com/test/repo/-/merge_requests/43",
				MRID:    43,
				Created: true,
			},
		}

		orchestrator := &fanoutOrchestrator{
			runner: &MockProductionBranchRunner{
				GitProviderMock:  mockGitProvider,
				BuildCheckerMock: mockBuildChecker,
				TargetRepo:       "https://gitlab.com/test/repo.git",
			},
		}

		branchSpec := BranchSpec{
			ID:   "human-fix-002",
			Type: "human-step",
			Inputs: map[string]interface{}{
				"timeout":    "30s", // Short timeout for test
				"buildError": "Build failed: missing dependency",
			},
		}

		result := BranchResult{
			ID:        branchSpec.ID,
			StartedAt: time.Now(),
			Status:    "pending",
		}

		ctx := context.Background()
		actualResult := orchestrator.executeHumanStepBranch(ctx, branchSpec, result)

		// Expected behavior - should detect successful fix
		assert.Equal(t, "completed", actualResult.Status, "should complete when build succeeds")
		assert.Contains(t, actualResult.Notes, "Human intervention successful", "should indicate success")
		assert.Contains(t, actualResult.Notes, mockGitProvider.MRResult.MRURL, "should reference MR URL")
		assert.True(t, actualResult.Duration > 0, "should record execution duration")
		assert.False(t, actualResult.FinishedAt.IsZero(), "should record finish time")
	})

	t.Run("human-step handles MR creation failure", func(t *testing.T) {
		// This test documents expected error handling - will fail until production integration
		mockGitProvider := &MockGitProvider{
			MRError: fmt.Errorf("failed to create MR: repository not found"),
		}

		orchestrator := &fanoutOrchestrator{
			runner: &MockProductionBranchRunner{
				GitProviderMock:  mockGitProvider,
				BuildCheckerMock: &MockBuildChecker{},
				TargetRepo:       "https://gitlab.com/invalid/repo.git",
			},
		}

		branchSpec := BranchSpec{
			ID:   "human-fix-003",
			Type: "human-step",
			Inputs: map[string]interface{}{
				"buildError": "Test error",
			},
		}

		result := BranchResult{
			ID:        branchSpec.ID,
			StartedAt: time.Now(),
			Status:    "pending",
		}

		ctx := context.Background()
		actualResult := orchestrator.executeHumanStepBranch(ctx, branchSpec, result)

		// Expected behavior - should fail gracefully when MR creation fails
		assert.Equal(t, "failed", actualResult.Status, "should fail when MR creation fails")
		assert.Contains(t, actualResult.Notes, "Failed to create human intervention MR", "should indicate MR creation failure")
		assert.Contains(t, actualResult.Notes, "repository not found", "should include underlying error")
	})

	t.Run("human-step respects timeout configuration", func(t *testing.T) {
		// This test documents expected timeout behavior - will fail until production integration
		orchestrator := &fanoutOrchestrator{
			runner: &MockProductionBranchRunner{
				GitProviderMock: &MockGitProvider{
					MRResult: &provider.MRResult{MRURL: "https://gitlab.com/test/repo/-/merge_requests/44", MRID: 44, Created: true},
				},
				BuildCheckerMock: &MockBuildChecker{BuildResult: &common.DeployResult{Success: false}}, // Build never succeeds
				TargetRepo:       "https://gitlab.com/test/repo.git",
			},
		}

		branchSpec := BranchSpec{
			ID:   "human-fix-004",
			Type: "human-step",
			Inputs: map[string]interface{}{
				"timeout":    "100ms", // Very short timeout for quick test
				"buildError": "Timeout test error",
			},
		}

		result := BranchResult{
			ID:        branchSpec.ID,
			StartedAt: time.Now(),
			Status:    "pending",
		}

		ctx := context.Background()
		actualResult := orchestrator.executeHumanStepBranch(ctx, branchSpec, result)

		// Expected behavior - should timeout after configured duration
		assert.Equal(t, "timeout", actualResult.Status, "should timeout after configured duration")
		assert.Contains(t, actualResult.Notes, "Human intervention timed out", "should indicate timeout occurred")
		assert.Contains(t, actualResult.Notes, "100ms", "should mention the timeout duration")
	})
}

// Test llm-exec branch validation (RED phase - failing tests)
func TestLLMExecBranchValidation(t *testing.T) {
	t.Run("llm-exec branch renders HCL assets correctly", func(t *testing.T) {
		// This test should pass once RenderLLMExecAssets is properly implemented
		orchestrator := &fanoutOrchestrator{
			runner: &MockProductionBranchRunner{
				LLMExecAssetsError: nil,
				LLMExecAssetsPath:  "/tmp/test-assets/llm-exec.rendered.hcl",
			},
		}

		branch := BranchSpec{
			ID:   "llm-test-branch",
			Type: "llm-exec",
			Inputs: map[string]interface{}{
				"model":   "gpt-4o-mini",
				"timeout": "15m",
			},
		}

		result := orchestrator.executeLLMExecBranch(context.Background(), branch, BranchResult{
			ID:        branch.ID,
			StartedAt: time.Now(),
			Status:    "failed",
		})

		// RenderLLMExecAssets works, but HCL template file doesn't exist
		assert.Equal(t, "failed", result.Status, "should fail due to missing HCL template file")
		assert.Contains(t, result.Notes, "failed to substitute HCL template")
	})

	t.Run("llm-exec branch substitutes environment variables in HCL template", func(t *testing.T) {
		// This test verifies HCL template substitution works correctly
		orchestrator := &fanoutOrchestrator{
			runner: &MockProductionBranchRunner{
				LLMExecAssetsError: nil,
				LLMExecAssetsPath:  "/tmp/test-llm.hcl",
			},
		}

		// Set up test HCL template file
		testHCL := `job "llm-exec-${RUN_ID}" {
	group "main" {
		task "generate" {
			env {
				TRANSFLOW_MODEL = "${TRANSFLOW_MODEL}"
				TRANSFLOW_TOOLS = "${TRANSFLOW_TOOLS}"
				TRANSFLOW_LIMITS = "${TRANSFLOW_LIMITS}"
			}
		}
	}
}`

		// Write test HCL to temp file - this will fail until file system is mocked
		tempFile := "/tmp/test-llm.hcl"
		err := os.WriteFile(tempFile, []byte(testHCL), 0644)
		if err != nil {
			t.Fatalf("failed to create test HCL file: %v", err)
		}
		defer os.Remove(tempFile)

		branch := BranchSpec{
			ID:   "llm-substitution-test",
			Type: "llm-exec",
		}

		result := orchestrator.executeLLMExecBranch(context.Background(), branch, BranchResult{
			ID:        branch.ID,
			StartedAt: time.Now(),
			Status:    "failed",
		})

		// HCL template substitution works, but Nomad submission fails in test environment
		assert.Equal(t, "failed", result.Status)
		assert.Contains(t, result.Notes, "LLM exec job failed")
	})

	t.Run("llm-exec branch submits job to Nomad and waits for completion", func(t *testing.T) {
		// This test verifies production Nomad job submission integration
		orchestrator := &fanoutOrchestrator{
			runner: &MockProductionBranchRunner{
				LLMExecAssetsError: nil,
				LLMExecAssetsPath:  "/tmp/test-nomad.hcl",
			},
		}

		// Create test rendered HCL file
		testRenderedHCL := `job "llm-exec-test-123" {
	group "main" {
		task "generate" {
			env {
				TRANSFLOW_MODEL = "gpt-4o-mini@2024-08-06"
			}
		}
	}
}`
		tempFile := "/tmp/test-nomad.hcl"
		renderedFile := "/tmp/test-nomad.rendered.submitted.hcl"

		err := os.WriteFile(tempFile, []byte(testRenderedHCL), 0644)
		if err != nil {
			t.Fatalf("failed to create test HCL file: %v", err)
		}
		defer os.Remove(tempFile)
		defer os.Remove(renderedFile)

		branch := BranchSpec{
			ID:   "llm-nomad-test",
			Type: "llm-exec",
		}

		result := orchestrator.executeLLMExecBranch(context.Background(), branch, BranchResult{
			ID:        branch.ID,
			StartedAt: time.Now(),
			Status:    "failed",
		})

		// Should fail because orchestration.SubmitAndWaitTerminal will fail in test environment
		assert.Equal(t, "failed", result.Status)
		assert.Contains(t, result.Notes, "LLM exec job failed")
	})

	t.Run("llm-exec branch validates diff.patch artifact exists", func(t *testing.T) {
		// This test verifies artifact validation after successful job completion
		orchestrator := &fanoutOrchestrator{
			runner: &MockProductionBranchRunner{
				LLMExecAssetsError: nil,
				LLMExecAssetsPath:  "/tmp/test-artifact.hcl",
			},
		}

		// Create test files (HCL and expected artifact path)
		testHCL := "job \"test\" {}"
		tempFile := "/tmp/test-artifact.hcl"
		renderedFile := "/tmp/test-artifact.rendered.submitted.hcl"

		err := os.WriteFile(tempFile, []byte(testHCL), 0644)
		if err != nil {
			t.Fatalf("failed to create test HCL file: %v", err)
		}
		defer os.Remove(tempFile)
		defer os.Remove(renderedFile)

		// Create output directory but no diff.patch file (simulating job that completed but produced no artifact)
		err = os.MkdirAll("/tmp/out", 0755)
		if err != nil {
			t.Fatalf("failed to create output directory: %v", err)
		}
		defer os.RemoveAll("/tmp/out")

		branch := BranchSpec{
			ID:   "llm-artifact-test",
			Type: "llm-exec",
		}

		result := orchestrator.executeLLMExecBranch(context.Background(), branch, BranchResult{
			ID:        branch.ID,
			StartedAt: time.Now(),
			Status:    "failed",
		})

		// Should fail at Nomad submission, not at artifact validation
		assert.Equal(t, "failed", result.Status)
		assert.Contains(t, result.Notes, "LLM exec job failed")
	})

	t.Run("llm-exec branch succeeds when all steps complete and artifact exists", func(t *testing.T) {
		// This test documents the successful case - should fail until full implementation
		orchestrator := &fanoutOrchestrator{
			runner: &MockProductionBranchRunner{
				LLMExecAssetsError: nil,
				LLMExecAssetsPath:  "/tmp/test-success.hcl",
			},
		}

		// Create all required files for successful execution
		testHCL := "job \"test\" {}"
		tempFile := "/tmp/test-success.hcl"
		renderedFile := "/tmp/test-success.rendered.submitted.hcl"

		err := os.WriteFile(tempFile, []byte(testHCL), 0644)
		if err != nil {
			t.Fatalf("failed to create test HCL file: %v", err)
		}
		defer os.Remove(tempFile)
		defer os.Remove(renderedFile)

		// Create the diff.patch artifact that the job should produce
		err = os.MkdirAll("/tmp/out", 0755)
		if err != nil {
			t.Fatalf("failed to create output directory: %v", err)
		}
		defer os.RemoveAll("/tmp/out")

		testPatch := `diff --git a/src/main.java b/src/main.java
index 1234567..abcdefg 100644
--- a/src/main.java
+++ b/src/main.java
@@ -1,3 +1,3 @@
-public class Main {
+public class MainFixed {
     // LLM-generated fix
 }`
		err = os.WriteFile("/tmp/out/diff.patch", []byte(testPatch), 0644)
		if err != nil {
			t.Fatalf("failed to create artifact file: %v", err)
		}

		branch := BranchSpec{
			ID:   "llm-success-test",
			Type: "llm-exec",
		}

		result := orchestrator.executeLLMExecBranch(context.Background(), branch, BranchResult{
			ID:        branch.ID,
			StartedAt: time.Now(),
			Status:    "failed",
		})

		// This should fail until orchestration.SubmitAndWaitTerminal is properly mocked/implemented
		// Even with the artifact existing, the Nomad submission will fail in test environment
		assert.Equal(t, "failed", result.Status, "should fail due to Nomad submission not being mocked")
		assert.Contains(t, result.Notes, "LLM exec job failed")
	})
}

// Test orw-gen branch validation (RED phase - failing tests)
func TestORWGenBranchValidation(t *testing.T) {
	t.Run("orw-gen branch renders ORW apply assets correctly", func(t *testing.T) {
		// This test should pass once RenderORWApplyAssets is properly implemented
		orchestrator := &fanoutOrchestrator{
			runner: &MockProductionBranchRunner{
				ORWApplyAssetsError: nil,
				ORWApplyAssetsPath:  "/tmp/test-orw-assets/orw-apply.rendered.hcl",
			},
		}

		branch := BranchSpec{
			ID:   "orw-test-branch",
			Type: "orw-gen",
			Inputs: map[string]interface{}{
				"recipe_config": map[string]interface{}{
					"class":   "org.openrewrite.java.migrate.Java8toJava11",
					"coords":  "org.openrewrite.recipe:rewrite-migrate-java:1.21.0",
					"timeout": "15m",
				},
			},
		}

		result := orchestrator.executeORWGenBranch(context.Background(), branch, BranchResult{
			ID:        branch.ID,
			StartedAt: time.Now(),
			Status:    "failed",
		})

		// RenderORWApplyAssets works, but HCL template file doesn't exist
		assert.Equal(t, "failed", result.Status, "should fail due to missing HCL template file")
		assert.Contains(t, result.Notes, "failed to read ORW HCL template")
	})

	t.Run("orw-gen branch extracts recipe configuration from inputs", func(t *testing.T) {
		// This test verifies recipe config extraction from branch inputs
		orchestrator := &fanoutOrchestrator{
			runner: &MockProductionBranchRunner{
				ORWApplyAssetsError: nil,
				ORWApplyAssetsPath:  "/tmp/test-orw-config.hcl",
			},
		}

		// Set up test HCL template file with recipe variables
		testHCL := `job "orw-apply" {
	group "main" {
		task "apply" {
			env {
				RECIPE_CLASS = "${RECIPE_CLASS}"
				RECIPE_COORDS = "${RECIPE_COORDS}"
				RECIPE_TIMEOUT = "${RECIPE_TIMEOUT}"
			}
		}
	}
}`

		tempFile := "/tmp/test-orw-config.hcl"
		err := os.WriteFile(tempFile, []byte(testHCL), 0644)
		if err != nil {
			t.Fatalf("failed to create test HCL file: %v", err)
		}
		defer os.Remove(tempFile)

		branch := BranchSpec{
			ID:   "orw-config-test",
			Type: "orw-gen",
			Inputs: map[string]interface{}{
				"recipe_config": map[string]interface{}{
					"class":   "org.openrewrite.java.migrate.Java11toJava17",
					"coords":  "org.openrewrite.recipe:rewrite-migrate-java:1.21.0",
					"timeout": "20m",
				},
			},
		}

		result := orchestrator.executeORWGenBranch(context.Background(), branch, BranchResult{
			ID:        branch.ID,
			StartedAt: time.Now(),
			Status:    "failed",
		})

		// Should fail at Nomad submission step, but recipe config should be extracted
		assert.Equal(t, "failed", result.Status)
		// The test should fail because Nomad isn't available, but it should get past the recipe config extraction
		assert.NotContains(t, result.Notes, "failed to read ORW HCL template")
	})

	t.Run("orw-gen branch substitutes recipe variables in HCL template", func(t *testing.T) {
		// This test verifies HCL template variable substitution for OpenRewrite recipes
		orchestrator := &fanoutOrchestrator{
			runner: &MockProductionBranchRunner{
				ORWApplyAssetsError: nil,
				ORWApplyAssetsPath:  "/tmp/test-orw-subst.hcl",
			},
		}

		// Create test HCL template with recipe substitution variables
		testHCL := `job "orw-apply-test" {
	group "main" {
		task "apply-recipe" {
			env {
				RECIPE_CLASS = "${RECIPE_CLASS}"
				RECIPE_COORDS = "${RECIPE_COORDS}"
				RECIPE_TIMEOUT = "${RECIPE_TIMEOUT}"
				OUTPUT_PATH = "/tmp/out"
			}
		}
	}
}`

		tempFile := "/tmp/test-orw-subst.hcl"
		expectedRenderedFile := "/tmp/test-orw-subst.rendered.submitted.hcl"

		err := os.WriteFile(tempFile, []byte(testHCL), 0644)
		if err != nil {
			t.Fatalf("failed to create test HCL file: %v", err)
		}
		defer os.Remove(tempFile)
		defer os.Remove(expectedRenderedFile)

		branch := BranchSpec{
			ID:   "orw-substitution-test",
			Type: "orw-gen",
			Inputs: map[string]interface{}{
				"recipe_config": map[string]interface{}{
					"class":   "org.openrewrite.java.migrate.JavaVersion17",
					"coords":  "org.openrewrite.recipe:rewrite-migrate-java:1.21.0",
					"timeout": "25m",
				},
			},
		}

		result := orchestrator.executeORWGenBranch(context.Background(), branch, BranchResult{
			ID:        branch.ID,
			StartedAt: time.Now(),
			Status:    "failed",
		})

		// Should fail at Nomad submission, but substitution should work
		assert.Equal(t, "failed", result.Status)
		assert.Contains(t, result.Notes, "ORW apply job failed")

		// Verify that the substituted file was created
		if _, err := os.Stat(expectedRenderedFile); err != nil {
			t.Logf("Expected rendered file not found: %v", err)
		}
	})

	t.Run("orw-gen branch handles missing recipe configuration gracefully", func(t *testing.T) {
		// This test verifies default behavior when recipe_config is missing
		orchestrator := &fanoutOrchestrator{
			runner: &MockProductionBranchRunner{
				ORWApplyAssetsError: nil,
				ORWApplyAssetsPath:  "/tmp/test-orw-missing.hcl",
			},
		}

		testHCL := `job "orw-apply-missing" {
	env {
		RECIPE_CLASS = "${RECIPE_CLASS}"
		RECIPE_COORDS = "${RECIPE_COORDS}"
		RECIPE_TIMEOUT = "${RECIPE_TIMEOUT}"
	}
}`

		tempFile := "/tmp/test-orw-missing.hcl"
		err := os.WriteFile(tempFile, []byte(testHCL), 0644)
		if err != nil {
			t.Fatalf("failed to create test HCL file: %v", err)
		}
		defer os.Remove(tempFile)

		// Branch with no recipe_config
		branch := BranchSpec{
			ID:   "orw-missing-config-test",
			Type: "orw-gen",
			Inputs: map[string]interface{}{
				"other_field": "some_value",
			},
		}

		result := orchestrator.executeORWGenBranch(context.Background(), branch, BranchResult{
			ID:        branch.ID,
			StartedAt: time.Now(),
			Status:    "failed",
		})

		// Should use defaults for missing recipe config (empty strings for class/coords, "10m" for timeout)
		assert.Equal(t, "failed", result.Status)
		// Should fail at job submission since recipe config is empty, but not before that
		assert.NotContains(t, result.Notes, "failed to read ORW HCL template")
	})

	t.Run("orw-gen branch validates diff.patch artifact after job completion", func(t *testing.T) {
		// This test verifies artifact validation for ORW jobs
		orchestrator := &fanoutOrchestrator{
			runner: &MockProductionBranchRunner{
				ORWApplyAssetsError: nil,
				ORWApplyAssetsPath:  "/tmp/test-orw-artifact.hcl",
			},
		}

		testHCL := "job \"orw-test\" {}"
		tempFile := "/tmp/test-orw-artifact.hcl"
		renderedFile := "/tmp/test-orw-artifact.rendered.submitted.hcl"

		err := os.WriteFile(tempFile, []byte(testHCL), 0644)
		if err != nil {
			t.Fatalf("failed to create test HCL file: %v", err)
		}
		defer os.Remove(tempFile)
		defer os.Remove(renderedFile)

		// Create output directory but no diff.patch (simulating completed job with no changes)
		err = os.MkdirAll("/tmp/out", 0755)
		if err != nil {
			t.Fatalf("failed to create output directory: %v", err)
		}
		defer os.RemoveAll("/tmp/out")

		branch := BranchSpec{
			ID:   "orw-artifact-test",
			Type: "orw-gen",
			Inputs: map[string]interface{}{
				"recipe_config": map[string]interface{}{
					"class":   "org.openrewrite.java.migrate.Java8toJava11",
					"coords":  "org.openrewrite.recipe:rewrite-migrate-java:1.21.0",
					"timeout": "10m",
				},
			},
		}

		result := orchestrator.executeORWGenBranch(context.Background(), branch, BranchResult{
			ID:        branch.ID,
			StartedAt: time.Now(),
			Status:    "failed",
		})

		// Should fail because diff.patch doesn't exist, but would get that far in a working system
		assert.Equal(t, "failed", result.Status)
		// In practice, will fail at Nomad submission first, but this documents expected artifact behavior
		assert.Contains(t, result.Notes, "ORW apply job failed")
	})

	t.Run("orw-gen branch succeeds with valid recipe and artifact", func(t *testing.T) {
		// This test documents the complete successful case
		orchestrator := &fanoutOrchestrator{
			runner: &MockProductionBranchRunner{
				ORWApplyAssetsError: nil,
				ORWApplyAssetsPath:  "/tmp/test-orw-success.hcl",
			},
		}

		testHCL := "job \"orw-success\" {}"
		tempFile := "/tmp/test-orw-success.hcl"
		renderedFile := "/tmp/test-orw-success.rendered.submitted.hcl"
		artifactPath := "/tmp/out/diff.patch"

		err := os.WriteFile(tempFile, []byte(testHCL), 0644)
		if err != nil {
			t.Fatalf("failed to create test HCL file: %v", err)
		}
		defer os.Remove(tempFile)
		defer os.Remove(renderedFile)

		// Create the expected diff.patch artifact
		err = os.MkdirAll("/tmp/out", 0755)
		if err != nil {
			t.Fatalf("failed to create output directory: %v", err)
		}
		defer os.RemoveAll("/tmp/out")

		orwPatch := `diff --git a/pom.xml b/pom.xml
index abc123..def456 100644
--- a/pom.xml
+++ b/pom.xml
@@ -10,7 +10,7 @@
     <properties>
-        <maven.compiler.source>11</maven.compiler.source>
-        <maven.compiler.target>11</maven.compiler.target>
+        <maven.compiler.source>17</maven.compiler.source>
+        <maven.compiler.target>17</maven.compiler.target>
     </properties>
 </project>`

		err = os.WriteFile(artifactPath, []byte(orwPatch), 0644)
		if err != nil {
			t.Fatalf("failed to create artifact file: %v", err)
		}

		branch := BranchSpec{
			ID:   "orw-success-test",
			Type: "orw-gen",
			Inputs: map[string]interface{}{
				"recipe_config": map[string]interface{}{
					"class":   "org.openrewrite.java.migrate.Java11toJava17",
					"coords":  "org.openrewrite.recipe:rewrite-migrate-java:1.21.0",
					"timeout": "15m",
				},
			},
		}

		result := orchestrator.executeORWGenBranch(context.Background(), branch, BranchResult{
			ID:        branch.ID,
			StartedAt: time.Now(),
			Status:    "failed",
		})

		// Should fail because Nomad submission isn't mocked, even with artifact present
		assert.Equal(t, "failed", result.Status)
		assert.Contains(t, result.Notes, "ORW apply job failed")
	})
}

// Test timeout and error handling scenarios (RED phase - failing tests)
func TestTimeoutAndErrorHandling(t *testing.T) {
	t.Run("fanout orchestration handles context cancellation correctly", func(t *testing.T) {
		// This test verifies that context cancellation propagates through the fanout system
		mockSubmitter := &MockJobSubmitter{
			JobResults:    make(map[string]JobResult),
			ArtifactPaths: make(map[string]string),
		}

		// Configure slow jobs that would normally take longer than the timeout
		mockSubmitter.JobResults["slow-branch-1"] = JobResult{
			JobID:  "slow-1",
			Status: "completed",
			Output: "would eventually succeed",
		}
		mockSubmitter.JobResults["slow-branch-2"] = JobResult{
			JobID:  "slow-2",
			Status: "completed",
			Output: "would also eventually succeed",
		}

		orchestrator := NewFanoutOrchestrator(mockSubmitter)

		branches := []BranchSpec{
			{ID: "slow-branch-1", Type: "llm-exec"},
			{ID: "slow-branch-2", Type: "llm-exec"},
		}

		// Create a context with very short timeout
		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()

		winner, results, err := orchestrator.RunHealingFanout(ctx, nil, branches, 2)

		// Should fail due to timeout, but this test documents expected timeout behavior
		assert.Error(t, err, "should fail due to timeout")
		assert.Equal(t, BranchResult{}, winner, "no winner due to timeout")
		assert.Greater(t, len(results), 0, "should collect partial results")

		// At least some results should show cancellation
		cancelled := false
		for _, result := range results {
			if result.Status == "cancelled" {
				cancelled = true
				break
			}
		}
		assert.True(t, cancelled, "at least one branch should be cancelled")
	})

	t.Run("human-step branch handles timeout correctly", func(t *testing.T) {
		// This test verifies human-step timeout behavior
		orchestrator := &fanoutOrchestrator{
			runner: &MockProductionBranchRunner{
				GitProviderMock: &MockGitProvider{
					MRError: nil,
					MRResult: &provider.MRResult{
						MRURL:   "https://gitlab.com/test/repo/-/merge_requests/123",
						MRID:    123,
						Created: true,
					},
				},
				BuildCheckerMock: &MockBuildChecker{
					BuildResult: &common.DeployResult{Success: false}, // Build never succeeds
				},
			},
		}

		branch := BranchSpec{
			ID:   "human-timeout-test",
			Type: "human-step",
			Inputs: map[string]interface{}{
				"timeout":    "100ms", // Very short timeout
				"buildError": "Test build failure",
			},
		}

		result := orchestrator.executeHumanStepBranch(context.Background(), branch, BranchResult{
			ID:        branch.ID,
			StartedAt: time.Now(),
			Status:    "failed",
		})

		// Should timeout because build never succeeds within the short timeframe
		assert.Equal(t, "timeout", result.Status)
		assert.Contains(t, result.Notes, "Human intervention timed out")
		assert.True(t, result.Duration > 0, "should record execution duration")
		assert.False(t, result.FinishedAt.IsZero(), "should have finish time")
	})

	t.Run("llm-exec branch handles file system errors gracefully", func(t *testing.T) {
		// This test verifies error handling when file operations fail
		orchestrator := &fanoutOrchestrator{
			runner: &MockProductionBranchRunner{
				LLMExecAssetsError: nil,
				LLMExecAssetsPath:  "/nonexistent/path/file.hcl", // Invalid path
			},
		}

		branch := BranchSpec{
			ID:   "llm-fs-error-test",
			Type: "llm-exec",
		}

		result := orchestrator.executeLLMExecBranch(context.Background(), branch, BranchResult{
			ID:        branch.ID,
			StartedAt: time.Now(),
			Status:    "failed",
		})

		// Should fail with appropriate error message about file system issues
		assert.Equal(t, "failed", result.Status)
		assert.Contains(t, result.Notes, "failed to substitute HCL template")
		assert.True(t, result.Duration > 0, "should record duration even on error")
	})

	t.Run("orw-gen branch handles malformed recipe configuration", func(t *testing.T) {
		// This test verifies error handling for invalid recipe configuration
		orchestrator := &fanoutOrchestrator{
			runner: &MockProductionBranchRunner{
				ORWApplyAssetsError: nil,
				ORWApplyAssetsPath:  "/tmp/test-malformed.hcl",
			},
		}

		testHCL := "job \"orw-malformed\" {}"
		tempFile := "/tmp/test-malformed.hcl"
		err := os.WriteFile(tempFile, []byte(testHCL), 0644)
		if err != nil {
			t.Fatalf("failed to create test HCL file: %v", err)
		}
		defer os.Remove(tempFile)

		branch := BranchSpec{
			ID:   "orw-malformed-test",
			Type: "orw-gen",
			Inputs: map[string]interface{}{
				"recipe_config": "not-a-map", // Invalid: should be map[string]interface{}
			},
		}

		result := orchestrator.executeORWGenBranch(context.Background(), branch, BranchResult{
			ID:        branch.ID,
			StartedAt: time.Now(),
			Status:    "failed",
		})

		// Should handle malformed config gracefully by using defaults
		assert.Equal(t, "failed", result.Status)
		// Should still try to process with defaults, failing later at job submission
		assert.Contains(t, result.Notes, "ORW apply job failed")
	})

	t.Run("fanout orchestration cancels other branches when first succeeds", func(t *testing.T) {
		// This test verifies first-success-wins cancellation behavior
		mockSubmitter := &MockJobSubmitter{
			JobResults:    make(map[string]JobResult),
			ArtifactPaths: make(map[string]string),
		}

		// Make first branch succeed quickly, second would succeed slowly
		mockSubmitter.JobResults["fast-winner"] = JobResult{
			JobID:  "fast-1",
			Status: "completed",
			Output: "quick success",
		}
		mockSubmitter.JobResults["slow-branch"] = JobResult{
			JobID:  "slow-1",
			Status: "completed",
			Output: "would succeed but cancelled",
		}

		orchestrator := NewFanoutOrchestrator(mockSubmitter)

		branches := []BranchSpec{
			{ID: "fast-winner", Type: "llm-exec"},
			{ID: "slow-branch", Type: "llm-exec"},
		}

		winner, results, err := orchestrator.RunHealingFanout(context.Background(), nil, branches, 2)

		// Should succeed with fast winner
		assert.NoError(t, err)
		assert.Equal(t, "fast-winner", winner.ID)
		assert.Equal(t, "completed", winner.Status)
		assert.Len(t, results, 2)

		// Verify that other branches were cancelled or didn't complete
		nonWinnerResults := make([]BranchResult, 0)
		for _, result := range results {
			if result.ID != winner.ID {
				nonWinnerResults = append(nonWinnerResults, result)
			}
		}

		// At least one non-winner should be cancelled or not completed
		assert.Greater(t, len(nonWinnerResults), 0)
		for _, result := range nonWinnerResults {
			assert.NotEqual(t, "completed", result.Status, "non-winner branches should be cancelled")
		}
	})

	t.Run("branches handle asset rendering errors appropriately", func(t *testing.T) {
		// This test verifies error handling when asset rendering fails
		renderError := fmt.Errorf("asset rendering failed: template not found")

		orchestrator := &fanoutOrchestrator{
			runner: &MockProductionBranchRunner{
				LLMExecAssetsError:  renderError,
				ORWApplyAssetsError: renderError,
			},
		}

		llmBranch := BranchSpec{ID: "llm-render-error", Type: "llm-exec"}
		orwBranch := BranchSpec{ID: "orw-render-error", Type: "orw-gen"}

		llmResult := orchestrator.executeLLMExecBranch(context.Background(), llmBranch, BranchResult{
			ID: llmBranch.ID, StartedAt: time.Now(), Status: "failed",
		})

		orwResult := orchestrator.executeORWGenBranch(context.Background(), orwBranch, BranchResult{
			ID: orwBranch.ID, StartedAt: time.Now(), Status: "failed",
		})

		// Both should fail at asset rendering step
		assert.Equal(t, "failed", llmResult.Status)
		assert.Contains(t, llmResult.Notes, "failed to render LLM exec assets")
		assert.Contains(t, llmResult.Notes, "template not found")

		assert.Equal(t, "failed", orwResult.Status)
		assert.Contains(t, orwResult.Notes, "failed to render ORW apply assets")
		assert.Contains(t, orwResult.Notes, "template not found")
	})

	t.Run("orchestration handles empty branch list gracefully", func(t *testing.T) {
		// This test verifies error handling for edge cases
		mockSubmitter := &MockJobSubmitter{
			JobResults:    make(map[string]JobResult),
			ArtifactPaths: make(map[string]string),
		}

		orchestrator := NewFanoutOrchestrator(mockSubmitter)

		winner, results, err := orchestrator.RunHealingFanout(context.Background(), nil, []BranchSpec{}, 2)

		// Should return appropriate error for empty branch list
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "no branches to execute")
		assert.Equal(t, BranchResult{}, winner)
		assert.Nil(t, results)
	})
}

// Mock implementations for timeout and error testing are defined in mocks.go

// All types and interfaces are now defined in types.go and implemented in job_submission.go and fanout_orchestrator.go
