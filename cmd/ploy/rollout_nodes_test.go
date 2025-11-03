package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/iw2rmb/ploy/internal/deploy"
)

func TestHandleRolloutNodesRequiresSelector(t *testing.T) {
	buf := &bytes.Buffer{}
	err := handleRolloutNodes(nil, buf)
	if err == nil {
		t.Fatalf("expected error when no selector provided")
	}
	if !strings.Contains(err.Error(), "either --all or --selector is required") {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(buf.String(), "Usage: ploy rollout nodes") {
		t.Fatalf("expected rollout nodes usage output, got: %q", buf.String())
	}
}

func TestHandleRolloutNodesRejectsBothAllAndSelector(t *testing.T) {
	buf := &bytes.Buffer{}
	err := handleRolloutNodes([]string{"--all", "--selector", "worker-*"}, buf)
	if err == nil {
		t.Fatalf("expected error when both --all and --selector provided")
	}
	if !strings.Contains(err.Error(), "--all and --selector are mutually exclusive") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestHandleRolloutNodesRejectsExtraArgs(t *testing.T) {
	buf := &bytes.Buffer{}
	err := handleRolloutNodes([]string{"--all", "extra"}, buf)
	if err == nil {
		t.Fatalf("expected error for unexpected args")
	}
	if !strings.Contains(err.Error(), "unexpected arguments:") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestHandleRolloutNodesValidatesConcurrency(t *testing.T) {
	tmpDir := t.TempDir()
	binPath := filepath.Join(tmpDir, "ployd-test")
	if err := os.WriteFile(binPath, []byte("fake binary"), 0755); err != nil {
		t.Fatalf("create test binary: %v", err)
	}
	identityPath := filepath.Join(tmpDir, "id_test")
	if err := os.WriteFile(identityPath, []byte("fake key"), 0600); err != nil {
		t.Fatalf("create test identity: %v", err)
	}

	// Stub API client to return empty node list.
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("[]"))
	}))
	defer mockServer.Close()

	old := rolloutNodesAPIClient
	oldURL := rolloutNodesAPIBaseURL
	rolloutNodesAPIClient = mockServer.Client()
	rolloutNodesAPIBaseURL = mockServer.URL
	defer func() {
		rolloutNodesAPIClient = old
		rolloutNodesAPIBaseURL = oldURL
	}()

	tests := []struct {
		name        string
		concurrency int
		expectErr   bool
	}{
		{"valid concurrency 1", 1, false},
		{"valid concurrency 5", 5, false},
		{"default concurrency 0", 0, false}, // 0 defaults to 1.
		{"invalid concurrency -1", -1, true},
		{"invalid concurrency -10", -10, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := rolloutNodesConfig{
				All:          true,
				Concurrency:  tt.concurrency,
				BinaryPath:   binPath,
				IdentityFile: identityPath,
			}

			err := runRolloutNodes(cfg, io.Discard)
			if tt.expectErr {
				if err == nil {
					t.Fatalf("expected error for concurrency %d", tt.concurrency)
				}
				if !strings.Contains(err.Error(), "concurrency must be positive") {
					t.Fatalf("expected concurrency validation error, got: %v", err)
				}
			} else {
				// For valid concurrency with empty node list, we expect success.
				if err != nil && strings.Contains(err.Error(), "concurrency must be positive") {
					t.Fatalf("unexpected concurrency validation error for valid concurrency %d: %v", tt.concurrency, err)
				}
			}
		})
	}
}

