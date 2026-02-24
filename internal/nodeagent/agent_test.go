package nodeagent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
)

// TestAgentCreation verifies that New() constructs all required components.
func TestAgentCreation(t *testing.T) {
	t.Parallel()

	cfg := Config{
		ServerURL:   "https://server.example.com",
		NodeID:      "aB3xY9",
		Concurrency: 2,
		HTTP: HTTPConfig{
			Listen: ":0", // random port
			TLS: TLSConfig{
				Enabled: false,
			},
		},
		Heartbeat: HeartbeatConfig{
			Interval: 30 * time.Second,
			Timeout:  10 * time.Second,
		},
	}

	agent, err := New(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify all components are initialized.
	if agent.server == nil {
		t.Error("server not initialized")
	}
	if agent.heartbeat == nil {
		t.Error("heartbeat manager not initialized")
	}
	if agent.claimer == nil {
		t.Error("claim manager not initialized")
	}
	if agent.controller == nil {
		t.Error("run controller not initialized")
	}
}

// TestAgentLifecycle verifies the full agent start/stop lifecycle.
func TestAgentLifecycle(t *testing.T) {
	t.Parallel()

	nodeID := "aB3xY9"
	// Create a mock control-plane server to handle heartbeats and claims.
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Handle heartbeat endpoint.
		if r.URL.Path == "/v1/nodes/"+nodeID+"/heartbeat" {
			w.WriteHeader(http.StatusOK)
			return
		}

		// Handle claim endpoint (unified jobs queue).
		if r.URL.Path == "/v1/nodes/"+nodeID+"/claim" {
			// Return no work available.
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{})
			return
		}

		w.WriteHeader(http.StatusNotFound)
	}))
	defer mockServer.Close()

	cfg := Config{
		ServerURL:   mockServer.URL,
		NodeID:      domaintypes.NodeID(nodeID),
		Concurrency: 1,
		HTTP: HTTPConfig{
			Listen: ":0", // random port
			TLS: TLSConfig{
				Enabled: false,
			},
		},
		Heartbeat: HeartbeatConfig{
			Interval: 100 * time.Millisecond, // fast for testing
			Timeout:  5 * time.Second,
		},
	}

	agent, err := New(cfg)
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}

	// Start the agent with a context that we'll cancel after a short time.
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- agent.Run(ctx)
	}()

	// Wait briefly to let the agent start all components.
	time.Sleep(200 * time.Millisecond)

	// Verify the server is listening by making a health check request.
	resp, err := http.Get(fmt.Sprintf("http://%s/health", agent.server.Address()))
	if err != nil {
		t.Fatalf("health check failed: %v", err)
	}
	defer func() {
		if resp != nil {
			_ = resp.Body.Close()
		}
	}()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("health check status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	// Wait for context cancellation and agent shutdown.
	select {
	case err := <-errCh:
		// Expect context.Canceled or context.DeadlineExceeded (from claim loop timeout).
		if err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
			t.Errorf("agent.Run() error = %v, want nil, context.Canceled, or context.DeadlineExceeded", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("agent did not shut down within timeout")
	}
}

// TestAgentGracefulShutdown verifies that the agent stops cleanly on context cancellation.
func TestAgentGracefulShutdown(t *testing.T) {
	t.Parallel()

	nodeID := "shutd0"
	// Create a mock control-plane server.
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Handle heartbeat endpoint.
		if r.URL.Path == "/v1/nodes/"+nodeID+"/heartbeat" {
			w.WriteHeader(http.StatusOK)
			return
		}

		// Handle claim endpoint (unified jobs queue).
		if r.URL.Path == "/v1/nodes/"+nodeID+"/claim" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{})
			return
		}

		w.WriteHeader(http.StatusNotFound)
	}))
	defer mockServer.Close()

	cfg := Config{
		ServerURL:   mockServer.URL,
		NodeID:      domaintypes.NodeID(nodeID),
		Concurrency: 1,
		HTTP: HTTPConfig{
			Listen: ":0",
			TLS: TLSConfig{
				Enabled: false,
			},
		},
		Heartbeat: HeartbeatConfig{
			Interval: 50 * time.Millisecond,
			Timeout:  5 * time.Second,
		},
	}

	agent, err := New(cfg)
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	errCh := make(chan error, 1)
	go func() {
		errCh <- agent.Run(ctx)
	}()

	// Let the agent run briefly.
	time.Sleep(150 * time.Millisecond)

	// Trigger graceful shutdown.
	cancel()

	// Verify shutdown completes within the shutdown timeout (10 seconds).
	select {
	case err := <-errCh:
		if err != nil && !errors.Is(err, context.Canceled) {
			t.Errorf("agent.Run() error = %v, want nil or context.Canceled", err)
		}
	case <-time.After(12 * time.Second):
		t.Fatal("agent did not shut down within shutdown timeout")
	}

	// Verify the server is no longer accepting connections.
	resp, err := http.Get(fmt.Sprintf("http://%s/health", agent.server.Address()))
	if err == nil && resp != nil {
		_ = resp.Body.Close()
		t.Error("expected connection error after shutdown, got nil")
	}
}

