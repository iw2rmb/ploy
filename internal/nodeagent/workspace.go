package nodeagent

import (
	"os"
	"path/filepath"
)

// createWorkspaceDir creates a temporary workspace directory for a single run.
//
// It prefers the base directory defined by the PLOYD_CACHE_HOME environment
// variable when set, falling back to the system temporary directory otherwise.
// The directory uses a unique 'ploy-run-*' prefix and is suitable for cleanup
// via os.RemoveAll after the run completes.
func createWorkspaceDir() (string, error) {
	base := os.Getenv("PLOYD_CACHE_HOME")
	if base == "" {
		base = os.TempDir()
	}

	// Ensure base exists; ignore errors only on success path.
	if err := os.MkdirAll(base, 0o755); err != nil {
		return "", err
	}

	// Resolve to an absolute path to avoid surprises in tests and logs.
	absBase, err := filepath.Abs(base)
	if err == nil {
		base = absBase
	}

	return os.MkdirTemp(base, "ploy-run-*")
}
