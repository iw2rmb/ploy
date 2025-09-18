package health

import (
	"testing"
)

func TestGetHealthAndReadinessStatus(t *testing.T) {
	hc := NewHealthChecker("", "127.0.0.1:8500", "http://127.0.0.1:4646")
	hc.SetDependencyChecksEnabled(false)
	h := hc.GetHealthStatus()
	if h.Dependencies == nil {
		t.Fatalf("expected dependencies map")
	}

	r := hc.GetReadinessStatus()
	if r.Dependencies == nil {
		t.Fatalf("expected readiness dependencies map")
	}
}
