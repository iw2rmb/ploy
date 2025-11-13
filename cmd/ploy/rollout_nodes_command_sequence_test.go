package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/iw2rmb/ploy/internal/deploy"
)

func TestExecuteRolloutNodeCommandSequence(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping rollout node command sequence test in short mode")
	}

	// Create a recording runner to track command execution.
	var calls [][]string
	runner := deploy.RunnerFunc(func(ctx context.Context, command string, args []string, stdin io.Reader, streams deploy.IOStreams) error {
		call := append([]string{command}, args...)
		calls = append(calls, call)
		return nil
	})

	// Stub the rollout runner.
	old := rolloutRunner
	rolloutRunner = runner
	defer func() { rolloutRunner = old }()

	// Stub control-plane API to provide immediate heartbeat so polling returns quickly.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/v1/nodes" {
			now := time.Now().UTC().Format(time.RFC3339)
			resp := []struct {
				ID            string  `json:"id"`
				Name          string  `json:"name"`
				IPAddress     string  `json:"ip_address"`
				Drained       bool    `json:"drained"`
				LastHeartbeat *string `json:"last_heartbeat,omitempty"`
			}{
				{ID: "node-1", Name: "worker-1", IPAddress: "10.0.0.1", Drained: false, LastHeartbeat: &now},
			}
			_ = json.NewEncoder(w).Encode(resp)
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	oldClient := rolloutNodesAPIClient
	oldBase := rolloutNodesAPIBaseURL
	rolloutNodesAPIClient = srv.Client()
	rolloutNodesAPIBaseURL = srv.URL
	defer func() {
		rolloutNodesAPIClient = oldClient
		rolloutNodesAPIBaseURL = oldBase
	}()

	node := nodeInfo{
		ID:        "node-1",
		Name:      "worker-1",
		IPAddress: "10.0.0.1",
	}

	opts := rolloutNodeOptions{
		User:            "testuser",
		Port:            22,
		IdentityFile:    "/tmp/id_test",
		PloydBinaryPath: "/tmp/ployd-test",
		Stderr:          io.Discard,
	}

	ctx := context.Background()
	_ = executeRolloutNode(ctx, node, opts)

	// Verify expected command sequence:
	// 1. scp (upload binary)
	// 2. ssh (install binary)
	// 3. ssh (restart service)
	// 4. ssh (check service active) - may be called multiple times
	if len(calls) < 4 {
		t.Fatalf("expected at least 4 commands (scp, ssh install, ssh restart, ssh check), got %d", len(calls))
	}

	// Check first command is scp.
	if calls[0][0] != "scp" {
		t.Fatalf("expected first command to be scp, got: %v", calls[0])
	}

	// Check second command is ssh with install.
	if calls[1][0] != "ssh" {
		t.Fatalf("expected second command to be ssh, got: %v", calls[1])
	}
	installCmd := strings.Join(calls[1], " ")
	if !strings.Contains(installCmd, "install") {
		t.Fatalf("expected second command to contain 'install', got: %v", calls[1])
	}

	// Check third command is ssh with systemctl restart ployd-node.
	if calls[2][0] != "ssh" {
		t.Fatalf("expected third command to be ssh, got: %v", calls[2])
	}
	restartCmd := strings.Join(calls[2], " ")
	if !strings.Contains(restartCmd, "systemctl restart ployd-node") {
		t.Fatalf("expected third command to restart 'ployd-node', got: %v", calls[2])
	}

	// Check subsequent commands are ssh (health checks).
	for i := 3; i < len(calls); i++ {
		if calls[i][0] != "ssh" {
			t.Fatalf("expected command %d to be ssh, got: %v", i, calls[i])
		}
	}
}
