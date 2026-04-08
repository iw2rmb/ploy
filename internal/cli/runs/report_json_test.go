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
		Repos: []RunEntry{
			{
				RepoID:    repoID,
				RepoURL:   "https://github.com/acme/service.git",
				BaseRef:   "main",
				TargetRef: "ploy/java17",
				Attempt:   1,
				Status:    "Running",
				PatchURL:  "https://example.test/patch",
				Jobs: []RunJobEntry{
					{
						JobID:               jobID,
						JobType:             "step",
						JobImage:            "ghcr.io/acme/runner:1",
						Status:              "Running",
						DurationMs:          1234,
						DisplayName:         "step-1",
						HookPlanReason:      "hook planned",
						HookConditionResult: "{\"evaluated\":true}",
						SBOMEvidence: &RunJobSBOMEvidence{
							ArtifactPresent:     boolPtr(true),
							ParsedPackageCount: intPtr(42),
						},
						Artifacts: []RunJobArtifact{
							{
								Name:      "gate-report",
								CID:       "bafy-gate-report",
								LookupURL: "https://example.test/v1/artifacts?cid=bafy-gate-report",
							},
						},
						JobLogURL: "https://example.test/v1/jobs/" + jobID.String() + "/logs",
						PatchURL:  "https://example.test/patch",
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

	for _, key := range []string{"run_id", "mig_id", "mig_name", "spec_id", "repos"} {
		if _, ok := parsed[key]; !ok {
			t.Fatalf("expected key %q in payload: %v", key, parsed)
		}
	}
	repos, ok := parsed["repos"].([]any)
	if !ok || len(repos) != 1 {
		t.Fatalf("expected one repo in payload: %v", parsed["repos"])
	}
	repo0, ok := repos[0].(map[string]any)
	if !ok {
		t.Fatalf("expected repo object, got %T", repos[0])
	}
	jobs, ok := repo0["jobs"].([]any)
	if !ok || len(jobs) != 1 {
		t.Fatalf("expected one job in payload: %v", repo0["jobs"])
	}
	job0, ok := jobs[0].(map[string]any)
	if !ok {
		t.Fatalf("expected job object, got %T", jobs[0])
	}
	artifacts, ok := job0["artifacts"].([]any)
	if !ok || len(artifacts) != 1 {
		t.Fatalf("expected one artifact in payload: %v", job0["artifacts"])
	}
	if got := job0["hook_plan_reason"]; got != "hook planned" {
		t.Fatalf("unexpected hook_plan_reason: %#v", got)
	}
	if got := job0["hook_condition_result"]; got != "{\"evaluated\":true}" {
		t.Fatalf("unexpected hook_condition_result: %#v", got)
	}
	sbomEvidence, ok := job0["sbom_evidence"].(map[string]any)
	if !ok {
		t.Fatalf("expected sbom_evidence object in payload: %#v", job0["sbom_evidence"])
	}
	if got := sbomEvidence["artifact_present"]; got != true {
		t.Fatalf("unexpected sbom_evidence.artifact_present: %#v", got)
	}
	if got := sbomEvidence["parsed_package_count"]; got != float64(42) {
		t.Fatalf("unexpected sbom_evidence.parsed_package_count: %#v", got)
	}
}

func TestRenderRunReportJSONOmitsEmptyOptionalFields(t *testing.T) {
	t.Parallel()

	report := RunReport{
		RunID:   domaintypes.NewRunID(),
		MigID:   domaintypes.NewMigID(),
		MigName: "minimal",
		SpecID:  domaintypes.NewSpecID(),
		Repos: []RunEntry{
			{
				RepoID:    domaintypes.NewMigRepoID(),
				RepoURL:   "https://github.com/acme/minimal.git",
				BaseRef:   "main",
				TargetRef: "ploy/minimal",
				Status:    "Queued",
				Attempt:   1,
			},
		},
	}

	var buf bytes.Buffer
	if err := RenderRunReportJSON(&buf, report); err != nil {
		t.Fatalf("RenderRunReportJSON error: %v", err)
	}

	out := buf.String()
	for _, field := range []string{
		"job_log_url",
		"patch_url",
		"last_error",
		"artifacts",
		"hook_plan_reason",
		"hook_condition_result",
		"sbom_evidence",
	} {
		if strings.Contains(out, "\""+field+"\"") {
			t.Fatalf("expected %q omitted for empty optional fields; got %q", field, out)
		}
	}
}

func boolPtr(v bool) *bool {
	return &v
}

func intPtr(v int) *int {
	return &v
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
