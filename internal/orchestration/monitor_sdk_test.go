package orchestration

import (
	"testing"
)

// fake adapter for tests
type fakeNomadAdapter struct {
	allocs   []AllocationStatus
	endpoint string
}

func (f *fakeNomadAdapter) ListAllocations(jobID string) ([]*AllocationStatus, error) {
	out := make([]*AllocationStatus, 0, len(f.allocs))
	for i := range f.allocs {
		a := f.allocs[i]
		out = append(out, &a)
	}
	return out, nil
}
func (f *fakeNomadAdapter) AllocationEndpoint(allocID string) (string, error) { return f.endpoint, nil }

// RED: until HealthMonitor supports injection and SDK adapter, these tests will fail to build or run
func TestHealthMonitor_IsJobHealthy_UsesAdapter(t *testing.T) {
	adapter := &fakeNomadAdapter{allocs: []AllocationStatus{{ID: "a1", ClientStatus: "running"}}}
	hm := NewHealthMonitorWithClient(adapter)
	if !hm.IsJobHealthy("job1") {
		t.Fatalf("expected job to be healthy when running allocation present")
	}
}

func TestHealthMonitor_GetJobEndpoint_UsesAdapter(t *testing.T) {
	adapter := &fakeNomadAdapter{allocs: []AllocationStatus{{ID: "a1", ClientStatus: "running"}}, endpoint: "http://10.0.0.1:8080"}
	hm := NewHealthMonitorWithClient(adapter)
	ep, err := hm.GetJobEndpoint("job1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ep != "http://10.0.0.1:8080" {
		t.Fatalf("expected endpoint, got %s", ep)
	}
}
