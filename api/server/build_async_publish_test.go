package server

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestWriteStatusPublishesBuildEvent(t *testing.T) {
	dir := t.TempDir()
	oldDir := uploadsBaseDir
	uploadsBaseDir = dir
	t.Cleanup(func() { uploadsBaseDir = oldDir })

	var (
		mu      sync.Mutex
		records []buildStatus
	)
	SetBuildStatusPublisher(func(ctx context.Context, st buildStatus) {
		if ctx == nil {
			t.Fatalf("expected context, got nil")
		}
		mu.Lock()
		defer mu.Unlock()
		records = append(records, st)
	})
	t.Cleanup(func() { SetBuildStatusPublisher(nil) })

	status := buildStatus{ID: "b-1", App: "demo", Status: "running"}
	writeStatus(status.ID, status)

	// Ensure status file persisted for existing behaviour
	if _, err := os.Stat(filepath.Join(dir, status.ID+".json")); err != nil {
		t.Fatalf("expected status file written: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(records) != 1 {
		t.Fatalf("expected 1 published status, got %d", len(records))
	}
	if records[0].Status != status.Status || records[0].ID != status.ID {
		t.Fatalf("unexpected status published: %+v", records[0])
	}
}
