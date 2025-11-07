package contracts

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	types "github.com/iw2rmb/ploy/internal/domain/types"
)

const (
	modsStage      = "mods-plan"
	buildGateStage = "build-gate"
)

func TestSubjectsForTicket(t *testing.T) {
	subjects := SubjectsForTicket("ticket-123")
	if subjects.CheckpointStream != "ploy.workflow.ticket-123.checkpoints" {
		t.Fatalf("CheckpointStream mismatch: %s", subjects.CheckpointStream)
	}
	if subjects.ArtifactStream != "ploy.artifact.ticket-123" {
		t.Fatalf("ArtifactStream mismatch: %s", subjects.ArtifactStream)
	}
	if subjects.StatusStream != "jobs.ticket-123.events" {
		t.Fatalf("StatusStream mismatch: %s", subjects.StatusStream)
	}
}

func TestSubjectsForTicketTrimsInput(t *testing.T) {
	subjects := SubjectsForTicket("  ticket-123  ")
	if subjects.CheckpointStream != "ploy.workflow.ticket-123.checkpoints" {
		t.Fatalf("CheckpointStream mismatch: %s", subjects.CheckpointStream)
	}
	if subjects.ArtifactStream != "ploy.artifact.ticket-123" {
		t.Fatalf("ArtifactStream mismatch: %s", subjects.ArtifactStream)
	}
	if subjects.StatusStream != "jobs.ticket-123.events" {
		t.Fatalf("StatusStream mismatch: %s", subjects.StatusStream)
	}
}

func TestSubjectsForTicketEmptyTicket(t *testing.T) {
	subjects := SubjectsForTicket("")
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

func TestWorkflowTicketValidate(t *testing.T) {
	ticket := WorkflowTicket{}
	if err := ticket.Validate(); err == nil {
		t.Fatal("expected validation error for empty ticket")
	}

	valid := WorkflowTicket{
		SchemaVersion: SchemaVersion,
		TicketID:      types.TicketID("ticket-123"),
		Manifest:      ManifestReference{Name: "smoke", Version: "2025-09-26"},
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("expected valid ticket, got %v", err)
	}

	withRepo := WorkflowTicket{
		SchemaVersion: SchemaVersion,
		TicketID:      types.TicketID("ticket-456"),
		Manifest:      ManifestReference{Name: "smoke", Version: "2025-09-26"},
		Repo: RepoMaterialization{
			URL:       types.RepoURL("https://gitlab.com/iw2rmb/sample.git"),
			BaseRef:   types.GitRef("main"),
			TargetRef: types.GitRef("mods/shift-grid"),
		},
	}
	if err := withRepo.Validate(); err != nil {
		t.Fatalf("expected ticket with repo to validate, got %v", err)
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

func TestWorkflowCheckpointValidateAndMarshal(t *testing.T) {
	empty := WorkflowCheckpoint{}
	if err := empty.Validate(); err == nil {
		t.Fatal("expected validation error for empty checkpoint")
	}

	cp := WorkflowCheckpoint{
		SchemaVersion: SchemaVersion,
		TicketID:      types.TicketID("ticket-123"),
		Stage:         StageName(modsStage),
		Status:        CheckpointStatusPending,
		CacheKey:      "node-wasm/cache@manifest=2025-09-26@aster=plan",
		StageMetadata: &CheckpointStage{
			Name:         modsStage,
			Kind:         modsStage,
			Lane:         "node-wasm",
			Dependencies: []string{},
			Manifest:     ManifestReference{Name: "smoke", Version: "2025-09-26"},
			Aster: CheckpointStageAster{
				Enabled: true,
				Toggles: []string{"plan"},
				Bundles: []CheckpointAsterBundle{{
					Stage:       modsStage,
					Toggle:      "plan",
					BundleID:    "mods-plan",
					Digest:      "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
					ArtifactCID: "cid-mods-plan",
				}},
			},
			Mods: &ModsStageMetadata{
				Plan: &ModsPlanMetadata{
					SelectedRecipes: []string{"recipe.alpha"},
					ParallelStages:  []string{"orw-apply", "orw-gen"},
					HumanGate:       true,
					Summary:         "apply recipe.alpha then review",
					PlanTimeout:     "2m",
					MaxParallel:     4,
				},
				Human: &ModsHumanMetadata{
					Required:  true,
					Playbooks: []string{"playbook.mods.review"},
				},
				Recommendations: []ModsRecommendation{{
					Source:     "knowledge-base",
					Message:    "Apply recipe.alpha before llm-exec",
					Confidence: 0.9,
				}},
			},
		},
		Artifacts: []CheckpointArtifact{{
			Name:        "mods-plan-bundle",
			ArtifactCID: types.CID("cid-mods-plan"),
			Digest:      types.Sha256Digest("sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"),
			MediaType:   "application/tar+zst",
		}},
	}
	if err := cp.Validate(); err != nil {
		t.Fatalf("expected valid checkpoint, got %v", err)
	}

	payload, err := json.Marshal(cp)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}
	if !strings.Contains(string(payload), SchemaVersion) {
		t.Fatalf("expected payload to contain schema version %q: %s", SchemaVersion, string(payload))
	}
	var decoded map[string]any
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	meta, ok := decoded["stage_metadata"].(map[string]any)
	if !ok {
		t.Fatalf("expected stage metadata in payload: %v", decoded)
	}
	if _, ok := meta["mods"].(map[string]any); !ok {
		t.Fatalf("expected mods metadata in stage metadata: %v", meta)
	}
	if artifacts, ok := decoded["artifacts"].([]any); !ok || len(artifacts) == 0 {
		t.Fatalf("expected artifacts in payload: %v", decoded)
	}
	if cp.Subject() != "ploy.workflow.ticket-123.checkpoints" {
		t.Fatalf("unexpected subject: %s", cp.Subject())
	}
}

func TestBuildGateMetadataValidate(t *testing.T) {
	meta := BuildGateStageMetadata{
		LogDigest: "bafy-build",
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
			Code:     "kb.git.auth",
			Severity: "error",
			Message:  "Authenticate Git fetch credentials for remote repository access.",
			Evidence: "fatal: unable to access 'https://example.com/repo'",
		}},
	}
	stage := CheckpointStage{
		Name:      buildGateStage,
		Kind:      buildGateStage,
		Lane:      "build-gate",
		Manifest:  ManifestReference{Name: "smoke", Version: "2025-09-26"},
		BuildGate: &meta,
	}
	if err := stage.Validate(); err != nil {
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
			Code:    "kb.git.auth",
			Message: "missing severity",
		}},
	}
	if err := meta.Validate(); err == nil {
		t.Fatal("expected validation error for log finding without severity")
	}
}

