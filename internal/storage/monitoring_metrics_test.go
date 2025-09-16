package storage

import (
	"encoding/json"
	"testing"
	"time"
)

func TestStorageMetrics_RecordAndSuccessRate(t *testing.T) {
	m := NewStorageMetrics()

	// Upload success
	m.RecordUpload(true, 10*time.Millisecond, 100, "")
	// Download failure
	m.RecordDownload(false, 0, 0, ErrorTypeNetwork)
	// Verification success
	m.RecordVerification(true, "")

	if m.TotalUploads != 1 || m.SuccessfulUploads != 1 || m.TotalDownloads != 1 || m.FailedDownloads != 1 || m.TotalVerifications != 1 || m.SuccessfulVerifications != 1 {
		t.Fatalf("unexpected counters: %+v", *m)
	}
	// Success rate: (1 upload + 1 verification) / (1+1+1) * 100 = 66.66...
	sr := m.GetSuccessRate()
	if sr < 66.0 || sr > 67.0 {
		t.Fatalf("expected success rate ~66.7, got %v", sr)
	}

    // Force degraded and unhealthy states and verify transitions
    // Create several failures to cross degraded/unhealthy thresholds
    for i := 0; i < 4; i++ { // 4 failures total to push degraded
        m.RecordDownload(false, 0, 0, ErrorTypeInternal)
    }
    if m.HealthStatus != HealthStatusDegraded && m.HealthStatus != HealthStatusUnhealthy {
        t.Fatalf("expected degraded or unhealthy after failures, got %s", m.HealthStatus)
    }
    for i := 0; i < 10; i++ { // increase consecutive failures
        m.RecordVerification(false, ErrorTypeInternal)
    }
    if m.HealthStatus != HealthStatusUnhealthy {
        t.Fatalf("expected unhealthy after many failures, got %s", m.HealthStatus)
    }

    // Snapshot JSON contains expected keys
	b, err := m.ToJSON()
	if err != nil {
		t.Fatalf("ToJSON error: %v", err)
	}
	var snap StorageMetrics
	if err := json.Unmarshal(b, &snap); err != nil {
		t.Fatalf("unmarshal snapshot: %v", err)
	}
	if snap.TotalUploads != 1 || snap.SuccessfulVerifications != 1 {
		t.Fatalf("unexpected snapshot: %+v", snap)
	}
}
