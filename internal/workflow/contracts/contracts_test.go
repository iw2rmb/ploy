package contracts

import (
	"testing"

	types "github.com/iw2rmb/ploy/internal/domain/types"
)

func TestSubjectsForRun(t *testing.T) {
	subjects := SubjectsForRun(types.RunID("run-123"))
	if subjects.CheckpointStream != "ploy.workflow.run-123.checkpoints" {
		t.Fatalf("CheckpointStream mismatch: %s", subjects.CheckpointStream)
	}
	if subjects.ArtifactStream != "ploy.artifact.run-123" {
		t.Fatalf("ArtifactStream mismatch: %s", subjects.ArtifactStream)
	}
	if subjects.StatusStream != "jobs.run-123.events" {
		t.Fatalf("StatusStream mismatch: %s", subjects.StatusStream)
	}
}

func TestSubjectsForRunTrimsInput(t *testing.T) {
	subjects := SubjectsForRun(types.RunID("  run-123  "))
	if subjects.CheckpointStream != "ploy.workflow.run-123.checkpoints" {
		t.Fatalf("CheckpointStream mismatch: %s", subjects.CheckpointStream)
	}
	if subjects.ArtifactStream != "ploy.artifact.run-123" {
		t.Fatalf("ArtifactStream mismatch: %s", subjects.ArtifactStream)
	}
	if subjects.StatusStream != "jobs.run-123.events" {
		t.Fatalf("StatusStream mismatch: %s", subjects.StatusStream)
	}
}

func TestSubjectsForRunEmptyRunID(t *testing.T) {
	subjects := SubjectsForRun(types.RunID(""))
	if subjects.CheckpointStream != "" {
		t.Fatalf("expected empty checkpoint stream, got %s", subjects.CheckpointStream)
	}
	if subjects.ArtifactStream != "" {
		t.Fatalf("expected empty artifact stream, got %s", subjects.ArtifactStream)
	}
	if subjects.StatusStream != "" {
		t.Fatalf("expected empty status stream, got %s", subjects.StatusStream)
	}
}

func TestWorkflowRunValidate(t *testing.T) {
	run := WorkflowRun{}
	if err := run.Validate(); err == nil {
		t.Fatal("expected validation error for empty run envelope")
	}

	valid := WorkflowRun{
		SchemaVersion: SchemaVersion,
		RunID:         types.RunID("run-123"),
		Manifest:      ManifestReference{Name: "smoke", Version: "2025-09-26"},
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("expected valid run envelope, got %v", err)
	}

	withRepo := WorkflowRun{
		SchemaVersion: SchemaVersion,
		RunID:         types.RunID("run-456"),
		Manifest:      ManifestReference{Name: "smoke", Version: "2025-09-26"},
		Repo: RepoMaterialization{
			URL:       types.RepoURL("https://gitlab.com/iw2rmb/sample.git"),
			BaseRef:   types.GitRef("main"),
			TargetRef: types.GitRef("migs/example-grid"),
		},
	}
	if err := withRepo.Validate(); err != nil {
		t.Fatalf("expected run with repo to validate, got %v", err)
	}

	badRepo := valid
	badRepo.Repo = RepoMaterialization{URL: types.RepoURL("https://example.com/repo.git")}
	if err := badRepo.Validate(); err == nil {
		t.Fatal("expected repo validation error when target ref missing")
	}

	commitOnly := valid
	commitOnly.Repo = RepoMaterialization{
		URL:    types.RepoURL("https://gitlab.com/iw2rmb/sample.git"),
		Commit: types.CommitSHA("abcdef1234567890"),
	}
	if err := commitOnly.Validate(); err != nil {
		t.Fatalf("expected repo with commit to validate, got %v", err)
	}

	invalidScheme := valid
	invalidScheme.Repo = RepoMaterialization{
		URL:       types.RepoURL("http://example.com/repo.git"),
		TargetRef: types.GitRef("main"),
	}
	if err := invalidScheme.Validate(); err == nil {
		t.Fatal("expected validation error for invalid repo url scheme")
	}
}