func TestWorkflowArtifactValidate(t *testing.T) {
	empty := WorkflowArtifact{}
	if err := empty.Validate(); err == nil {
		t.Fatal("expected validation error for empty artifact envelope")
	}

	envelope := WorkflowArtifact{
		SchemaVersion: SchemaVersion,
		TicketID:      types.TicketID("ticket-123"),
		Stage:         StageName(modsStage),
		CacheKey:      "node-wasm/cache@manifest=2025-09-26@aster=plan",
		StageMetadata: &CheckpointStage{
			Name:     modsStage,
			Kind:     modsStage,
			Lane:     "node-wasm",
			Manifest: ManifestReference{Name: "smoke", Version: "2025-09-26"},
		},
		Artifact: CheckpointArtifact{
			Name:        "mods-plan",
			ArtifactCID: types.CID("cid-mods-plan"),
			Digest:      types.Sha256Digest("sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"),
			MediaType:   "application/tar+zst",
		},
	}
	if err := envelope.Validate(); err != nil {
		t.Fatalf("expected valid artifact envelope, got %v", err)
	}

	if subject := envelope.Subject(); subject != "ploy.artifact.ticket-123" {
		t.Fatalf("unexpected artifact subject: %s", subject)
	}
}

func TestModsPlanMetadataValidateRejectsInvalidValues(t *testing.T) {
	invalidTimeout := ModsPlanMetadata{PlanTimeout: "not-a-duration"}
	if err := invalidTimeout.Validate(); err == nil {
		t.Fatal("expected validation error for invalid plan timeout")
	}

	invalidParallel := ModsPlanMetadata{MaxParallel: -1}
	if err := invalidParallel.Validate(); err == nil {
		t.Fatal("expected validation error for negative max parallel")
	}

	valid := ModsPlanMetadata{PlanTimeout: "90s", MaxParallel: 3}
	if err := valid.Validate(); err != nil {
		t.Fatalf("expected valid metadata, got %v", err)
	}
}

