package mods

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestModsFixtureScriptsAndCI ensures fixture seeding automation and CI wiring exist.
func TestModsFixtureScriptsAndCI(t *testing.T) {
	t.Parallel()

	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("unable to determine caller information")
	}
	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", ".."))

	scriptPath := filepath.Join(repoRoot, "scripts", "mods-seed-fixtures.sh")
	info, err := os.Stat(scriptPath)
	if err != nil {
		t.Fatalf("mods fixture seed script missing at %s: %v", scriptPath, err)
	}
	if info.IsDir() {
		t.Fatalf("expected script file at %s, found directory", scriptPath)
	}

	scriptBody, err := os.ReadFile(scriptPath)
	require.NoError(t, err, "failed reading mods fixture seed script")
	script := string(scriptBody)
	assert.Contains(t, script, "PLOY_SEAWEEDFS_URL", "script should honor SeaweedFS endpoint overrides")
	assert.Contains(t, script, "PLOY_GITLAB_PAT", "script should use PLOY_GITLAB_PAT for GitLab fixtures")
	assert.Contains(t, script, "artifacts/mods", "script should upload mods artifacts into SeaweedFS")
	assert.Contains(t, script, "GITHUB_PLOY_DEV_USERNAME", "script should support authenticated Git operations for fixtures")

	workflowPath := filepath.Join(repoRoot, ".github", "workflows", "ci.yml")
	workflowBody, err := os.ReadFile(workflowPath)
	require.NoError(t, err, "failed reading CI workflow")
	workflow := string(workflowBody)
	assert.Contains(t, workflow, "mods-integration-harness", "CI should define mods integration harness job")
	assert.Contains(t, workflow, "make mods-integration-vps", "CI job should invoke mods integration harness")
	assert.Contains(t, workflow, "PLOY_GITLAB_PAT", "CI job should propagate PLOY_GITLAB_PAT to mods harness")
}
