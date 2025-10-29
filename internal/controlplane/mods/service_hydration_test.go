package mods

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/iw2rmb/ploy/internal/controlplane/hydration"
	"github.com/iw2rmb/ploy/internal/controlplane/scheduler"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

// TestServiceSubmitReusesHydrationSnapshot verifies stage submission reuses cached snapshots when available.
func TestServiceSubmitReusesHydrationSnapshot(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	e, client := newTestEtcd(t)
	defer e.Close()
	defer client.Close()

	index, err := hydration.NewIndex(client, hydration.IndexOptions{
		Prefix: "hydration/index/",
		Clock: func() time.Time {
			return time.Date(2025, 10, 28, 21, 0, 0, 0, time.UTC)
		},
	})
	if err != nil {
		t.Fatalf("new hydration index: %v", err)
	}

	bundle := scheduler.BundleRecord{
		CID:       "bafy-snapshot",
		Digest:    "sha256:abcdef",
		Size:      8192,
		TTL:       scheduler.HydrationSnapshotTTL,
		ExpiresAt: time.Date(2025, 10, 29, 21, 0, 0, 0, time.UTC).Format(time.RFC3339Nano),
		Retained:  true,
	}
	if _, err := index.UpsertSnapshot(ctx, hydration.SnapshotRecord{
		RepoURL:  "https://git.example.com/org/repo.git",
		Revision: "deadbeef",
		TicketID: "mod-existing",
		Bundle:   bundle,
		Replication: hydration.ReplicationPolicy{
			Min: 2,
			Max: 3,
		},
		Sharing: hydration.SharingPolicy{Enabled: true},
	}); err != nil {
		t.Fatalf("upsert hydration snapshot: %v", err)
	}

	manifest := contracts.StepManifest{
		ID:    "mods-plan",
		Name:  "Mods Plan",
		Image: "ghcr.io/ploy/mods/plan:latest",
		Inputs: []contracts.StepInput{
			{
				Name:      "workspace",
				MountPath: "/workspace",
				Mode:      contracts.StepInputModeReadWrite,
				Hydration: &contracts.StepInputHydration{
					Repo: &contracts.RepoMaterialization{
						URL:    "https://git.example.com/org/repo.git",
						Commit: "deadbeef",
					},
				},
			},
		},
	}
	manifestBytes, err := json.Marshal(manifest)
	if err != nil {
		t.Fatalf("marshal manifest: %v", err)
	}

	schedulerStub := newFakeScheduler()
	service, err := NewService(client, Options{
		Prefix:    "mods/",
		Scheduler: schedulerStub,
		Clock: func() time.Time {
			return time.Date(2025, 10, 28, 21, 0, 0, 0, time.UTC)
		},
		Hydration: HydrationOptions{
			Index: index,
		},
	})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	defer func() {
		if cerr := service.Close(); cerr != nil {
			t.Fatalf("close service: %v", cerr)
		}
	}()

	spec := TicketSpec{
		TicketID:   "mod-new",
		Submitter:  "alice@example.com",
		Repository: "https://git.example.com/org/repo.git",
		Stages: []StageDefinition{
			{
				ID:          "mods-plan",
				MaxAttempts: 1,
				Metadata: map[string]string{
					"step_manifest":        string(manifestBytes),
					"hydration_repo_url":   "https://git.example.com/org/repo.git",
					"hydration_revision":   "deadbeef",
					"hydration_input_name": "workspace",
				},
			},
		},
	}

	if _, err := service.Submit(ctx, spec); err != nil {
		t.Fatalf("submit ticket: %v", err)
	}

	jobs := schedulerStub.SubmittedJobs()
	if len(jobs) != 1 {
		t.Fatalf("expected one job submitted, got %d", len(jobs))
	}

	updated, ok := jobs[0].Metadata["step_manifest"]
	if !ok {
		t.Fatalf("job metadata missing step_manifest")
	}
	var manifestWithReuse contracts.StepManifest
	if err := json.Unmarshal([]byte(updated), &manifestWithReuse); err != nil {
		t.Fatalf("decode manifest: %v", err)
	}
	input := manifestWithReuse.Inputs[0]
	if input.Hydration == nil || input.Hydration.BaseSnapshot.CID != bundle.CID {
		t.Fatalf("expected base snapshot cid %q injected, got %+v", bundle.CID, input.Hydration)
	}
	if input.Hydration.Repo == nil || input.Hydration.Repo.Commit != "deadbeef" {
		t.Fatalf("expected repo metadata retained")
	}
}