func TestInMemoryBusRecordsMessages(t *testing.T) {
	bus := NewInMemoryBus()
	ticket, err := bus.ClaimTicket(context.Background(), "ticket-123")
	if err != nil {
		t.Fatalf("claim error: %v", err)
	}
	// tenant removed
	if len(bus.ClaimedTickets) != 1 {
		t.Fatalf("expected claimed ticket to be recorded")
	}
	if ticket.Manifest.Name == "" || ticket.Manifest.Version == "" {
		t.Fatalf("expected manifest reference to be set, got %+v", ticket.Manifest)
	}

	checkpoint := WorkflowCheckpoint{
		SchemaVersion: SchemaVersion,
		TicketID:      types.TicketID("ticket-123"),
		Stage:         StageName("ticket-claimed"),
		Status:        CheckpointStatusClaimed,
	}
	if err := bus.PublishCheckpoint(context.Background(), checkpoint); err != nil {
		t.Fatalf("publish error: %v", err)
	}
	if len(bus.Checkpoints) != 1 {
		t.Fatalf("expected checkpoint to be recorded")
	}

	artifact := WorkflowArtifact{
		SchemaVersion: SchemaVersion,
		TicketID:      types.TicketID("ticket-123"),
		Stage:         StageName(modsStage),
		Artifact: CheckpointArtifact{
			Name:        "mods-plan",
			ArtifactCID: types.CID("cid-mods-plan"),
		},
	}
	if err := bus.PublishArtifact(context.Background(), artifact); err != nil {
		t.Fatalf("publish artifact error: %v", err)
	}
	if len(bus.Artifacts) != 1 {
		t.Fatalf("expected artifact envelope to be recorded")
	}
}

func TestInMemoryBusAutoTicketFallback(t *testing.T) {
	bus := NewInMemoryBus()
	bus.EnqueueTicket("queued-1")
	ticket, err := bus.ClaimTicket(context.Background(), "")
	if err != nil {
		t.Fatalf("claim error: %v", err)
	}
	if ticket.TicketID != "queued-1" {
		t.Fatalf("expected queued ticket, got %s", ticket.TicketID)
	}
	if len(bus.ClaimedTickets) != 1 || bus.ClaimedTickets[0] != "queued-1" {
		t.Fatalf("unexpected claimed tickets: %v", bus.ClaimedTickets)
	}

	second, err := bus.ClaimTicket(context.Background(), "")
	if err != nil {
		t.Fatalf("claim error: %v", err)
	}
	if second.TicketID == "" {
		t.Fatal("expected auto-generated ticket id")
	}
	if second.TicketID == "queued-1" {
		t.Fatal("expected different ticket id for auto fallback")
	}
	if len(bus.ClaimedTickets) != 2 {
		t.Fatalf("expected two claimed tickets, got %v", bus.ClaimedTickets)
	}
	if second.Manifest.Name == "" || second.Manifest.Version == "" {
		t.Fatalf("expected auto manifest assignment, got %+v", second.Manifest)
	}
}

// TestModsStageMetadataValidateRequiresMessage ensures recommendations require messages.
func TestModsStageMetadataValidateRequiresMessage(t *testing.T) {
	meta := ModsStageMetadata{
		Recommendations: []ModsRecommendation{{Source: "knowledge-base", Message: ""}},
	}
	if err := meta.Validate(); err == nil {
		t.Fatal("expected validation error for empty recommendation message")
	}
}

// TestModsRecommendationValidateBounds enforces confidence bounds.
func TestModsRecommendationValidateBounds(t *testing.T) {
	rec := ModsRecommendation{Message: "ok", Confidence: 1.2}
	if err := rec.Validate(); err == nil {
		t.Fatal("expected confidence validation error")
	}
}

// TestModsHumanMetadataValidate ensures blank playbooks are rejected.
func TestModsHumanMetadataValidate(t *testing.T) {
	meta := ModsHumanMetadata{Required: true, Playbooks: []string{"", "playbook.mods.review"}}
	if err := meta.Validate(); err == nil {
		t.Fatal("expected validation error for blank playbook")
	}
}
