package runs

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
)

func TestRenderRunReportJSON(t *testing.T) {
	t.Parallel()

	runID := domaintypes.NewRunID()
	migID := domaintypes.NewMigID()
	specID := domaintypes.NewSpecID()
	repoID := domaintypes.NewMigRepoID()
	jobID := domaintypes.NewJobID()

	report := RunReport{
		RunID:   runID,
		MigID:   migID,
		MigName: "java17-upgrade",
		SpecID:  specID,
		Repos: []RepoReport{
			{
				RepoID:      repoID,
				RepoURL:     "https://github.com/acme/service.git",
				BaseRef:     "main",
				TargetRef:   "ploy/java17",
				Status:      "Running",
				Attempt:     1,
				BuildLogURL: "https://example.test/logs",
				PatchURL:    "https://example.test/patch",
			},
		},
		Runs: []RunEntry{
			{
				RepoID:      repoID,
				RepoURL:     "https://github.com/acme/service.git",
				BaseRef:     "main",
				TargetRef:   "ploy/java17",
				Attempt:     1,
				Status:      "Running",
				BuildLogURL: "https://example.test/logs",
				PatchURL:    "https://example.test/patch",
				Jobs: []RunJobEntry{
					{
						JobID:       jobID,
						JobType:     "step",
						JobImage:    "ghcr.io/acme/runner:1",
						Status:      "Running",
						DurationMs:  1234,
						DisplayName: "step-1",
						BuildLogURL: "https://example.test/logs",
						PatchURL:    "https://example.test/patch",
					},
				},
			},
		},
	}

	var buf bytes.Buffer
	if err := RenderRunReportJSON(&buf, report); err != nil {
		t.Fatalf("RenderRunReportJSON error: %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(buf.Bytes(), &parsed); err != nil {
		t.Fatalf("json unmarshal: %v", err)
	}

	for _, key := range []string{"run_id", "mig_id", "mig_name", "spec_id", "repos", "runs"} {
		if _, ok := parsed[key]; !ok {
			t.Fatalf("expected key %q in payload: %v", key, parsed)
		}
	}
}

func TestRenderRunReportJSONOmitsEmptyOptionalFields(t *testing.T) {
	t.Parallel()

	report := RunReport{
		RunID:   domaintypes.NewRunID(),
		MigID:   domaintypes.NewMigID(),
		MigName: "minimal",
		SpecID:  domaintypes.NewSpecID(),
		Repos: []RepoReport{
			{
				RepoID:    domaintypes.NewMigRepoID(),
				RepoURL:   "https://github.com/acme/minimal.git",
				BaseRef:   "main",
				TargetRef: "ploy/minimal",
				Status:    "Queued",
				Attempt:   1,
			},
		},
		Runs: []RunEntry{},
	}

	var buf bytes.Buffer
	if err := RenderRunReportJSON(&buf, report); err != nil {
		t.Fatalf("RenderRunReportJSON error: %v", err)
	}

	out := buf.String()
	if strings.Contains(out, "\"build_log_url\"") {
		t.Fatalf("expected build_log_url omitted for empty optional fields; got %q", out)
	}
	if strings.Contains(out, "\"patch_url\"") {
		t.Fatalf("expected patch_url omitted for empty optional fields; got %q", out)
	}
	if strings.Contains(out, "\"last_error\"") {
		t.Fatalf("expected last_error omitted for empty optional fields; got %q", out)
	}
}

func TestRenderRunReportJSONRequiresWriter(t *testing.T) {
	t.Parallel()

	err := RenderRunReportJSON(nil, RunReport{})
	if err == nil {
		t.Fatal("expected error for nil writer")
	}
	if !strings.Contains(err.Error(), "output writer required") {
		t.Fatalf("unexpected error: %v", err)
	}
}
