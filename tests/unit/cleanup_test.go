package unit

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPhase5CleanupCompleted(t *testing.T) {
	// This test documents that Phase 5 cleanup has been completed
	// and verifies that obsolete tools have been properly removed
	
	tests := []struct {
		name        string
		path        string
		description string
	}{
		{
			name:        "api-dist tool removed",
			path:        "tools/api-dist",
			description: "Legacy binary upload tool replaced by unified deployment",
		},
		{
			name:        "deploy.sh script removed",
			path:        "scripts/deploy.sh",
			description: "Legacy deployment script replaced by ployman push",
		},
	}

	// Get repository root
	repoRoot := findRepoRoot(t)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fullPath := filepath.Join(repoRoot, tt.path)
			
			if _, err := os.Stat(fullPath); !os.IsNotExist(err) {
				t.Errorf("Obsolete tool %s should be removed: %s", tt.path, tt.description)
			} else {
				t.Logf("✓ Phase 5 cleanup verified: %s - %s", tt.path, tt.description)
			}
		})
	}
}

func TestObsoleteToolsRemovedAfterCleanup(t *testing.T) {
	// This test will pass after cleanup is complete
	tests := []struct {
		name string
		path string
	}{
		{
			name: "api-dist directory removed",
			path: "tools/api-dist",
		},
		{
			name: "deploy.sh script removed", 
			path: "scripts/deploy.sh",
		},
	}

	// Get repository root
	repoRoot := findRepoRoot(t)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fullPath := filepath.Join(repoRoot, tt.path)
			
			if _, err := os.Stat(fullPath); !os.IsNotExist(err) {
				t.Errorf("Obsolete tool %s should be removed but still exists", tt.path)
			} else {
				t.Logf("✓ Confirmed removal: %s", tt.path)
			}
		})
	}
}

// findRepoRoot walks up the directory tree to find the repository root
func findRepoRoot(t *testing.T) string {
	t.Helper()
	
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get working directory: %v", err)
	}
	
	// Walk up until we find .git or reach filesystem root
	for {
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			return dir
		}
		
		parent := filepath.Dir(dir)
		if parent == dir {
			// Reached filesystem root
			break
		}
		dir = parent
	}
	
	t.Fatalf("Could not find repository root (no .git directory found)")
	return ""
}