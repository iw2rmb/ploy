package mods

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestModsFixtureScriptsAndHarness verifies fixture seeding and VPS runner scripts.
func TestModsFixtureScriptsAndHarness(t *testing.T) {
	t.Parallel()

	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("unable to determine caller information")
	}
	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", ".."))

	seedPath := filepath.Join(repoRoot, "scripts", "mods-seed-fixtures.sh")
	seedInfo, err := os.Stat(seedPath)
	if err != nil {
		t.Fatalf("mods fixture seed script missing at %s: %v", seedPath, err)
	}
	if seedInfo.IsDir() {
		t.Fatalf("expected script file at %s, found directory", seedPath)
	}

	seedBody, err := os.ReadFile(seedPath)
	require.NoError(t, err, "failed reading mods fixture seed script")
	seed := string(seedBody)
	assert.Contains(t, seed, "PLOY_SEAWEEDFS_URL", "seed script should honor SeaweedFS endpoint overrides")
	assert.Contains(t, seed, "PLOY_GITLAB_PAT", "seed script should rely on PLOY_GITLAB_PAT for GitLab access")
	assert.Contains(t, seed, "artifacts/mods", "seed script should upload fixtures into the mods namespace")

	runnerPath := filepath.Join(repoRoot, "scripts", "run-mods-integration-vps.sh")
	runnerInfo, err := os.Stat(runnerPath)
	if err != nil {
		t.Fatalf("mods integration runner script missing at %s: %v", runnerPath, err)
	}
	if runnerInfo.IsDir() {
		t.Fatalf("expected runner script file at %s, found directory", runnerPath)
	}

	runnerBody, err := os.ReadFile(runnerPath)
	require.NoError(t, err, "failed reading mods integration runner script")
	runner := string(runnerBody)
	assert.Contains(t, runner, "ssh -o ConnectTimeout=10 \"root@${TARGET_HOST}\"", "runner should execute on the VPS via SSH")
	assert.Contains(t, runner, "git fetch", "runner should fetch the target commit")
	assert.Contains(t, runner, "NATS_ADDR=", "runner should propagate NATS connectivity")
	assert.Contains(t, runner, "go test -tags=integration -run Integration", "runner should invoke the mods integration suite")
}
