package nodeagent

import "testing"

func TestCrashReconcile_StartupRunsBeforeFirstClaim_Contract(t *testing.T) {
	t.Skip("phase 0 contract: enable when startup reconciliation wiring (phase 5) is implemented")
}

func TestCrashReconcile_ClassifiesByRuntimeState_Contract(t *testing.T) {
	t.Skip("phase 0 contract: enable when crash reconciler classification (phase 2) is implemented")
}

func TestCrashReconcile_UsesFinishedAtCutoff_Contract(t *testing.T) {
	t.Skip("phase 0 contract: enable when terminal finished_at cutoff (phase 2) is implemented")
}

func TestCrashReconcile_SkipsStaleTerminalContainers_Contract(t *testing.T) {
	t.Skip("phase 0 contract: enable when startup terminal replay policy (phase 2) is implemented")
}
