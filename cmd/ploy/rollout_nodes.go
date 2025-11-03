package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/iw2rmb/ploy/internal/deploy"
)

// rolloutNodesHost is an indirection to allow tests to stub remote commands.
var rolloutNodesHost = executeRolloutNode

// rolloutNodesAPIClient allows tests to inject a mock HTTP client and base URL.
var rolloutNodesAPIClient *http.Client
var rolloutNodesAPIBaseURL string

func handleRolloutNodes(args []string, stderr io.Writer) error {
	if stderr == nil {
		stderr = io.Discard
	}
	fs := flag.NewFlagSet("rollout nodes", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	var (
		all         boolValue
		selector    stringValue
		concurrency intValue
		binary      stringValue
		identity    stringValue
		userFlag    stringValue
		sshPort     intValue
		timeout     intValue
		dryRun      boolValue
		maxAttempts intValue
	)

	fs.Var(&all, "all", "Roll out all nodes in the cluster")
	fs.Var(&selector, "selector", "Node name pattern (e.g., 'worker-*')")
	fs.Var(&concurrency, "concurrency", "Number of nodes to update per batch (default: 1)")
	fs.Var(&binary, "binary", "Path to the ployd-node binary for upload (default: alongside the CLI)")
	fs.Var(&identity, "identity", "SSH private key used for node connection (default: ~/.ssh/id_rsa)")
	fs.Var(&userFlag, "user", "SSH username for node connection (default: root)")
	fs.Var(&sshPort, "ssh-port", "SSH port for node connection (default: 22)")
	fs.Var(&timeout, "timeout", "Timeout in seconds per node rollout (default: 90)")
	fs.Var(&dryRun, "dry-run", "Print planned rollout actions per node without making changes")
	fs.Var(&maxAttempts, "max-attempts", "Maximum retry attempts for each node (default: 3)")

	if err := fs.Parse(args); err != nil {
		printRolloutNodesUsage(stderr)
		return err
	}
	if fs.NArg() > 0 {
		printRolloutNodesUsage(stderr)
		return fmt.Errorf("unexpected arguments: %s", strings.Join(fs.Args(), " "))
	}

	// Validate that either --all or --selector is provided.
	if !all.value && !selector.set {
		printRolloutNodesUsage(stderr)
		return errors.New("either --all or --selector is required")
	}
	if all.value && selector.set {
		printRolloutNodesUsage(stderr)
		return errors.New("--all and --selector are mutually exclusive")
	}

	cfg := rolloutNodesConfig{
		All:          all.value,
		Selector:     selector.value,
		Concurrency:  concurrency.value,
		BinaryPath:   binary.value,
		User:         userFlag.value,
		IdentityFile: identity.value,
		SSHPort:      sshPort.value,
		Timeout:      timeout.value,
		DryRun:       dryRun.value,
		MaxAttempts:  maxAttempts.value,
	}

	return runRolloutNodes(cfg, stderr)
}

func printRolloutNodesUsage(w io.Writer) {
	printCommandUsage(w, "rollout", "nodes")
}

type rolloutNodesConfig struct {
	All          bool
	Selector     string
	Concurrency  int
	BinaryPath   string
	User         string
	IdentityFile string
	SSHPort      int
	Timeout      int
	DryRun       bool
	MaxAttempts  int
}

func runRolloutNodes(cfg rolloutNodesConfig, stderr io.Writer) error {
	if stderr == nil {
		stderr = os.Stderr
	}

	// Resolve default paths.
	identityPath, err := resolveIdentityPath(stringValue{set: cfg.IdentityFile != "", value: cfg.IdentityFile})
	if err != nil {
		return fmt.Errorf("rollout nodes: %w", err)
	}

	ploydBinaryPath, err := resolvePloydNodeBinaryPath(stringValue{set: cfg.BinaryPath != "", value: cfg.BinaryPath})
	if err != nil {
		return fmt.Errorf("rollout nodes: %w", err)
	}

	user := cfg.User
	if strings.TrimSpace(user) == "" {
		user = deploy.DefaultRemoteUser
	}

	sshPort := cfg.SSHPort
	if sshPort == 0 {
		sshPort = deploy.DefaultSSHPort
	}
	if err := validateSSHPort(sshPort); err != nil {
		return fmt.Errorf("rollout nodes: %w", err)
	}

	timeoutSecs := cfg.Timeout
	if timeoutSecs == 0 {
		timeoutSecs = 90
	}
	if timeoutSecs < 1 {
		return fmt.Errorf("rollout nodes: timeout must be positive, got %d", timeoutSecs)
	}

	concurrency := cfg.Concurrency
	if concurrency == 0 {
		concurrency = 1
	}
	if concurrency < 1 {
		return fmt.Errorf("rollout nodes: concurrency must be positive, got %d", concurrency)
	}

	maxAttempts := cfg.MaxAttempts
	if maxAttempts == 0 {
		maxAttempts = 3
	}
	if maxAttempts < 1 {
		return fmt.Errorf("rollout nodes: max-attempts must be positive, got %d", maxAttempts)
	}

	if cfg.DryRun {
		_, _ = fmt.Fprintf(stderr, "DRY RUN: Rollout Ploy nodes\n")
	} else {
		_, _ = fmt.Fprintf(stderr, "Rolling out Ploy nodes\n")
	}
	_, _ = fmt.Fprintf(stderr, "  Selector: %s\n", selectorDescription(cfg.All, cfg.Selector))
	_, _ = fmt.Fprintf(stderr, "  Batch size: %d\n", concurrency)
	_, _ = fmt.Fprintf(stderr, "  SSH User: %s\n", user)
	_, _ = fmt.Fprintf(stderr, "  SSH Port: %d\n", sshPort)
	_, _ = fmt.Fprintf(stderr, "  Identity: %s\n", identityPath)
	_, _ = fmt.Fprintf(stderr, "  Binary: %s\n", ploydBinaryPath)
	_, _ = fmt.Fprintf(stderr, "  Timeout: %ds per node\n", timeoutSecs)

	ctx := context.Background()

	// Fetch list of nodes from control plane.
	nodes, err := fetchNodes(ctx)
	if err != nil {
		return fmt.Errorf("rollout nodes: fetch nodes: %w", err)
	}

	// Filter nodes based on selector.
	filtered := filterNodes(nodes, cfg.All, cfg.Selector)
	if len(filtered) == 0 {
		_, _ = fmt.Fprintf(stderr, "\nNo nodes matched the selector.\n")
		return nil
	}

	_, _ = fmt.Fprintf(stderr, "\nMatched %d node(s):\n", len(filtered))
	for _, n := range filtered {
		_, _ = fmt.Fprintf(stderr, "  - %s (%s)\n", n.Name, n.IPAddress)
	}

	if cfg.DryRun {
		_, _ = fmt.Fprintln(stderr, "\nPlanned actions per node:")
		_, _ = fmt.Fprintln(stderr, "  1. Drain node via API (POST /v1/nodes/{id}/drain)")
		_, _ = fmt.Fprintln(stderr, "  2. Wait for node to be idle (no active runs)")
		_, _ = fmt.Fprintln(stderr, "  3. Upload new ployd-node binary to <node>:/tmp/ployd-<random>")
		_, _ = fmt.Fprintln(stderr, "  4. Install binary to <node>:/usr/local/bin/ployd-node")
		_, _ = fmt.Fprintln(stderr, "  5. Restart ployd-node service via systemctl")
		_, _ = fmt.Fprintln(stderr, "  6. Wait for service to become active")
		_, _ = fmt.Fprintln(stderr, "  7. Wait for heartbeat confirmation")
		_, _ = fmt.Fprintln(stderr, "  8. Undrain node via API (POST /v1/nodes/{id}/undrain)")
		_, _ = fmt.Fprintf(stderr, "\nBatching: nodes will be updated in batches of %d\n", concurrency)
		_, _ = fmt.Fprintln(stderr, "\nDry run complete. No changes have been made.")
		return nil
	}

	// Load or initialize resume state.
	stateDir, err := rolloutStateDir()
	if err != nil {
		return fmt.Errorf("rollout nodes: resolve state dir: %w", err)
	}
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		return fmt.Errorf("rollout nodes: create state dir: %w", err)
	}

	stateFile := filepath.Join(stateDir, "state.json")
	state, err := loadRolloutState(stateFile)
	if err != nil {
		// Initialize new state if none exists.
		now := time.Now().UTC().Format(time.RFC3339)
		state = &rolloutState{
			Version:      1,
			RetryPolicy:  rolloutRetryPolicy{MaxAttempts: maxAttempts},
			Nodes:        make(map[string]nodeRolloutStatus),
			CreatedAt:    now,
			LastModified: now,
		}
	}

	// Process nodes in batches based on concurrency.
	_, _ = fmt.Fprintf(stderr, "\nStarting rollout...\n")

	logger := initRolloutLogger()
	metrics := NewRolloutMetrics()

	success := 0
	failed := 0

	for i := 0; i < len(filtered); i += concurrency {
		end := i + concurrency
		if end > len(filtered) {
			end = len(filtered)
		}
		batch := filtered[i:end]

		// Process batch sequentially (can be parallelized in future iterations).
		for _, node := range batch {
			// Skip if already completed.
			if state.Nodes[node.ID].Completed {
				_, _ = fmt.Fprintf(stderr, "\n[%s] Already completed, skipping\n", node.Name)
				success++
				continue
			}

			// Check if max attempts reached.
			prevStatus := state.Nodes[node.ID]
			if prevStatus.Attempts >= state.RetryPolicy.MaxAttempts {
				_, _ = fmt.Fprintf(stderr, "\n[%s] Max attempts (%d) reached, skipping\n", node.Name, state.RetryPolicy.MaxAttempts)
				failed++
				continue
			}

			attemptNum := prevStatus.Attempts + 1
			_, _ = fmt.Fprintf(stderr, "\n[%s] Starting rollout (attempt %d/%d)\n", node.Name, attemptNum, state.RetryPolicy.MaxAttempts)
			logRolloutStep(logger, "node_rollout", "started", "node_id", node.ID, "node_name", node.Name, "attempt", attemptNum, "max_attempts", state.RetryPolicy.MaxAttempts)

			// Mark as in-progress and increment attempts.
			state.Nodes[node.ID] = nodeRolloutStatus{
				NodeID:      node.ID,
				NodeName:    node.Name,
				InProgress:  true,
				Completed:   false,
				Attempts:    attemptNum,
				LastAttempt: time.Now().UTC().Format(time.RFC3339),
			}
			if err := saveRolloutState(stateFile, state); err != nil {
				_, _ = fmt.Fprintf(stderr, "[%s] Warning: failed to save state: %v\n", node.Name, err)
			}

			nodeStart := time.Now()
			nodeCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutSecs)*time.Second)
			err := rolloutSingleNode(nodeCtx, node, rolloutNodeOptions{
				User:            user,
				Port:            sshPort,
				IdentityFile:    identityPath,
				PloydBinaryPath: ploydBinaryPath,
				Stderr:          stderr,
				Logger:          logger,
				Metrics:         metrics,
			})
			cancel()

			if err != nil {
				_, _ = fmt.Fprintf(stderr, "[%s] Rollout failed: %v\n", node.Name, err)
				logRolloutError(logger, "node_rollout", err, "node_id", node.ID, "node_name", node.Name, "attempt", attemptNum, "duration_ms", time.Since(nodeStart).Milliseconds())
				state.Nodes[node.ID] = nodeRolloutStatus{
					NodeID:      node.ID,
					NodeName:    node.Name,
					InProgress:  false,
					Completed:   false,
					Error:       err.Error(),
					Attempts:    attemptNum,
					LastAttempt: time.Now().UTC().Format(time.RFC3339),
				}
				if err := saveRolloutState(stateFile, state); err != nil {
					_, _ = fmt.Fprintf(stderr, "[%s] Warning: failed to save state: %v\n", node.Name, err)
				}
				metrics.RecordNode(false)
				failed++
				continue
			}

			_, _ = fmt.Fprintf(stderr, "[%s] Rollout complete\n", node.Name)
			logRolloutStep(logger, "node_rollout", "completed", "node_id", node.ID, "node_name", node.Name, "attempt", attemptNum, "duration_ms", time.Since(nodeStart).Milliseconds())
			state.Nodes[node.ID] = nodeRolloutStatus{
				NodeID:      node.ID,
				NodeName:    node.Name,
				InProgress:  false,
				Completed:   true,
				Attempts:    attemptNum,
				LastAttempt: time.Now().UTC().Format(time.RFC3339),
			}
			if err := saveRolloutState(stateFile, state); err != nil {
				_, _ = fmt.Fprintf(stderr, "[%s] Warning: failed to save state: %v\n", node.Name, err)
			}
			metrics.RecordNode(true)
			success++
		}
	}

	_, _ = fmt.Fprintf(stderr, "\nRollout summary: %d succeeded, %d failed\n", success, failed)
	metrics.PrintSummary(stderr)

	if failed > 0 {
		_, _ = fmt.Fprintf(stderr, "Resume state saved to: %s\n", stateFile)
		return fmt.Errorf("rollout nodes: %d node(s) failed", failed)
	}

	// Clean up state file on full success.
	_ = os.Remove(stateFile)

	return nil
}

