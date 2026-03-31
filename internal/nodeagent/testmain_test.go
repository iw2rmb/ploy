package nodeagent

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMain(m *testing.M) {
	// Ensure bearer token path resolves to a temp file for tests.
	tmpDir, _ := os.MkdirTemp("", "nodeagent-*")
	tokenFile := filepath.Join(tmpDir, "bearer-token")
	_ = os.WriteFile(tokenFile, []byte("test-token"), 0600)
	_ = os.Setenv("PLOY_NODE_BEARER_TOKEN_PATH", tokenFile)

	code := m.Run()

	_ = os.RemoveAll(tmpDir)
	os.Exit(code)
}
