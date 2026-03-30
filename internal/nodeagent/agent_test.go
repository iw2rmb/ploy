package nodeagent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
)

func TestAgentLifecycle(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		serverOpts []agentServerOption
		configOpts []configOption
		startup    time.Duration
		check      func(t *testing.T, agent *Agent)
		postCheck  func(t *testing.T, agent *Agent) // called after shutdown
	}{
		{
			name:    "health check responds during run",
			startup: 200 * time.Millisecond,
			check: func(t *testing.T, agent *Agent) {
				t.Helper()
				resp, err := http.Get(fmt.Sprintf("http://%s/health", agent.server.Address()))
				if err != nil {
					t.Fatalf("health check failed: %v", err)
				}
				defer func() { _ = resp.Body.Close() }()
				if resp.StatusCode != http.StatusOK {
					t.Errorf("health check status=%d, want %d", resp.StatusCode, http.StatusOK)
				}
			},
		},
		{
			name:    "graceful shutdown stops server",
			startup: 150 * time.Millisecond,
			postCheck: func(t *testing.T, agent *Agent) {
				t.Helper()
				resp, err := http.Get(fmt.Sprintf("http://%s/health", agent.server.Address()))
				if err == nil && resp != nil {
					_ = resp.Body.Close()
					t.Error("expected connection error after shutdown, got nil")
				}
			},
		},
		{
			name:    "accepts run start request",
			startup: 200 * time.Millisecond,
			check: func(t *testing.T, agent *Agent) {
				t.Helper()
				startReq := map[string]string{
					"run_id": domaintypes.NewRunID().String(), "job_id": domaintypes.NewJobID().String(),
					"job_type": "mig", "repo_url": "https://github.com/example/repo.git", "base_ref": "main",
				}
				reqBody, _ := json.Marshal(startReq)
				resp, err := http.Post(
					fmt.Sprintf("http://%s/v1/run/start", agent.server.Address()),
					"application/json", io.NopCloser(bytes.NewReader(reqBody)),
				)
				if err != nil {
					t.Fatalf("send run start: %v", err)
				}
				defer func() { _ = resp.Body.Close() }()
				if resp.StatusCode != http.StatusAccepted {
					body, _ := io.ReadAll(resp.Body)
					t.Errorf("start run status=%d, want %d, body: %s", resp.StatusCode, http.StatusAccepted, body)
				}
			},
		},
		{
			name:    "TLS initialization succeeds",
			startup: 200 * time.Millisecond,
			configOpts: func() []configOption {
				pk := generateTestPKI(t)
				return []configOption{
					withNodeID("tlsTst"),
					withTLS(
						writeTempFile(t, []byte(pk.NodeCert.CertPEM)),
						writeTempFile(t, []byte(pk.NodeKey.KeyPEM)),
						writeTempFile(t, []byte(pk.CA.CertPEM)),
					),
				}
			}(),
			check: func(t *testing.T, agent *Agent) {
				t.Helper()
				if agent.server.Address() == "" {
					t.Error("agent server address is empty after TLS init")
				}
			},
		},
		{
			name:       "heartbeat failure is non-fatal",
			serverOpts: []agentServerOption{withHeartbeatStatus(http.StatusInternalServerError)},
			configOpts: []configOption{withHeartbeatInterval(100 * time.Millisecond), withHeartbeatTimeout(2 * time.Second)},
			startup:    1500 * time.Millisecond,
			check: func(t *testing.T, agent *Agent) {
				// Agent is still running (not crashed) — that's the assertion.
				// Health check verifies the server is alive despite heartbeat failures.
				t.Helper()
				resp, err := http.Get(fmt.Sprintf("http://%s/health", agent.server.Address()))
				if err != nil {
					t.Fatalf("health check failed (agent should survive heartbeat errors): %v", err)
				}
				_ = resp.Body.Close()
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			nodeID := domaintypes.NodeID(testNodeID)
			for _, o := range tt.configOpts {
				// Peek for custom nodeID.
				var cfg Config
				o(&cfg)
				if cfg.NodeID != "" {
					nodeID = cfg.NodeID
				}
			}

			ts := newAgentMockServer(t, string(nodeID), tt.serverOpts...)
			cfg := newAgentConfig(ts.URL, tt.configOpts...)

			agent, err := New(cfg)
			if err != nil {
				t.Fatalf("New() error: %v", err)
			}
			installNoopStartupReconciler(agent.claimer)

			runErr := runAgentUntil(t, agent, tt.startup, 12*time.Second, tt.check)

			// Verify shutdown error is benign.
			if runErr != nil &&
				!errors.Is(runErr, context.Canceled) &&
				!errors.Is(runErr, context.DeadlineExceeded) &&
				!strings.Contains(runErr.Error(), "use of closed network connection") {
				t.Errorf("agent.Run() error = %v", runErr)
			}

			if tt.postCheck != nil {
				tt.postCheck(t, agent)
			}
		})
	}
}
