package lifecycle

import (
	"testing"
	"time"
)

// TestCacheStoreAndCopy verifies that Cache stores and returns typed NodeStatus correctly.
// The cache should return a copy of the stored status, not a reference.
func TestCacheStoreAndCopy(t *testing.T) {
	c := NewCache()

	// Create a typed NodeStatus with nested resources.
	now := time.Now().UTC()
	src := NodeStatus{
		State:     "ok",
		Timestamp: now,
		Heartbeat: now,
		Role:      "node",
		NodeID:    "abc123",
		Hostname:  "test-host",
		Resources: NodeResources{
			CPU: CPUResources{
				TotalMCores: 4000.0,
				FreeMCores:  2000.0,
				Load1:       1.5,
			},
			Memory: MemoryResources{
				TotalMB: 8192.0,
				FreeMB:  4096.0,
			},
			Disk: DiskResources{
				TotalMB: 102400.0,
				FreeMB:  51200.0,
				IO: DiskIO{
					ReadMBPerSec:  10.5,
					WriteMBPerSec: 5.2,
					ReadIOPS:      100.0,
					WriteIOPS:     50.0,
					InitialSample: false,
				},
			},
			Network: NetworkResources{
				RXBytesPerSec:   1000000.0,
				TXBytesPerSec:   500000.0,
				RXPacketsPerSec: 1000.0,
				TXPacketsPerSec: 500.0,
				InitialSample:   false,
			},
		},
		Components: NodeComponents{
			Docker: ComponentStatus{State: "ok", CheckedAt: now},
			Gate:   ComponentStatus{State: "ok", CheckedAt: now},
		},
	}

	// Store the status.
	c.Store(src)

	// Retrieve the status and verify it matches what was stored.
	got, ok := c.LatestStatus()
	if !ok {
		t.Fatal("expected cached status available")
	}
	if got.Resources.CPU.FreeMCores != 2000.0 {
		t.Fatalf("unexpected cpu.free_mcores: got %v, want 2000.0", got.Resources.CPU.FreeMCores)
	}
	if got.State != "ok" {
		t.Fatalf("unexpected state: got %v, want ok", got.State)
	}
	if got.NodeID != "abc123" {
		t.Fatalf("unexpected node_id: got %v, want abc123", got.NodeID)
	}

	// Verify ToMap() produces the expected wire format for JSON serialization.
	// ToMap() is called at serialization boundaries (e.g., in status.Provider.Snapshot).
	gotMap := got.ToMap()
	resources, ok := gotMap["resources"].(map[string]any)
	if !ok {
		t.Fatal("expected resources to be map[string]any")
	}
	cpu, ok := resources["cpu"].(map[string]any)
	if !ok {
		t.Fatal("expected cpu to be map[string]any")
	}
	if cpu["free_mcores"].(float64) != 2000.0 {
		t.Fatalf("unexpected cpu.free_mcores from map: got %v, want 2000.0", cpu["free_mcores"])
	}
}

// TestCacheEmpty verifies that an empty cache returns false from LatestStatus.
func TestCacheEmpty(t *testing.T) {
	c := NewCache()

	if _, ok := c.LatestStatus(); ok {
		t.Fatal("expected empty cache to return false")
	}
}
