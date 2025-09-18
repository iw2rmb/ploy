package build

import "testing"

func TestLaneResourceForDocker(t *testing.T) {
	if got := getInstanceCountForLane("D"); got != 2 {
		t.Fatalf("instance count: got %d want 2", got)
	}
	if got := getCpuLimitForLane("D"); got != 600 {
		t.Fatalf("cpu limit: got %d want 600", got)
	}
	if got := getMemoryLimitForLane("D"); got != 512 {
		t.Fatalf("memory limit: got %d want 512", got)
	}
	if got := getJvmMemoryForLane("D"); got != 0 {
		t.Fatalf("jvm memory: got %d want 0", got)
	}
}

func TestLaneResourceDefaultFallback(t *testing.T) {
	if got := getInstanceCountForLane("unknown"); got != 2 {
		t.Fatalf("instance count default: got %d want 2", got)
	}
	if got := getCpuLimitForLane("unknown"); got != 600 {
		t.Fatalf("cpu limit default: got %d want 600", got)
	}
	if got := getMemoryLimitForLane("unknown"); got != 512 {
		t.Fatalf("memory limit default: got %d want 512", got)
	}
	if got := getJvmMemoryForLane("unknown"); got != 0 {
		t.Fatalf("jvm memory default: got %d want 0", got)
	}
}
