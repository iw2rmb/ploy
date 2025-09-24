package mods

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Test job submission helpers (planner)
func TestSubmitPlannerJob(t *testing.T) {
	tests := []struct {
		name          string
		config        *ModConfig
		buildError    string
		expectError   bool
		expectJobType string
	}{
		{
			name: "successful planner submission",
			config: &ModConfig{
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
			config: &ModConfig{
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

type stubPlannerSubmitter struct{}

func (stubPlannerSubmitter) SubmitAndWaitTerminal(ctx context.Context, spec JobSpec) (JobResult, error) {
	return JobResult{JobID: "planner-job", Status: "completed"}, nil
}

type fakeHCLSubmitter struct{}

func (fakeHCLSubmitter) Validate(string) error                                  { return nil }
func (fakeHCLSubmitter) Submit(string, time.Duration) error                     { return nil }
func (fakeHCLSubmitter) SubmitCtx(context.Context, string, time.Duration) error { return nil }

func TestSubmitPlannerJobFallbacksToControllerArtifacts(t *testing.T) {
	t.Setenv("MOD_ID", "mod-xyz")
	t.Setenv("PLOY_CONTROLLER", "https://controller.example/v1")
	t.Setenv("PLOY_SEAWEEDFS_URL", "http://seaweed.local")
	t.Setenv("NOMAD_DC", "dc1")

	workspace := t.TempDir()
	cfg := &ModConfig{
		ID:         "workflow",
		TargetRepo: "https://git.example/repo.git",
		BaseRef:    "main",
		Lane:       "D",
		Steps: []ModStep{{
			ID:   "s1",
			Type: string(StepTypeLLMExec),
		}},
	}

	runner, err := NewModRunner(cfg, workspace)
	require.NoError(t, err)

	origValidate := validateJob
	origSubmit := submitAndWaitTerminal
	origDownload := downloadToFileFn
	origHead := headURLFn
	origPut := putFileFn
	origWait := waitForStepContainingFn
	defer func() {
		validateJob = origValidate
		submitAndWaitTerminal = origSubmit
		downloadToFileFn = origDownload
		headURLFn = origHead
		putFileFn = origPut
		waitForStepContainingFn = origWait
	}()

	validateJob = func(string) error { return nil }
	submitAndWaitTerminal = func(string, time.Duration) error { return nil }
	headURLFn = func(string) bool { return true }
	putFileFn = func(string, string, string, string) error { return nil }
	waitForStepContainingFn = func(string, string, string, string, time.Duration) error { return nil }

	planJSON := `{"plan_id":"fallback","options":[{"id":"llm-1","type":"llm-exec"}]}`
	downloadToFileFn = func(url, dest string) error {
		if strings.Contains(url, "/artifacts/mods/mod-xyz/planner/") {
			return fmt.Errorf("http 404")
		}
		if strings.Contains(url, "/mods/mod-xyz/artifacts/plan_json") {
			return os.WriteFile(dest, []byte(planJSON), 0o644)
		}
		return fmt.Errorf("unexpected url: %s", url)
	}

	helper := NewJobSubmissionHelperWithRunner(stubPlannerSubmitter{}, runner)
	plan, err := helper.SubmitPlannerJob(context.Background(), cfg, "compilation failed", workspace)
	require.NoError(t, err)
	require.NotNil(t, plan)
	assert.Equal(t, "fallback", plan.PlanID)
	require.Len(t, plan.Options, 1)
	if typ, _ := plan.Options[0]["type"].(string); typ != "llm-exec" {
		t.Fatalf("unexpected option type: %s", typ)
	}
}

type recordingArtifactUploader struct {
	files []string
}

func (r *recordingArtifactUploader) UploadFile(ctx context.Context, baseURL, key, srcPath, contentType string) error {
	r.files = append(r.files, fmt.Sprintf("%s|%s|%s", baseURL, key, contentType))
	return nil
}

func (r *recordingArtifactUploader) UploadJSON(ctx context.Context, baseURL, key string, body []byte) error {
	return nil
}

func TestSubmitPlannerJobUsesArtifactUploader(t *testing.T) {
	t.Setenv("MOD_ID", "mod-abc")
	t.Setenv("PLOY_CONTROLLER", "https://controller.example/v1")
	t.Setenv("PLOY_SEAWEEDFS_URL", "http://seaweed.local")
	t.Setenv("NOMAD_DC", "dc1")

	workspace := t.TempDir()
	cfg := &ModConfig{
		ID:         "workflow",
		TargetRepo: "https://git.example/repo.git",
		BaseRef:    "main",
	}

	runner, err := NewModRunner(cfg, workspace)
	require.NoError(t, err)

	uploader := &recordingArtifactUploader{}
	runner.SetArtifactUploader(uploader)
	runner.SetHCLSubmitter(fakeHCLSubmitter{})

	origValidate := validateJob
	origSubmit := submitAndWaitTerminal
	origDownload := downloadToFileFn
	origHead := headURLFn
	origPut := putFileFn
	origWait := waitForStepContainingFn
	defer func() {
		validateJob = origValidate
		submitAndWaitTerminal = origSubmit
		downloadToFileFn = origDownload
		headURLFn = origHead
		putFileFn = origPut
		waitForStepContainingFn = origWait
	}()

	validateJob = func(string) error { return nil }
	submitAndWaitTerminal = func(string, time.Duration) error { return nil }
	headURLFn = func(string) bool { return true }
	putFileFn = func(base, key, srcPath, contentType string) error {
		t.Fatalf("legacy putFileFn should not be invoked: base=%s key=%s", base, key)
		return nil
	}
	waitForStepContainingFn = func(string, string, string, string, time.Duration) error { return nil }

	planJSON := `{"plan_id":"using-uploader","options":[{"id":"llm-1","type":"llm-exec"}]}`
	downloadToFileFn = func(url, dest string) error {
		if !strings.Contains(url, "/mods/mod-abc/artifacts/plan_json") {
			return fmt.Errorf("unexpected url: %s", url)
		}
		return os.WriteFile(dest, []byte(planJSON), 0o644)
	}

	helper := NewJobSubmissionHelperWithRunner(stubPlannerSubmitter{}, runner)
	plan, err := helper.SubmitPlannerJob(context.Background(), cfg, "compilation failed", workspace)
	if err == nil {
		require.NotNil(t, plan)
	} else {
		t.Fatalf("expected planner submission to succeed: %v", err)
	}
	require.NotEmpty(t, uploader.files, "expected artifact uploader to capture uploads")
}
