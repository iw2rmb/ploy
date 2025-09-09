package orchestration

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

// Test that newSDKNomadAdapter uses a client with RetryTransport by verifying
// it retries on 429 responses and eventually succeeds.
func TestSDKNomadAdapter_UsesRetryTransport(t *testing.T) {
	calls := 0
	// Minimal handler for the allocations endpoint
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Simulate 2 transient 429s, then success
		calls++
		if calls <= 2 {
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		if r.URL.Path == "/v1/job/test-job/allocations" {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode([]map[string]interface{}{
				{"ID": "alloc-1", "ClientStatus": "running", "DesiredStatus": "run"},
			})
			return
		}
		// Default 200
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	// Point Nomad client to our test server
	os.Setenv("NOMAD_ADDR", srv.URL)
	defer os.Unsetenv("NOMAD_ADDR")

	adapter := newSDKNomadAdapter()
	if adapter == nil || adapter.client == nil {
		t.Fatalf("expected adapter and client")
	}
	allocs, err := adapter.ListAllocations("test-job")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(allocs) != 1 || allocs[0].ClientStatus != "running" {
		t.Fatalf("unexpected allocations: %+v", allocs)
	}
	if calls < 3 {
		t.Fatalf("expected retries to occur, calls=%d", calls)
	}
}
