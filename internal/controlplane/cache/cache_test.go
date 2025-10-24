package cache_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	"github.com/iw2rmb/ploy/internal/controlplane/cache"
)

func TestCacheRememberNodesPersists(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "controlplane.json")

	store, err := cache.New(path)
	if err != nil {
		t.Fatalf("new cache: %v", err)
	}

	nodes := []cache.Node{
		{ID: "node-a", Address: "10.0.0.10", SSHPort: 22, APIPort: 8443},
		{ID: "node-b", Address: "10.0.0.11", SSHPort: 2200, APIPort: 9443},
	}

	if err := store.RememberNodes(nodes); err != nil {
		t.Fatalf("remember nodes: %v", err)
	}

	stored := store.Nodes()
	if diff := cmp.Diff(nodes, stored, cmpopts.SortSlices(func(a, b cache.Node) bool { return a.ID < b.ID })); diff != "" {
		t.Fatalf("nodes mismatch (-want +got):\n%s", diff)
	}

	// Reload from disk to ensure persistence.
	reloaded, err := cache.New(path)
	if err != nil {
		t.Fatalf("reload cache: %v", err)
	}
	reloadedNodes := reloaded.Nodes()
	if diff := cmp.Diff(nodes, reloadedNodes, cmpopts.SortSlices(func(a, b cache.Node) bool { return a.ID < b.ID })); diff != "" {
		t.Fatalf("reloaded nodes mismatch (-want +got):\n%s", diff)
	}
}

func TestCacheRememberJobPersists(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "controlplane.json")

	store, err := cache.New(path)
	if err != nil {
		t.Fatalf("new cache: %v", err)
	}

	nodes := []cache.Node{
		{ID: "node-a", Address: "10.0.0.10", SSHPort: 22, APIPort: 8443},
		{ID: "node-b", Address: "10.0.0.11", SSHPort: 22, APIPort: 8443},
	}
	if err := store.RememberNodes(nodes); err != nil {
		t.Fatalf("remember nodes: %v", err)
	}

	now := time.Now()
	if err := store.RememberJob("job-1", "node-b", now); err != nil {
		t.Fatalf("remember job: %v", err)
	}

	nodeID, ok := store.LookupJob("job-1")
	if !ok {
		t.Fatalf("job not found in cache")
	}
	if nodeID != "node-b" {
		t.Fatalf("expected node-b, got %q", nodeID)
	}

	// Ensure persistence when reloading.
	reloaded, err := cache.New(path)
	if err != nil {
		t.Fatalf("reload cache: %v", err)
	}
	nodeID, ok = reloaded.LookupJob("job-1")
	if !ok {
		t.Fatalf("job not found after reload")
	}
	if nodeID != "node-b" {
		t.Fatalf("expected node-b after reload, got %q", nodeID)
	}

	// Overwrite job assignment and ensure it updates.
	if err := reloaded.RememberJob("job-1", "node-a", now.Add(time.Minute)); err != nil {
		t.Fatalf("update job: %v", err)
	}
	nodeID, ok = reloaded.LookupJob("job-1")
	if !ok {
		t.Fatalf("job not found after update")
	}
	if nodeID != "node-a" {
		t.Fatalf("expected node-a after update, got %q", nodeID)
	}

	// Remove job to keep cache in sync.
	if err := reloaded.RemoveJob("job-1"); err != nil {
		t.Fatalf("remove job: %v", err)
	}
	if _, ok := reloaded.LookupJob("job-1"); ok {
		t.Fatalf("job still present after removal")
	}
}

func TestCacheNewCreatesParentDir(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nested", "controlplane.json")

	if _, err := cache.New(path); err != nil {
		t.Fatalf("new cache: %v", err)
	}
	if _, err := os.Stat(filepath.Dir(path)); err != nil {
		t.Fatalf("expected parent directory to exist: %v", err)
	}
}