func TestHandleRolloutNodesValidatesTimeout(t *testing.T) {
	tmpDir := t.TempDir()
	binPath := filepath.Join(tmpDir, "ployd-test")
	if err := os.WriteFile(binPath, []byte("fake binary"), 0755); err != nil {
		t.Fatalf("create test binary: %v", err)
	}
	identityPath := filepath.Join(tmpDir, "id_test")
	if err := os.WriteFile(identityPath, []byte("fake key"), 0600); err != nil {
		t.Fatalf("create test identity: %v", err)
	}

	// Stub API client to return empty node list.
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("[]"))
	}))
	defer mockServer.Close()

	old := rolloutNodesAPIClient
	oldURL := rolloutNodesAPIBaseURL
	rolloutNodesAPIClient = mockServer.Client()
	rolloutNodesAPIBaseURL = mockServer.URL
	defer func() {
		rolloutNodesAPIClient = old
		rolloutNodesAPIBaseURL = oldURL
	}()

	tests := []struct {
		name      string
		timeout   int
		expectErr bool
	}{
		{"valid timeout 90", 90, false},
		{"valid timeout 120", 120, false},
		{"default timeout 0", 0, false}, // 0 defaults to 90.
		{"invalid timeout -1", -1, true},
		{"invalid timeout -100", -100, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := rolloutNodesConfig{
				All:          true,
				Timeout:      tt.timeout,
				BinaryPath:   binPath,
				IdentityFile: identityPath,
			}

			err := runRolloutNodes(cfg, io.Discard)
			if tt.expectErr {
				if err == nil {
					t.Fatalf("expected error for timeout %d", tt.timeout)
				}
				if !strings.Contains(err.Error(), "timeout must be positive") {
					t.Fatalf("expected timeout validation error, got: %v", err)
				}
			} else {
				// For valid timeout with empty node list, we expect success.
				if err != nil && strings.Contains(err.Error(), "timeout must be positive") {
					t.Fatalf("unexpected timeout validation error for valid timeout %d: %v", tt.timeout, err)
				}
			}
		})
	}
}

func TestFilterNodesAll(t *testing.T) {
	nodes := []nodeInfo{
		{ID: "1", Name: "worker-1", IPAddress: "10.0.0.1"},
		{ID: "2", Name: "worker-2", IPAddress: "10.0.0.2"},
		{ID: "3", Name: "server-1", IPAddress: "10.0.0.3"},
	}

	filtered := filterNodes(nodes, true, "")
	if len(filtered) != 3 {
		t.Fatalf("expected 3 nodes with --all, got %d", len(filtered))
	}
}

func TestFilterNodesSelector(t *testing.T) {
	nodes := []nodeInfo{
		{ID: "1", Name: "worker-1", IPAddress: "10.0.0.1"},
		{ID: "2", Name: "worker-2", IPAddress: "10.0.0.2"},
		{ID: "3", Name: "server-1", IPAddress: "10.0.0.3"},
	}

	tests := []struct {
		name     string
		selector string
		expected int
	}{
		{"prefix wildcard", "worker-*", 2},
		{"suffix wildcard", "*-1", 2},
		{"exact match", "worker-1", 1},
		{"no match", "non-existent", 0},
		{"match all wildcard", "*", 3},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filtered := filterNodes(nodes, false, tt.selector)
			if len(filtered) != tt.expected {
				t.Fatalf("selector %q: expected %d nodes, got %d", tt.selector, tt.expected, len(filtered))
			}
		})
	}
}

func TestMatchesSelector(t *testing.T) {
	tests := []struct {
		name     string
		nodeName string
		pattern  string
		expected bool
	}{
		{"exact match", "worker-1", "worker-1", true},
		{"prefix wildcard match", "worker-1", "worker-*", true},
		{"prefix wildcard no match", "server-1", "worker-*", false},
		{"suffix wildcard match", "worker-1", "*-1", true},
		{"suffix wildcard no match", "worker-2", "*-1", false},
		{"wildcard match", "anything", "*", true},
		{"prefix-suffix match", "worker-1-prod", "worker-*-prod", true},
		{"prefix-suffix no match", "worker-1-dev", "worker-*-prod", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := matchesSelector(tt.nodeName, tt.pattern)
			if result != tt.expected {
				t.Fatalf("matchesSelector(%q, %q): expected %v, got %v", tt.nodeName, tt.pattern, tt.expected, result)
			}
		})
	}
}

