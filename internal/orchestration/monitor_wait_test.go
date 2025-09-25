package orchestration

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"
)

func TestHealthMonitor_WaitForHealthyAllocations_Succeeds(t *testing.T) {
	adapter := &fakeNomadAdapter{allocs: []AllocationStatus{{ID: "a1", ClientStatus: "running"}}}
	hm := NewHealthMonitorWithClient(adapter)
	if err := hm.WaitForHealthyAllocations("jobX", 1, 50*time.Millisecond); err != nil {
		t.Fatalf("expected success, got %v", err)
	}
}

func TestHealthMonitor_WaitForHealthyAllocations_Timeout(t *testing.T) {
	adapter := &fakeNomadAdapter{allocs: []AllocationStatus{}} // no running allocs
	hm := NewHealthMonitorWithClient(adapter)
	if err := hm.WaitForHealthyAllocations("jobX", 1, 5*time.Millisecond); err == nil {
		t.Fatal("expected timeout error, got nil")
	}
}

func TestHealthMonitor_WaitForHealthyAllocations_ErrorRespectsTimeout(t *testing.T) {
	adapter := &errorOnlyAdapter{}
	hm := NewHealthMonitorWithClient(adapter)

	deadline := 75 * time.Millisecond
	start := time.Now()
	err := hm.WaitForHealthyAllocations("jobX", 1, deadline)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected error when allocations cannot be listed")
	}
	if elapsed > 250*time.Millisecond {
		t.Fatalf("expected timeout respected (~%v), but waited %v", deadline, elapsed)
	}
	if adapter.calls == 0 {
		t.Fatalf("expected adapter to be invoked, calls=%d", adapter.calls)
	}
}

type errorOnlyAdapter struct{ calls int }

func (e *errorOnlyAdapter) ListAllocations(jobID string) ([]*AllocationStatus, error) {
	e.calls++
	return nil, fmt.Errorf("failed to list allocations")
}

func (e *errorOnlyAdapter) AllocationEndpoint(string) (string, error) {
	return "", fmt.Errorf("not implemented")
}

func TestWaitForHealthyAllocationsPublishesReadyEvent(t *testing.T) {
	adapter := &fakeNomadAdapter{allocs: []AllocationStatus{{ID: "alloc-1", ClientStatus: "running"}}}
	var (
		mu       sync.Mutex
		recorded []struct {
			jobID   string
			alloc   *AllocationStatus
			healthy int
		}
	)
	SetReadyNotifier(func(ctx context.Context, jobID string, alloc *AllocationStatus, healthy int) {
		if ctx == nil {
			t.Fatalf("expected context, got nil")
		}
		mu.Lock()
		defer mu.Unlock()
		recorded = append(recorded, struct {
			jobID   string
			alloc   *AllocationStatus
			healthy int
		}{jobID: jobID, alloc: alloc, healthy: healthy})
	})
	t.Cleanup(func() { SetReadyNotifier(nil) })

	hm := NewHealthMonitorWithClient(adapter)

	if err := hm.WaitForHealthyAllocations("job-ready", 1, 50*time.Millisecond); err != nil {
		t.Fatalf("expected success, got %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(recorded) == 0 {
		t.Fatalf("expected readiness event to be published")
	}
	if recorded[0].jobID != "job-ready" {
		t.Fatalf("unexpected job id: %s", recorded[0].jobID)
	}
	if recorded[0].alloc == nil || recorded[0].alloc.ID != "alloc-1" {
		t.Fatalf("unexpected alloc payload: %+v", recorded[0].alloc)
	}
	if recorded[0].healthy != 1 {
		t.Fatalf("expected healthy count 1, got %d", recorded[0].healthy)
	}
}
