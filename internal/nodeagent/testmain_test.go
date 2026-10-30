package nodeagent

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestMain(m *testing.M) {
	// Ensure bearer token path resolves to a temp file for tests.
	tmpDir, _ := os.MkdirTemp("", "nodeagent-*")
	lockPath := filepath.Join(tmpDir, "maintenance.lock")
	_ = os.WriteFile(lockPath, nil, 0600)
	_ = os.Setenv("PLOY_NODE_MAINTENANCE_LOCK_FILE", lockPath)
	tokenFile := filepath.Join(tmpDir, "bearer-token")
	_ = os.WriteFile(tokenFile, []byte("test-token"), 0600)
	_ = os.Setenv("PLOY_NODE_BEARER_TOKEN_PATH", tokenFile)
	_ = os.Setenv(nodeCacheRootEnv, filepath.Join(tmpDir, "node-cache"))
	_ = os.Setenv(nodeJobConfigRootEnv, filepath.Join(tmpDir, "job-config"))

	code := m.Run()

	_ = os.RemoveAll(tmpDir)
	os.Exit(code)
}

func testRunSlot(t *testing.T, r *runController) *RunSlot {
	t.Helper()
	slot, err := r.AcquireSlot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return slot
}
