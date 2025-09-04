package orchestration

import (
    "testing"
    "time"
)

type fakeConsulAdapter struct{ checks []*ServiceHealth }

func (f *fakeConsulAdapter) Checks(service string) ([]*ServiceHealth, error) { return f.checks, nil }

// RED: until ConsulHealth supports injection and SDK adapter, these tests will fail or not compile
func TestConsulHealth_CheckServiceHealth_UsesAdapter(t *testing.T) {
    adapter := &fakeConsulAdapter{checks: []*ServiceHealth{{ServiceName: "svc", Status: "passing"}}}
    ch := NewConsulHealthWithClient(adapter)
    checks, err := ch.CheckServiceHealth("svc")
    if err != nil { t.Fatalf("unexpected error: %v", err) }
    if len(checks) != 1 || checks[0].Status != "passing" {
        t.Fatalf("expected passing check via adapter")
    }
}

func TestConsulHealth_WaitForServiceHealth_UsesAdapter(t *testing.T) {
    adapter := &fakeConsulAdapter{checks: []*ServiceHealth{{ServiceName: "svc", Status: "passing"}}}
    ch := NewConsulHealthWithClient(adapter)
    if err := ch.WaitForServiceHealth("svc", 1*time.Second); err != nil {
        t.Fatalf("expected healthy, got %v", err)
    }
}

