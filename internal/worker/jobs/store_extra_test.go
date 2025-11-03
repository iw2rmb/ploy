package jobs_test

import (
	"testing"
	"time"

	"github.com/iw2rmb/ploy/internal/worker/jobs"
)

// TestStore_Start_ExistingAndEmptyCoversEdges covers Start with empty id and
// starting an already-known id to exercise bump/refresh paths.
func TestStore_Start_ExistingAndEmptyCoversEdges(t *testing.T) {
	var s *jobs.Store
	// Nil receiver and empty id are safe no-ops.
	s.Start("")

	s = jobs.NewStore(jobs.Options{Capacity: 3})
	s.Start("a")
	// Start same id should keep it at front (newest-first) and remain running.
	time.Sleep(1 * time.Millisecond)
	s.Start("b")
	time.Sleep(1 * time.Millisecond)
	s.Start("a") // bump existing to front
	list := s.List()
	if len(list) != 2 {
		t.Fatalf("List length=%d want 2", len(list))
	}
	if list[0].ID != "a" || list[1].ID != "b" {
		t.Fatalf("order=%v want [a b]", []string{list[0].ID, list[1].ID})
	}
	if list[0].State != jobs.StateRunning {
		t.Fatalf("state=%s want running", list[0].State)
	}
}

// TestStore_Complete_UnknownAndInvalidState ensures Complete creates missing
// records and coerces invalid terminal states to failed.
func TestStore_Complete_UnknownAndInvalidState(t *testing.T) {
	s := jobs.NewStore(jobs.Options{Capacity: 2})
	// Complete unknown id with invalid state -> record created and failed.
	s.Complete("z", "not-a-state", "boom")
	got, ok := s.Get("z")
	if !ok {
		t.Fatalf("expected record created for unknown id")
	}
	if got.State != jobs.StateFailed {
		t.Fatalf("state=%s want failed", got.State)
	}
	if got.CompletedAt.IsZero() {
		t.Fatalf("expected CompletedAt to be set")
	}
	if got.Error != "boom" {
		t.Fatalf("error msg=%q want boom", got.Error)
	}
}

// TestStore_List_Dedup ensures internal ordering stays unique if duplicates
// slip in (covers bumpToFrontLocked de-dup logic).
func TestStore_List_Dedup(t *testing.T) {
	s := jobs.NewStore(jobs.Options{Capacity: 5})
	for i := 0; i < 3; i++ {
		s.Start("x")
	}
	// Force some variety and another id.
	s.Start("y")
	records := s.List()
	seen := map[string]bool{}
	for _, r := range records {
		if seen[r.ID] {
			t.Fatalf("duplicate id %q in list %v", r.ID, records)
		}
		seen[r.ID] = true
	}
}
