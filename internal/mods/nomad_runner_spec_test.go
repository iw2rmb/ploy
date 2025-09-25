package mods

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestModsIntegrationNomadJobSpec ensures the Nomad job specification for Mods
// integration testing exists and exposes the expected command and environment hooks.
func TestModsIntegrationNomadJobSpec(t *testing.T) {
	t.Parallel()

	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("unable to determine caller information")
	}
	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", ".."))
	jobPath := filepath.Join(repoRoot, "tests", "nomad-jobs", "mods-integration.nomad.hcl")

	info, err := os.Stat(jobPath)
	if err != nil {
		t.Fatalf("mods integration Nomad job missing at %s: %v", jobPath, err)
	}
	if info.IsDir() {
		t.Fatalf("expected Nomad job file at %s, found directory", jobPath)
	}

	contents, err := os.ReadFile(jobPath)
	require.NoError(t, err, "failed reading mods integration Nomad job spec")
	if len(contents) == 0 {
		t.Fatalf("mods integration Nomad job spec %s is empty", jobPath)
	}

	spec := string(contents)
	if !strings.Contains(spec, `job "mods-integration-tests"`) && !strings.Contains(spec, `job "${MODS_INTEGRATION_JOB_NAME}"`) {
		t.Fatalf("job spec should declare mods integration job name")
	}
	assert.Contains(t, spec, `driver = "docker"`, "job should run inside Docker to access builder images")
	assert.Contains(t, spec, `go test ./internal/mods -tags=integration`, "job must execute the integration test suite")
	assert.Contains(t, spec, `PLOY_CONTROLLER`, "job should expose controller endpoint via env var")
	assert.Contains(t, spec, `PLOY_SEAWEEDFS_URL`, "job should surface SeaweedFS endpoint")
	assert.Contains(t, spec, `MODS_SEAWEED_MASTER`, "job should allow overriding Seaweed master host")
	assert.Contains(t, spec, `GITHUB_PLOY_DEV_USERNAME`, "job should accept GitHub credentials for repo checkout")
	assert.Contains(t, spec, `GITHUB_PLOY_DEV_PAT`, "job should accept GitHub PAT for repo checkout")
	assert.Contains(t, spec, `git clone`, "job should fetch repository contents before testing")
}