func TestRolloutNodesEmptyList(t *testing.T) {
	tmpDir := t.TempDir()
	binPath := filepath.Join(tmpDir, "ployd-test")
	if err := os.WriteFile(binPath, []byte("fake binary"), 0755); err != nil {
		t.Fatalf("create test binary: %v", err)
	}
	identityPath := filepath.Join(tmpDir, "id_test")
	if err := os.WriteFile(identityPath, []byte("fake key"), 0600); err != nil {
		t.Fatalf("create test identity: %v", err)
	}

	// Stub API client to return empty node list.
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("[]"))
	}))
	defer mockServer.Close()

	old := rolloutNodesAPIClient
	oldURL := rolloutNodesAPIBaseURL
	rolloutNodesAPIClient = mockServer.Client()
	rolloutNodesAPIBaseURL = mockServer.URL
	defer func() {
		rolloutNodesAPIClient = old
		rolloutNodesAPIBaseURL = oldURL
	}()

	buf := &bytes.Buffer{}
	cfg := rolloutNodesConfig{
		All:          true,
		BinaryPath:   binPath,
		IdentityFile: identityPath,
	}

	err := runRolloutNodes(cfg, buf)
	if err != nil {
		t.Fatalf("expected success for empty node list, got: %v", err)
	}
	if !strings.Contains(buf.String(), "No nodes matched the selector") {
		t.Fatalf("expected message about no matching nodes, got: %q", buf.String())
	}
}

func TestRolloutNodesWithSelector(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping rollout nodes test in short mode")
	}

	tmpDir := t.TempDir()
	binPath := filepath.Join(tmpDir, "ployd-test")
	if err := os.WriteFile(binPath, []byte("fake binary"), 0755); err != nil {
		t.Fatalf("create test binary: %v", err)
	}
	identityPath := filepath.Join(tmpDir, "id_test")
	if err := os.WriteFile(identityPath, []byte("fake key"), 0600); err != nil {
		t.Fatalf("create test identity: %v", err)
	}

	// Create a mock server for API calls.
	drainCalled := 0
	undrainCalled := 0
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/v1/nodes" {
			nodes := []map[string]interface{}{
				{"id": "node-1", "name": "worker-1", "ip_address": "10.0.0.1", "drained": false, "last_heartbeat": "2025-11-03T12:00:00Z"},
				{"id": "node-2", "name": "worker-2", "ip_address": "10.0.0.2", "drained": false, "last_heartbeat": "2025-11-03T12:00:00Z"},
				{"id": "node-3", "name": "server-1", "ip_address": "10.0.0.3", "drained": false, "last_heartbeat": "2025-11-03T12:00:00Z"},
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(nodes)
			return
		}
		if r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/drain") {
			drainCalled++
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/undrain") {
			undrainCalled++
			w.WriteHeader(http.StatusNoContent)
			return
		}
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer mockServer.Close()

	old := rolloutNodesAPIClient
	oldURL := rolloutNodesAPIBaseURL
	rolloutNodesAPIClient = mockServer.Client()
	rolloutNodesAPIBaseURL = mockServer.URL
	defer func() {
		rolloutNodesAPIClient = old
		rolloutNodesAPIBaseURL = oldURL
	}()

	// Stub the rollout host function to avoid real SSH.
	oldHost := rolloutNodesHost
	rolloutNodesHost = func(ctx context.Context, node nodeInfo, opts rolloutNodeOptions) error {
		return nil
	}
	defer func() { rolloutNodesHost = oldHost }()

	buf := &bytes.Buffer{}
	cfg := rolloutNodesConfig{
		Selector:     "worker-*",
		BinaryPath:   binPath,
		IdentityFile: identityPath,
	}

	err := runRolloutNodes(cfg, buf)
	if err != nil {
		t.Fatalf("expected success, got: %v", err)
	}

	// Verify that two worker nodes were selected.
	if !strings.Contains(buf.String(), "Matched 2 node(s)") {
		t.Fatalf("expected 2 nodes matched, got: %q", buf.String())
	}

	// Verify drain and undrain were called for each node.
	if drainCalled != 2 {
		t.Fatalf("expected 2 drain calls, got %d", drainCalled)
	}
	if undrainCalled != 2 {
		t.Fatalf("expected 2 undrain calls, got %d", undrainCalled)
	}
}

