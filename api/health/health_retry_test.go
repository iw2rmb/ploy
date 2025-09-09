package health

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// Test that checkNomad tolerates transient 429 and eventually reports healthy
func TestCheckNomad_RetriesOn429(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/status/leader" {
			calls++
			if calls <= 2 {
				w.Header().Set("Retry-After", "1")
				w.WriteHeader(http.StatusTooManyRequests)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("\"127.0.0.1:4647\""))
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	hc := NewHealthChecker("", "", srv.URL)
	dep := hc.checkNomad()
	if dep.Status != "healthy" {
		t.Fatalf("expected healthy after retries, got: %+v", dep)
	}
	if calls < 3 {
		t.Fatalf("expected retries, calls=%d", calls)
	}
}