// TestAgentComponentIntegration verifies cross-component behavior: server receives
// run start request and coordinates with the run controller.
func TestAgentComponentIntegration(t *testing.T) {
	t.Parallel()

	nodeID := "intg01"
	// Create a mock control-plane server.
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/nodes/"+nodeID+"/heartbeat" {
			w.WriteHeader(http.StatusOK)
			return
		}
		if r.URL.Path == "/v1/nodes/"+nodeID+"/claim" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer mockServer.Close()

	cfg := Config{
		ServerURL:   mockServer.URL,
		NodeID:      domaintypes.NodeID(nodeID),
		Concurrency: 1,
		HTTP: HTTPConfig{
			Listen: ":0",
			TLS: TLSConfig{
				Enabled: false,
			},
		},
		Heartbeat: HeartbeatConfig{
			Interval: 100 * time.Millisecond,
			Timeout:  5 * time.Second,
		},
	}

	agent, err := New(cfg)
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- agent.Run(ctx)
	}()

	// Wait for agent to start.
	time.Sleep(200 * time.Millisecond)

	// Send a run start request to the agent's HTTP server.
	runID := domaintypes.NewRunID().String()
	jobID := domaintypes.NewJobID().String()
	startReq := map[string]string{
		"run_id":   runID,
		"job_id":   jobID,
		"job_type": "mig",
		"repo_url": "https://github.com/example/repo.git",
		"base_ref": "main",
	}
	reqBody, _ := json.Marshal(startReq)

	resp, err := http.Post(
		fmt.Sprintf("http://%s/v1/run/start", agent.server.Address()),
		"application/json",
		io.NopCloser(bytes.NewReader(reqBody)),
	)
	if err != nil {
		t.Fatalf("send run start request: %v", err)
	}
	defer func() {
		if resp != nil {
			_ = resp.Body.Close()
		}
	}()

	// Verify the server accepted the request.
	if resp.StatusCode != http.StatusAccepted {
		body, _ := io.ReadAll(resp.Body)
		t.Errorf("start run status = %d, want %d, body: %s", resp.StatusCode, http.StatusAccepted, body)
	}

	// Clean shutdown.
	cancel()
	select {
	case <-errCh:
	case <-time.After(2 * time.Second):
		t.Error("agent did not shut down")
	}
}

// TestAgentWithTLS verifies agent creation and startup with TLS enabled.
func TestAgentWithTLS(t *testing.T) {
	t.Parallel()

	// Generate test certificates.
	certPEM, keyPEM, caPEM := generateTestCerts(t)
	certPath := writeTempFile(t, certPEM)
	keyPath := writeTempFile(t, keyPEM)
	caPath := writeTempFile(t, caPEM)

	// Create a mock control-plane server with TLS.
	nodeID := "tlsTst"
	mockServer := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/nodes/"+nodeID+"/heartbeat" {
			w.WriteHeader(http.StatusOK)
			return
		}
		if r.URL.Path == "/v1/nodes/"+nodeID+"/claim" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer mockServer.Close()

	cfg := Config{
		ServerURL:   mockServer.URL,
		NodeID:      domaintypes.NodeID(nodeID),
		Concurrency: 1,
		HTTP: HTTPConfig{
			Listen: ":0",
			TLS: TLSConfig{
				Enabled:  true,
				CertPath: certPath,
				KeyPath:  keyPath,
				CAPath:   caPath,
			},
		},
		Heartbeat: HeartbeatConfig{
			Interval: 100 * time.Millisecond,
			Timeout:  5 * time.Second,
		},
	}

	agent, err := New(cfg)
	if err != nil {
		t.Fatalf("create agent with TLS: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- agent.Run(ctx)
	}()

	// Wait briefly for startup.
	time.Sleep(200 * time.Millisecond)

	// Verify the server address is set (indicates successful TLS initialization).
	if agent.server.Address() == "" {
		t.Error("agent server address is empty")
	}

	// Wait for shutdown.
	select {
	case err := <-errCh:
		if err != nil &&
			!errors.Is(err, context.Canceled) &&
			!errors.Is(err, context.DeadlineExceeded) &&
			// Some platforms return a net.OpError wrapping "use of closed network connection"
			// when the listener has already been closed. Treat it as benign.
			!strings.Contains(err.Error(), "use of closed network connection") {
			t.Errorf("agent.Run() error = %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("agent with TLS did not shut down")
	}
}

// TestAgentHeartbeatFailure verifies that the agent continues running even if
// heartbeat fails (heartbeat errors are non-fatal).
func TestAgentHeartbeatFailure(t *testing.T) {
	t.Parallel()

	var heartbeatAttempts int
	nodeID := "hbFail"

	// Create a mock server that always fails heartbeats.
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/nodes/"+nodeID+"/heartbeat" {
			heartbeatAttempts++
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		if r.URL.Path == "/v1/nodes/"+nodeID+"/claim" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer mockServer.Close()

	cfg := Config{
		ServerURL:   mockServer.URL,
		NodeID:      domaintypes.NodeID(nodeID),
		Concurrency: 1,
		HTTP: HTTPConfig{
			Listen: ":0",
			TLS: TLSConfig{
				Enabled: false,
			},
		},
		Heartbeat: HeartbeatConfig{
			Interval: 100 * time.Millisecond,
			Timeout:  2 * time.Second,
		},
	}

	agent, err := New(cfg)
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}

	// Run agent for a bit to ensure heartbeat attempts.
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- agent.Run(ctx)
	}()

	// Wait for heartbeat attempts.
	time.Sleep(1500 * time.Millisecond)

	select {
	case err := <-errCh:
		// Agent should shut down cleanly even with heartbeat failures.
		if err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
			t.Errorf("agent.Run() error = %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("agent did not shut down")
	}

	// Verify that heartbeats were attempted despite failures.
	if heartbeatAttempts < 1 {
		t.Errorf("heartbeat attempts = %d, want at least 1", heartbeatAttempts)
	}
}