func TestBuildGateMetadataValidate(t *testing.T) {
	meta := BuildGateStageMetadata{
		LogDigest: types.Sha256Digest("sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"),
		StaticChecks: []BuildGateStaticCheckReport{{
			Language: "go",
			Tool:     "go vet",
			Passed:   false,
			Failures: []BuildGateStaticCheckFailure{{
				RuleID:   "GOVET001",
				File:     "internal/pkg/main.go",
				Line:     12,
				Column:   4,
				Severity: "warning",
				Message:  "unused result",
			}},
		}},
		LogFindings: []BuildGateLogFinding{{
			Code:     "git.auth",
			Severity: "error",
			Message:  "Authenticate Git fetch credentials for remote repository access.",
			Evidence: "fatal: unable to access 'https://example.com/repo'",
		}},
	}
	if err := meta.Validate(); err != nil {
		t.Fatalf("expected valid build gate metadata, got %v", err)
	}
}

func TestBuildGateMetadataValidateRejectsEmptyFailureMessage(t *testing.T) {
	meta := BuildGateStageMetadata{
		StaticChecks: []BuildGateStaticCheckReport{{
			Language: "go",
			Tool:     "go vet",
			Failures: []BuildGateStaticCheckFailure{{
				RuleID:   "GOVET001",
				File:     "main.go",
				Severity: "error",
			}},
		}},
	}
	if err := meta.Validate(); err == nil {
		t.Fatal("expected validation error for missing failure message")
	}
}

func TestBuildGateMetadataValidateRejectsInvalidLogFinding(t *testing.T) {
	meta := BuildGateStageMetadata{
		LogFindings: []BuildGateLogFinding{{
			Code:    "git.auth",
			Message: "missing severity",
		}},
	}
	if err := meta.Validate(); err == nil {
		t.Fatal("expected validation error for log finding without severity")
	}
}

func TestBuildGateMetadataValidate_AcceptsReportLinks(t *testing.T) {
	meta := BuildGateStageMetadata{
		StaticChecks: []BuildGateStaticCheckReport{{
			Tool:   "gradle",
			Passed: true,
		}},
		ReportLinks: []BuildGateReportLink{
			{
				Type:        BuildGateReportTypeGradleJUnitXML,
				Path:        "/out/gradle-test-results",
				ArtifactID:  "017f22f9-9a5f-4b39-88c1-3894f0f4c04d",
				BundleCID:   "bafybeigdyrzt2qlr5l4ck2u6m3m3nv2tj7kq6ejm2lqk2ccx7lyy6ebmvu",
				URL:         "http://localhost:8080/v1/artifacts/017f22f9-9a5f-4b39-88c1-3894f0f4c04d",
				DownloadURL: "http://localhost:8080/v1/artifacts/017f22f9-9a5f-4b39-88c1-3894f0f4c04d?download=true",
			},
			{
				Type:       BuildGateReportTypeGradleHTML,
				Path:       "/out/gradle-test-report",
				ArtifactID: "7bb10136-151d-4665-82cd-6404f37ea3ca",
				URL:        "http://localhost:8080/v1/artifacts/7bb10136-151d-4665-82cd-6404f37ea3ca",
			},
		},
	}
	if err := meta.Validate(); err != nil {
		t.Fatalf("expected report links to validate, got %v", err)
	}
}

func TestBuildGateMetadataValidate_RejectsInvalidReportLinks(t *testing.T) {
	meta := BuildGateStageMetadata{
		StaticChecks: []BuildGateStaticCheckReport{{
			Tool:   "gradle",
			Passed: true,
		}},
		ReportLinks: []BuildGateReportLink{{
			Type:       "unknown",
			Path:       "relative/path",
			ArtifactID: "",
			URL:        "http://localhost:8080/v1/artifacts/x",
		}},
	}
	if err := meta.Validate(); err == nil {
		t.Fatal("expected validation error for invalid report link")
	}
}

// TestBuildGateValidateRequestValidate tests the ref-only BuildGateValidateRequest validation.
