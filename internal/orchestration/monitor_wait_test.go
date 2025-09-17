package orchestration

import (
	"fmt"
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
