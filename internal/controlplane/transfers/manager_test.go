package transfers_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	controlplaneartifacts "github.com/iw2rmb/ploy/internal/controlplane/artifacts"
	"github.com/iw2rmb/ploy/internal/controlplane/transfers"
	workflowartifacts "github.com/iw2rmb/ploy/internal/workflow/artifacts"
)

func TestUploadAndDownloadSlotLifecycle(t *testing.T) {
	now := time.Date(2025, 10, 26, 10, 0, 0, 0, time.UTC)
	mgr := transfers.NewManager(transfers.Options{Now: func() time.Time { return now }})
	slot, err := mgr.CreateUploadSlot(transfers.KindRepo, "job-1", "plan", "node-a", 1024)
	if err != nil {
		t.Fatalf("CreateUploadSlot: %v", err)
	}
	if slot.RemotePath == "" {
		t.Fatalf("expected remote path")
	}
	if _, err := mgr.Commit(context.Background(), slot.ID, 1024, "sha256:deadbeef"); err != nil {
		t.Fatalf("Commit: %v", err)
	}
	downloadSlot, artifact, err := mgr.CreateDownloadSlot("job-1", slot.ID, transfers.KindRepo)
	if err != nil {
		t.Fatalf("CreateDownloadSlot: %v", err)
	}
	if downloadSlot.RemotePath != slot.RemotePath {
		t.Fatalf("expected remote path to match upload slot")
	}
	if artifact.ID != slot.ID {
		t.Fatalf("expected artifact to reference original slot")
	}
}

func TestCommitPublishesAndStoresMetadata(t *testing.T) {
	now := time.Date(2025, 10, 26, 11, 0, 0, 0, time.UTC)
	tempDir := t.TempDir()
	store := &recordingArtifactStore{}
	publisher := &recordingPublisher{}
	mgr := transfers.NewManager(transfers.Options{
		BaseDir:   tempDir,
		Now:       func() time.Time { return now },
		Store:     store,
		Publisher: publisher,
	})
	slot, err := mgr.CreateUploadSlot(transfers.KindRepo, "job-store", "plan", "node-b", 0)
	if err != nil {
		t.Fatalf("CreateUploadSlot: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(slot.RemotePath), 0o755); err != nil {
		t.Fatalf("create slot dir: %v", err)
	}
	if err := os.WriteFile(slot.RemotePath, []byte("payload-bytes"), 0o644); err != nil {
		t.Fatalf("write payload: %v", err)
	}
	if _, err := mgr.Commit(context.Background(), slot.ID, 0, ""); err != nil {
		t.Fatalf("Commit: %v", err)
	}
	if store.last.ID != slot.ID {
		t.Fatalf("expected store to record slot id, got %q", store.last.ID)
	}
	if store.last.JobID != "job-store" || store.last.Stage != "plan" {
		t.Fatalf("unexpected metadata stored: %#v", store.last)
	}
	if store.last.CID == "" {
		t.Fatalf("expected CID recorded")
	}
	if len(publisher.payloads) != 1 {
		t.Fatalf("expected publisher invocation")
	}
}

type recordingArtifactStore struct {
	last controlplaneartifacts.Metadata
}

func (r *recordingArtifactStore) Create(ctx context.Context, meta controlplaneartifacts.Metadata) (controlplaneartifacts.Metadata, error) {
	meta.CreatedAt = time.Now()
	meta.UpdatedAt = meta.CreatedAt
	r.last = meta
	return meta, nil
}

type recordingPublisher struct {
	payloads [][]byte
}

func (p *recordingPublisher) Add(ctx context.Context, req workflowartifacts.AddRequest) (workflowartifacts.AddResponse, error) {
	p.payloads = append(p.payloads, append([]byte(nil), req.Payload...))
	return workflowartifacts.AddResponse{
		CID:    "bafy-recorded",
		Digest: "sha256:recorded",
		Size:   int64(len(req.Payload)),
		Name:   req.Name,
	}, nil
}
