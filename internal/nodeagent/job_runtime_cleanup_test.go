package nodeagent

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	types "github.com/iw2rmb/ploy/internal/domain/types"
)

func TestSweepAbandonedRuntimeRemovesOnlyTerminalJobs(t *testing.T) {
	cacheHome := t.TempDir()
	t.Setenv("PLOYD_CACHE_HOME", cacheHome)

	runID := types.NewRunID()
	terminalID := types.NewJobID()
	runningID := types.NewJobID()
	terminalDirs := createJobWithRuntimeFixture(t, runID, terminalID)
	runningDirs := createJobWithRuntimeFixture(t, runID, runningID)

	statuses := map[string]string{
		terminalID.String(): types.JobStatusSuccess.String(),
		runningID.String():  types.JobStatusRunning.String(),
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		jobID := filepath.Base(filepath.Dir(r.URL.Path))
		status, ok := statuses[jobID]
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"job_id":"` + jobID + `","status":"` + status + `"}`))
	}))
	defer server.Close()

	controller := newTestController(t, newAgentConfig(server.URL))
	controller.sweepAbandonedRuntimeIfIdle()

	for _, dir := range []string{terminalDirs.Cache, terminalDirs.Home, terminalDirs.Staging, terminalDirs.Tmp} {
		if _, err := os.Stat(dir); !os.IsNotExist(err) {
			t.Errorf("terminal runtime directory remains: %s", dir)
		}
	}
	for _, path := range []string{terminalDirs.In, terminalDirs.Out, terminalDirs.Stdout} {
		if _, err := os.Stat(path); err != nil {
			t.Errorf("terminal durable artifact was removed: %s: %v", path, err)
		}
	}
	for _, dir := range []string{runningDirs.Cache, runningDirs.Home, runningDirs.Staging, runningDirs.Tmp} {
		if _, err := os.Stat(dir); err != nil {
			t.Errorf("running job runtime directory was removed: %s: %v", dir, err)
		}
	}
}

func TestSweepAbandonedRuntimeWaitsForIdleNode(t *testing.T) {
	cacheHome := t.TempDir()
	t.Setenv("PLOYD_CACHE_HOME", cacheHome)

	runID := types.NewRunID()
	jobID := types.NewJobID()
	dirs := createJobWithRuntimeFixture(t, runID, jobID)
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"job_id":"` + jobID.String() + `","status":"Success"}`))
	}))
	defer server.Close()

	controller := newTestController(t, newAgentConfig(server.URL))
	controller.jobs[types.NewJobID()] = &jobContext{}
	controller.sweepAbandonedRuntimeIfIdle()

	if requests != 0 {
		t.Fatalf("status requests = %d, want 0 while node is active", requests)
	}
	if _, err := os.Stat(dirs.Cache); err != nil {
		t.Fatalf("active-node sweep removed runtime directory: %v", err)
	}
}

func createJobWithRuntimeFixture(t *testing.T, runID types.RunID, jobID types.JobID) JobDirectories {
	t.Helper()
	dirs := jobDirectories(runID, jobID)
	if err := ensureJobDirectories(dirs); err != nil {
		t.Fatalf("ensureJobDirectories() error = %v", err)
	}
	for _, dir := range []string{dirs.Cache, dirs.Home, dirs.Staging, dirs.Tmp} {
		if err := os.WriteFile(filepath.Join(dir, "runtime.bin"), []byte("runtime"), 0o600); err != nil {
			t.Fatalf("write runtime fixture: %v", err)
		}
	}
	if err := os.WriteFile(filepath.Join(dirs.In, "input.txt"), []byte("input"), 0o600); err != nil {
		t.Fatalf("write input fixture: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dirs.Out, "output.txt"), []byte("output"), 0o600); err != nil {
		t.Fatalf("write output fixture: %v", err)
	}
	if err := os.WriteFile(dirs.Stdout, []byte("log"), 0o600); err != nil {
		t.Fatalf("write log fixture: %v", err)
	}
	return dirs
}
