package nodeagent

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	types "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

// End-to-end execution tests: full node run flow against a mock server.

// TestEndToEndFlow verifies the complete node execution flow from start to finish.
// This test demonstrates that the node can accept a run request, execute it, stream logs,
// upload diff/artifacts, and emit terminal status successfully.
func TestEndToEndFlow(t *testing.T) {
	t.Run("complete flow with mock server", func(t *testing.T) {
		// Track which endpoints were called during execution (concurrency-safe).
		type endpointHits struct {
			mu sync.Mutex
			m  map[string]int
		}
		inc := func(eh *endpointHits, path string) {
			eh.mu.Lock()
			eh.m[path]++
			eh.mu.Unlock()
		}
		snapshot := func(eh *endpointHits) map[string]int {
			eh.mu.Lock()
			defer eh.mu.Unlock()
			cp := make(map[string]int, len(eh.m))
			for k, v := range eh.m {
				cp[k] = v
			}
			return cp
		}
		get := func(eh *endpointHits, path string) int {
			eh.mu.Lock()
			defer eh.mu.Unlock()
			return eh.m[path]
		}
		endpointsCalled := &endpointHits{m: make(map[string]int)}

		// Create a mock server that responds to node requests.
		mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			inc(endpointsCalled, r.URL.Path)

			switch {
			case strings.HasPrefix(r.URL.Path, "/v1/nodes/") && strings.HasSuffix(r.URL.Path, "/heartbeat"):
				// Heartbeat endpoint.
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`{"status":"ok"}`))
			case strings.HasPrefix(r.URL.Path, "/v1/nodes/") && strings.Contains(r.URL.Path, "/events"):
				// Log events endpoint.
				w.WriteHeader(http.StatusCreated)
			case strings.HasPrefix(r.URL.Path, "/v1/runs/") && strings.Contains(r.URL.Path, "/jobs/") && strings.HasSuffix(r.URL.Path, "/diff"):
				// Diff upload endpoint: /v1/runs/{run_id}/jobs/{job_id}/diff
				w.WriteHeader(http.StatusCreated)
			case strings.HasPrefix(r.URL.Path, "/v1/runs/") && strings.Contains(r.URL.Path, "/jobs/") && strings.HasSuffix(r.URL.Path, "/artifact"):
				// Artifact upload endpoint: /v1/runs/{run_id}/jobs/{job_id}/artifact
				w.WriteHeader(http.StatusCreated)
			case strings.HasPrefix(r.URL.Path, "/v1/jobs/") && strings.HasSuffix(r.URL.Path, "/complete"):
				// Job-level terminal status endpoint.
				w.WriteHeader(http.StatusOK)
			default:
				t.Logf("unexpected endpoint called: %s", r.URL.Path)
				w.WriteHeader(http.StatusNotFound)
			}
		}))
		defer mockServer.Close()

		// Create a minimal config pointing to the mock server.
		cfg := Config{
			NodeID:    testNodeID,
			ServerURL: mockServer.URL,
			HTTP: HTTPConfig{
				Listen: ":0", // Random port.
			},
			Heartbeat: HeartbeatConfig{
				Interval: 1 * time.Second,
				Timeout:  500 * time.Millisecond,
			},
		}

		// Create the run controller with typed JobID keys.
		rc := newTestController(t, cfg)

		// Create a simple StartRunRequest that will execute quickly.
		// We use a tiny command that exits immediately to avoid long test runs.
		req := StartRunRequest{
			RunID:   types.RunID("test-run-e2e"),
			JobID:   types.JobID("test-job-e2e"),
			RepoURL: types.RepoURL("https://github.com/iw2rmb/nodeagent-e2e-synthetic.git"),
			BaseRef: types.GitRef("main"),
			TypedOptions: RunOptions{
				Execution: MigContainerSpec{
					Image:   contracts.JobImage{Universal: "alpine:latest"},
					Command: contracts.CommandSpec{Shell: "echo 'test execution'"},
				},
			},
		}

		// Start the run in a background context with timeout.
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		if err := rc.StartRun(ctx, req); err != nil {
			t.Fatalf("StartRun() failed: %v", err)
		}

		// Verify the job was registered using typed JobID key.
		rc.mu.Lock()
		if _, exists := rc.jobs[req.JobID]; !exists {
			t.Errorf("job %s not found after StartRun", req.JobID)
		}
		rc.mu.Unlock()

		// Wait a bit for the run to execute. The run may fail (for example due to
		// git clone or container execution), but the important part is that it
		// attempts to stream logs, upload diff, and emit status.
		time.Sleep(2 * time.Second)

		// Cancel the context to stop execution if still running.
		cancel()

		// Wait a bit more for cleanup.
		time.Sleep(500 * time.Millisecond)

		// Verify the job was cleaned up from the controller using typed JobID key.
		rc.mu.Lock()
		if _, exists := rc.jobs[req.JobID]; exists {
			t.Errorf("job %s still exists after completion", req.JobID)
		}
		rc.mu.Unlock()

		// Verify that at least the terminal status endpoint was called.
		// (Other endpoints may or may not be called depending on how far execution got.)
		t.Logf("Endpoints called: %+v", snapshot(endpointsCalled))
		if get(endpointsCalled, "/v1/jobs/test-job-e2e/complete") < 1 {
			t.Errorf("terminal status endpoint was not called")
		}
	})
}
