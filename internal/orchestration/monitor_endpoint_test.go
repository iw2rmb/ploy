package orchestration

import "testing"

func TestHealthMonitor_GetJobEndpoint_Passthrough(t *testing.T) {
	adapter := &fakeNomadAdapter{allocs: []AllocationStatus{{ID: "alloc-1", ClientStatus: "running"}}, endpoint: "http://127.0.0.1:12345"}
	hm := NewHealthMonitorWithClient(adapter)
	ep, err := hm.GetJobEndpoint("jobA")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ep != "http://127.0.0.1:12345" {
		t.Fatalf("expected endpoint passthrough, got %s", ep)
	}
}