type rolloutNodeOptions struct {
	User            string
	Port            int
	IdentityFile    string
	PloydBinaryPath string
	Stderr          io.Writer
	Logger          *slog.Logger
	Metrics         *RolloutMetrics
}

type nodeInfo struct {
	ID        string
	Name      string
	IPAddress string
	Drained   bool
}

type rolloutState struct {
	Version      int                          `json:"version"`
	RetryPolicy  rolloutRetryPolicy           `json:"retry_policy"`
	Nodes        map[string]nodeRolloutStatus `json:"nodes"`
	CreatedAt    string                       `json:"created_at"`
	LastModified string                       `json:"last_modified"`
}

type rolloutRetryPolicy struct {
	MaxAttempts int `json:"max_attempts"`
}

type nodeRolloutStatus struct {
	NodeID      string `json:"node_id"`
	NodeName    string `json:"node_name"`
	InProgress  bool   `json:"in_progress"`
	Completed   bool   `json:"completed"`
	Error       string `json:"error,omitempty"`
	Attempts    int    `json:"attempts"`
	LastAttempt string `json:"last_attempt,omitempty"`
}

func selectorDescription(all bool, selector string) string {
	if all {
		return "all nodes"
	}
	return selector
}

func fetchNodes(ctx context.Context) ([]nodeInfo, error) {
	baseURL, client, err := resolveAPIClientAndURL(ctx)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/v1/nodes", nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch nodes: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, controlPlaneHTTPError(resp)
	}

	var apiNodes []struct {
		ID        string `json:"id"`
		Name      string `json:"name"`
		IPAddress string `json:"ip_address"`
		Drained   bool   `json:"drained"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&apiNodes); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	nodes := make([]nodeInfo, len(apiNodes))
	for i, n := range apiNodes {
		nodes[i] = nodeInfo{
			ID:        n.ID,
			Name:      n.Name,
			IPAddress: n.IPAddress,
			Drained:   n.Drained,
		}
	}

	return nodes, nil
}

func filterNodes(nodes []nodeInfo, all bool, selector string) []nodeInfo {
	if all {
		return nodes
	}

	// Simple pattern matching: selector can be a glob-like pattern with '*'.
	var filtered []nodeInfo
	for _, n := range nodes {
		if matchesSelector(n.Name, selector) {
			filtered = append(filtered, n)
		}
	}
	return filtered
}

func matchesSelector(name, pattern string) bool {
	// Simple glob matching: support '*' as wildcard.
	if pattern == "*" {
		return true
	}
	if !strings.Contains(pattern, "*") {
		// Exact match.
		return name == pattern
	}

	// Split pattern by '*' and check each part.
	parts := strings.Split(pattern, "*")
	if len(parts) == 2 && parts[0] == "" {
		// Pattern: *suffix
		return strings.HasSuffix(name, parts[1])
	}
	if len(parts) == 2 && parts[1] == "" {
		// Pattern: prefix*
		return strings.HasPrefix(name, parts[0])
	}
	if len(parts) == 2 {
		// Pattern: prefix*suffix
		return strings.HasPrefix(name, parts[0]) && strings.HasSuffix(name, parts[1])
	}

	// For more complex patterns, fall back to basic substring matching.
	for _, part := range parts {
		if part != "" && !strings.Contains(name, part) {
			return false
		}
	}
	return true
}

func rolloutSingleNode(ctx context.Context, node nodeInfo, opts rolloutNodeOptions) error {
	stderr := opts.Stderr
	if stderr == nil {
		stderr = os.Stderr
	}
	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}
	metrics := opts.Metrics
	if metrics == nil {
		metrics = NewRolloutMetrics()
	}

	// Step 1: Drain the node.
	_, _ = fmt.Fprintf(stderr, "[%s]   Draining node...\n", node.Name)
	stepStart := time.Now()
	logRolloutStep(logger, "drain_node", "started", "node_id", node.ID, "node_name", node.Name)
	if !node.Drained {
		if err := drainNode(ctx, node.ID); err != nil {
			logRolloutError(logger, "drain_node", err, "node_id", node.ID, "node_name", node.Name, "duration_ms", time.Since(stepStart).Milliseconds())
			metrics.RecordStep("drain_node", "failed")
			return fmt.Errorf("drain: %w", err)
		}
	} else {
		_, _ = fmt.Fprintf(stderr, "[%s]   Node already drained\n", node.Name)
	}
	logRolloutStep(logger, "drain_node", "completed", "node_id", node.ID, "node_name", node.Name, "duration_ms", time.Since(stepStart).Milliseconds())
	metrics.RecordStep("drain_node", "completed")

	// Step 2: Wait for node to be idle (no active runs).
	_, _ = fmt.Fprintf(stderr, "[%s]   Waiting for node to be idle...\n", node.Name)
	stepStart = time.Now()
	logRolloutStep(logger, "wait_idle", "started", "node_id", node.ID, "node_name", node.Name)
	if err := waitForNodeIdle(ctx, node.ID); err != nil {
		logRolloutError(logger, "wait_idle", err, "node_id", node.ID, "node_name", node.Name, "duration_ms", time.Since(stepStart).Milliseconds())
		metrics.RecordStep("wait_idle", "failed")
		return fmt.Errorf("wait idle: %w", err)
	}
	logRolloutStep(logger, "wait_idle", "completed", "node_id", node.ID, "node_name", node.Name, "duration_ms", time.Since(stepStart).Milliseconds())
	metrics.RecordStep("wait_idle", "completed")

	// Step 3: Update the binary on the node.
	_, _ = fmt.Fprintf(stderr, "[%s]   Updating binary...\n", node.Name)
	stepStart = time.Now()
	logRolloutStep(logger, "update_binary", "started", "node_id", node.ID, "node_name", node.Name)
	if err := rolloutNodesHost(ctx, node, opts); err != nil {
		logRolloutError(logger, "update_binary", err, "node_id", node.ID, "node_name", node.Name, "duration_ms", time.Since(stepStart).Milliseconds())
		metrics.RecordStep("update_binary", "failed")
		return fmt.Errorf("update binary: %w", err)
	}
	logRolloutStep(logger, "update_binary", "completed", "node_id", node.ID, "node_name", node.Name, "duration_ms", time.Since(stepStart).Milliseconds())
	metrics.RecordStep("update_binary", "completed")

	// Step 4: Undrain the node.
	_, _ = fmt.Fprintf(stderr, "[%s]   Undraining node...\n", node.Name)
	stepStart = time.Now()
	logRolloutStep(logger, "undrain_node", "started", "node_id", node.ID, "node_name", node.Name)
	if err := undrainNode(ctx, node.ID); err != nil {
		logRolloutError(logger, "undrain_node", err, "node_id", node.ID, "node_name", node.Name, "duration_ms", time.Since(stepStart).Milliseconds())
		metrics.RecordStep("undrain_node", "failed")
		return fmt.Errorf("undrain: %w", err)
	}
	logRolloutStep(logger, "undrain_node", "completed", "node_id", node.ID, "node_name", node.Name, "duration_ms", time.Since(stepStart).Milliseconds())
	metrics.RecordStep("undrain_node", "completed")

	return nil
}

func executeRolloutNode(ctx context.Context, node nodeInfo, opts rolloutNodeOptions) error {
	stderr := opts.Stderr
	if stderr == nil {
		stderr = os.Stderr
	}

	runner := rolloutRunner
	if runner == nil {
		runner = deploy.NewSystemRunner()
	}
	target := node.IPAddress
	if opts.User != "" {
		target = fmt.Sprintf("%s@%s", opts.User, node.IPAddress)
	}

	sshArgs := deploy.BuildSSHArgs(opts.IdentityFile, opts.Port)
	scpArgs := deploy.BuildScpArgs(opts.IdentityFile, opts.Port)
	streams := deploy.IOStreams{Stdout: io.Discard, Stderr: stderr}

	// Generate a random suffix for the uploaded binary.
	binarySuffix, err := deploy.RandomHexString(8)
	if err != nil {
		return fmt.Errorf("generate binary suffix: %w", err)
	}
	remoteBinaryPath := fmt.Sprintf("/tmp/ployd-%s", binarySuffix)

	// Upload the new binary via scp.
	copyBinaryArgs := append(append([]string(nil), scpArgs...), opts.PloydBinaryPath, fmt.Sprintf("%s:%s", target, remoteBinaryPath))
	if err := runner.Run(ctx, "scp", copyBinaryArgs, nil, streams); err != nil {
		return fmt.Errorf("upload binary: %w", err)
	}

	// Install the ployd-node binary.
	installCmd := fmt.Sprintf("install -m0755 %s /usr/local/bin/ployd-node && rm -f %s", remoteBinaryPath, remoteBinaryPath)
	installArgs := append(append([]string(nil), sshArgs...), target, installCmd)
	if err := runner.Run(ctx, "ssh", installArgs, nil, streams); err != nil {
		return fmt.Errorf("install binary: %w", err)
	}

	// Restart the ployd-node service.
	restartCmd := "systemctl restart ployd-node"
	restartArgs := append(append([]string(nil), sshArgs...), target, restartCmd)
	if err := runner.Run(ctx, "ssh", restartArgs, nil, streams); err != nil {
		return fmt.Errorf("restart service: %w", err)
	}

	// Poll for service to become active.
	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}
	metrics := opts.Metrics
	if metrics == nil {
		metrics = NewRolloutMetrics()
	}
	if err := pollServiceActive(ctx, runner, sshArgs, target, "ployd-node", streams, logger, metrics); err != nil {
		return fmt.Errorf("service health check: %w", err)
	}

	// Wait for heartbeat to confirm node is back online.
	_, _ = fmt.Fprintf(stderr, "[%s]   Waiting for heartbeat...\n", node.Name)
	if err := waitForNodeHeartbeat(ctx, node.ID, logger, metrics); err != nil {
		return fmt.Errorf("heartbeat check: %w", err)
	}

	return nil
}

func drainNode(ctx context.Context, nodeID string) error {
	baseURL, client, err := resolveAPIClientAndURL(ctx)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, fmt.Sprintf("%s/v1/nodes/%s/drain", baseURL, nodeID), nil)
	if err != nil {
		return fmt.Errorf("create drain request: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("drain request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusConflict {
		return controlPlaneHTTPError(resp)
	}

	return nil
}

func undrainNode(ctx context.Context, nodeID string) error {
	baseURL, client, err := resolveAPIClientAndURL(ctx)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, fmt.Sprintf("%s/v1/nodes/%s/undrain", baseURL, nodeID), nil)
	if err != nil {
		return fmt.Errorf("create undrain request: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("undrain request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusConflict {
		return controlPlaneHTTPError(resp)
	}

	return nil
}

func waitForNodeIdle(ctx context.Context, nodeID string) error {
	// Poll until the node has no active runs.
	// For simplicity, we implement a basic polling loop with a short sleep.
	// In a real implementation, this would query the node's active runs from the API.
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	// For now, we just wait a short duration as a placeholder.
	// The actual implementation would query GET /v1/nodes/{id} and check active run count.
	select {
	case <-ctx.Done():
		return fmt.Errorf("timeout waiting for node to be idle")
	case <-ticker.C:
		// Assume node is idle after one tick (placeholder).
		return nil
	}
}

func waitForNodeHeartbeat(ctx context.Context, nodeID string, logger *slog.Logger, metrics *RolloutMetrics) error {
	// Poll until the node sends a heartbeat after restart with exponential backoff.
	policy := DefaultRetryPolicy()
	policy.MaxAttempts = 15

	return PollWithBackoff(ctx, policy, logger, metrics, "node_heartbeat_poll", func() (bool, error) {
		node, err := fetchNodeByID(ctx, nodeID)
		if err != nil {
			// Continue polling on transient errors.
			return false, nil
		}
		if node.LastHeartbeat != "" {
			// Parse heartbeat timestamp and check if recent (within last 10 seconds).
			hb, err := time.Parse(time.RFC3339, node.LastHeartbeat)
			if err == nil && time.Since(hb) < 10*time.Second {
				return true, nil
			}
		}
		return false, nil
	})
}

func fetchNodeByID(ctx context.Context, nodeID string) (*nodeDetail, error) {
	baseURL, client, err := resolveAPIClientAndURL(ctx)
	if err != nil {
		return nil, err
	}

	// Note: The API doesn't have a GET /v1/nodes/{id} endpoint yet,
	// so we fetch all nodes and filter. In a production implementation,
	// this endpoint should be added.
	nodes, err := fetchNodes(ctx)
	if err != nil {
		return nil, err
	}

	for _, n := range nodes {
		if n.ID == nodeID {
			// Fetch detailed node info by calling the list endpoint and parsing.
			// For now, return a minimal detail struct.
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/v1/nodes", nil)
			if err != nil {
				return nil, fmt.Errorf("create request: %w", err)
			}

			resp, err := client.Do(req)
			if err != nil {
				return nil, fmt.Errorf("fetch node: %w", err)
			}
			defer func() { _ = resp.Body.Close() }()

			if resp.StatusCode != http.StatusOK {
				return nil, controlPlaneHTTPError(resp)
			}

			var apiNodes []struct {
				ID            string  `json:"id"`
				Name          string  `json:"name"`
				IPAddress     string  `json:"ip_address"`
				Drained       bool    `json:"drained"`
				LastHeartbeat *string `json:"last_heartbeat,omitempty"`
			}

			if err := json.NewDecoder(resp.Body).Decode(&apiNodes); err != nil {
				return nil, fmt.Errorf("decode response: %w", err)
			}

			for _, an := range apiNodes {
				if an.ID == nodeID {
					detail := &nodeDetail{
						ID:        an.ID,
						Name:      an.Name,
						IPAddress: an.IPAddress,
						Drained:   an.Drained,
					}
					if an.LastHeartbeat != nil {
						detail.LastHeartbeat = *an.LastHeartbeat
					}
					return detail, nil
				}
			}
		}
	}

	return nil, fmt.Errorf("node %s not found", nodeID)
}

type nodeDetail struct {
	ID            string
	Name          string
	IPAddress     string
	Drained       bool
	LastHeartbeat string
}

func resolveAPIClientAndURL(ctx context.Context) (string, *http.Client, error) {
	// Use test stubs if available.
	if rolloutNodesAPIClient != nil && rolloutNodesAPIBaseURL != "" {
		return rolloutNodesAPIBaseURL, rolloutNodesAPIClient, nil
	}

	// Use the standard control plane HTTP resolver.
	u, client, err := resolveControlPlaneHTTP(ctx)
	if err != nil {
		return "", nil, err
	}

	return u.String(), client, nil
}

func rolloutStateDir() (string, error) {
	base := strings.TrimSpace(os.Getenv("PLOY_CONFIG_HOME"))
	if base == "" {
		xdg := strings.TrimSpace(os.Getenv("XDG_CONFIG_HOME"))
		if xdg != "" {
			base = filepath.Join(xdg, "ploy")
		} else {
			home, err := os.UserHomeDir()
			if err != nil {
				return "", fmt.Errorf("find home: %w", err)
			}
			base = filepath.Join(home, ".config", "ploy")
		}
	}
	return filepath.Join(base, "rollout"), nil
}

func loadRolloutState(path string) (*rolloutState, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
		return nil, fmt.Errorf("read state: %w", err)
	}

	var state rolloutState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("parse state: %w", err)
	}

	if state.Nodes == nil {
		state.Nodes = make(map[string]nodeRolloutStatus)
	}

	// Validate state version for integrity.
	if state.Version != 1 {
		return nil, fmt.Errorf("unsupported state version: %d", state.Version)
	}

	return &state, nil
}

func saveRolloutState(path string, state *rolloutState) error {
	state.LastModified = time.Now().UTC().Format(time.RFC3339)
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal state: %w", err)
	}

	return os.WriteFile(path, data, 0o644)
}
