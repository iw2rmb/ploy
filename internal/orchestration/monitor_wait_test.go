package orchestration

import (
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