func TestRolloutNodesWithFailure(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping rollout nodes failure test in short mode")
	}

	tmpDir := t.TempDir()
	binPath := filepath.Join(tmpDir, "ployd-test")
	if err := os.WriteFile(binPath, []byte("fake binary"), 0755); err != nil {
		t.Fatalf("create test binary: %v", err)
	}
	identityPath := filepath.Join(tmpDir, "id_test")
	if err := os.WriteFile(identityPath, []byte("fake key"), 0600); err != nil {
		t.Fatalf("create test identity: %v", err)
	}

	// Create a mock server for API calls.
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/v1/nodes" {
			nodes := []map[string]interface{}{
				{"id": "node-1", "name": "worker-1", "ip_address": "10.0.0.1", "drained": false, "last_heartbeat": "2025-11-03T12:00:00Z"},
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(nodes)
			return
		}
		if r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/drain") {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/undrain") {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer mockServer.Close()

	old := rolloutNodesAPIClient
	oldURL := rolloutNodesAPIBaseURL
	rolloutNodesAPIClient = mockServer.Client()
	rolloutNodesAPIBaseURL = mockServer.URL
	defer func() {
		rolloutNodesAPIClient = old
		rolloutNodesAPIBaseURL = oldURL
	}()

	// Stub the rollout host function to simulate a failure.
	oldHost := rolloutNodesHost
	rolloutNodesHost = func(ctx context.Context, node nodeInfo, opts rolloutNodeOptions) error {
		return errors.New("simulated SSH failure")
	}
	defer func() { rolloutNodesHost = oldHost }()

	buf := &bytes.Buffer{}
	cfg := rolloutNodesConfig{
		All:          true,
		BinaryPath:   binPath,
		IdentityFile: identityPath,
	}

	err := runRolloutNodes(cfg, buf)
	if err == nil {
		t.Fatalf("expected error for rollout failure")
	}
	if !strings.Contains(err.Error(), "1 node(s) failed") {
		t.Fatalf("expected error about failed nodes, got: %v", err)
	}
	if !strings.Contains(buf.String(), "Rollout failed") {
		t.Fatalf("expected failure message in output, got: %q", buf.String())
	}
}

