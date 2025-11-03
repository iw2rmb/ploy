package jobs_test

import (
	"testing"
	"time"

	"github.com/iw2rmb/ploy/internal/worker/jobs"
)

// TestStore_StartListComplete_Bounds verifies ordering and retention bounds.
func TestStore_StartListComplete_Bounds(t *testing.T) {
	t.Parallel()

	store := jobs.NewStore(jobs.Options{Capacity: 2})

	store.Start("job-1")
	time.Sleep(1 * time.Millisecond)
	store.Start("job-2")
	time.Sleep(1 * time.Millisecond)
	store.Start("job-3")

	list := store.List()
	if len(list) != 2 {
		t.Fatalf("list len=%d want 2", len(list))
	}
	if list[0].ID != "job-3" || list[1].ID != "job-2" {
		t.Fatalf("order=%v want [job-3 job-2]", []string{list[0].ID, list[1].ID})
	}

	store.Complete("job-2", jobs.StateSucceeded, "")
	got, ok := store.Get("job-2")
	if !ok {
		t.Fatalf("expected job-2 present")
	}
	if got.State != jobs.StateSucceeded || got.CompletedAt.IsZero() {
		t.Fatalf("unexpected state=%s completed_at=%v", got.State, got.CompletedAt)
	}
}

// TestStore_Get_Unknown returns false for missing id.
func TestStore_Get_Unknown(t *testing.T) {
	t.Parallel()
	store := jobs.NewStore(jobs.Options{})
	if _, ok := store.Get("missing"); ok {
		t.Fatalf("expected missing=false for unknown id")
	}
}