func TestRolloutNodesResume(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping rollout nodes resume test in short mode")
	}

	tmpDir := t.TempDir()
	binPath := filepath.Join(tmpDir, "ployd-test")
	if err := os.WriteFile(binPath, []byte("fake binary"), 0755); err != nil {
		t.Fatalf("create test binary: %v", err)
	}
	identityPath := filepath.Join(tmpDir, "id_test")
	if err := os.WriteFile(identityPath, []byte("fake key"), 0600); err != nil {
		t.Fatalf("create test identity: %v", err)
	}

	// Create a mock server for API calls.
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/v1/nodes" {
			nodes := []map[string]interface{}{
				{"id": "node-1", "name": "worker-1", "ip_address": "10.0.0.1", "drained": false, "last_heartbeat": "2025-11-03T12:00:00Z"},
				{"id": "node-2", "name": "worker-2", "ip_address": "10.0.0.2", "drained": false, "last_heartbeat": "2025-11-03T12:00:00Z"},
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(nodes)
			return
		}
		if r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/drain") {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/undrain") {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer mockServer.Close()

	old := rolloutNodesAPIClient
	oldURL := rolloutNodesAPIBaseURL
	rolloutNodesAPIClient = mockServer.Client()
	rolloutNodesAPIBaseURL = mockServer.URL
	defer func() {
		rolloutNodesAPIClient = old
		rolloutNodesAPIBaseURL = oldURL
	}()

	// Override state directory to use temp dir.
	oldEnv := os.Getenv("PLOY_CONFIG_HOME")
	_ = os.Setenv("PLOY_CONFIG_HOME", tmpDir)
	defer func() { _ = os.Setenv("PLOY_CONFIG_HOME", oldEnv) }()

	stateDir := filepath.Join(tmpDir, "rollout")
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		t.Fatalf("create state dir: %v", err)
	}

	// Create a pre-existing state file with one completed node.
	stateFile := filepath.Join(stateDir, "state.json")
	state := &rolloutState{
		Nodes: map[string]nodeRolloutStatus{
			"node-1": {
				NodeID:     "node-1",
				NodeName:   "worker-1",
				InProgress: false,
				Completed:  true,
			},
		},
	}
	data, _ := json.MarshalIndent(state, "", "  ")
	if err := os.WriteFile(stateFile, data, 0o644); err != nil {
		t.Fatalf("write state file: %v", err)
	}

	// Stub the rollout host function.
	var rolledOutNodes []string
	oldHost := rolloutNodesHost
	rolloutNodesHost = func(ctx context.Context, node nodeInfo, opts rolloutNodeOptions) error {
		rolledOutNodes = append(rolledOutNodes, node.Name)
		return nil
	}
	defer func() { rolloutNodesHost = oldHost }()

	buf := &bytes.Buffer{}
	cfg := rolloutNodesConfig{
		All:          true,
		BinaryPath:   binPath,
		IdentityFile: identityPath,
	}

	err := runRolloutNodes(cfg, buf)
	if err != nil {
		t.Fatalf("expected success, got: %v", err)
	}

	// Verify that only worker-2 was rolled out (worker-1 was already completed).
	if len(rolledOutNodes) != 1 || rolledOutNodes[0] != "worker-2" {
		t.Fatalf("expected only worker-2 to be rolled out, got: %v", rolledOutNodes)
	}

	// Verify state file was cleaned up on success.
	if _, err := os.Stat(stateFile); !os.IsNotExist(err) {
		t.Fatalf("expected state file to be cleaned up on success")
	}
}

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

func TestLoadAndSaveRolloutState(t *testing.T) {
	tmpDir := t.TempDir()
	stateFile := filepath.Join(tmpDir, "state.json")

	// Test saving state.
	state := &rolloutState{
		Nodes: map[string]nodeRolloutStatus{
			"node-1": {
				NodeID:     "node-1",
				NodeName:   "worker-1",
				InProgress: true,
				Completed:  false,
			},
		},
	}

	if err := saveRolloutState(stateFile, state); err != nil {
		t.Fatalf("save state: %v", err)
	}

	// Test loading state.
	loaded, err := loadRolloutState(stateFile)
	if err != nil {
		t.Fatalf("load state: %v", err)
	}

	if len(loaded.Nodes) != 1 {
		t.Fatalf("expected 1 node in state, got %d", len(loaded.Nodes))
	}

	status, ok := loaded.Nodes["node-1"]
	if !ok {
		t.Fatalf("expected node-1 in state")
	}

	if status.NodeName != "worker-1" {
		t.Errorf("expected node name worker-1, got %s", status.NodeName)
	}

	if !status.InProgress {
		t.Errorf("expected in_progress to be true")
	}

	if status.Completed {
		t.Errorf("expected completed to be false")
	}
}

func TestLoadRolloutStateNonExistent(t *testing.T) {
	tmpDir := t.TempDir()
	stateFile := filepath.Join(tmpDir, "non-existent.json")

	_, err := loadRolloutState(stateFile)
	if err == nil {
		t.Fatalf("expected error for non-existent state file")
	}
	if !os.IsNotExist(err) {
		t.Fatalf("expected os.IsNotExist error, got: %v", err)
	}
}
