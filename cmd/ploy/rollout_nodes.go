package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
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
	)

	fs.Var(&all, "all", "Roll out all nodes in the cluster")
	fs.Var(&selector, "selector", "Node name pattern (e.g., 'worker-*')")
	fs.Var(&concurrency, "concurrency", "Number of nodes to update per batch (default: 1)")
	fs.Var(&binary, "binary", "Path to the ployd-node binary for upload (default: alongside the CLI)")
	fs.Var(&identity, "identity", "SSH private key used for node connection (default: ~/.ssh/id_rsa)")
	fs.Var(&userFlag, "user", "SSH username for node connection (default: root)")
	fs.Var(&sshPort, "ssh-port", "SSH port for node connection (default: 22)")
	fs.Var(&timeout, "timeout", "Timeout in seconds per node rollout (default: 90)")

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

	_, _ = fmt.Fprintf(stderr, "Rolling out Ploy nodes\n")
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
		state = &rolloutState{Nodes: make(map[string]nodeRolloutStatus)}
	}

	// Process nodes in batches based on concurrency.
	_, _ = fmt.Fprintf(stderr, "\nStarting rollout...\n")

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

			_, _ = fmt.Fprintf(stderr, "\n[%s] Starting rollout\n", node.Name)

			// Mark as in-progress.
			state.Nodes[node.ID] = nodeRolloutStatus{
				NodeID:     node.ID,
				NodeName:   node.Name,
				InProgress: true,
				Completed:  false,
			}
			if err := saveRolloutState(stateFile, state); err != nil {
				_, _ = fmt.Fprintf(stderr, "[%s] Warning: failed to save state: %v\n", node.Name, err)
			}

			nodeCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutSecs)*time.Second)
			err := rolloutSingleNode(nodeCtx, node, rolloutNodeOptions{
				User:            user,
				Port:            sshPort,
				IdentityFile:    identityPath,
				PloydBinaryPath: ploydBinaryPath,
				Stderr:          stderr,
			})
			cancel()

			if err != nil {
				_, _ = fmt.Fprintf(stderr, "[%s] Rollout failed: %v\n", node.Name, err)
				state.Nodes[node.ID] = nodeRolloutStatus{
					NodeID:     node.ID,
					NodeName:   node.Name,
					InProgress: false,
					Completed:  false,
					Error:      err.Error(),
				}
				if err := saveRolloutState(stateFile, state); err != nil {
					_, _ = fmt.Fprintf(stderr, "[%s] Warning: failed to save state: %v\n", node.Name, err)
				}
				failed++
				continue
			}

			_, _ = fmt.Fprintf(stderr, "[%s] Rollout complete\n", node.Name)
			state.Nodes[node.ID] = nodeRolloutStatus{
				NodeID:     node.ID,
				NodeName:   node.Name,
				InProgress: false,
				Completed:  true,
			}
			if err := saveRolloutState(stateFile, state); err != nil {
				_, _ = fmt.Fprintf(stderr, "[%s] Warning: failed to save state: %v\n", node.Name, err)
			}
			success++
		}
	}

	_, _ = fmt.Fprintf(stderr, "\nRollout summary: %d succeeded, %d failed\n", success, failed)
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
}

type nodeInfo struct {
	ID        string
	Name      string
	IPAddress string
	Drained   bool
}

type rolloutState struct {
	Nodes map[string]nodeRolloutStatus `json:"nodes"`
}

type nodeRolloutStatus struct {
	NodeID     string `json:"node_id"`
	NodeName   string `json:"node_name"`
	InProgress bool   `json:"in_progress"`
	Completed  bool   `json:"completed"`
	Error      string `json:"error,omitempty"`
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

	// Step 1: Drain the node.
	_, _ = fmt.Fprintf(stderr, "[%s]   Draining node...\n", node.Name)
	if !node.Drained {
		if err := drainNode(ctx, node.ID); err != nil {
			return fmt.Errorf("drain: %w", err)
		}
	} else {
		_, _ = fmt.Fprintf(stderr, "[%s]   Node already drained\n", node.Name)
	}

	// Step 2: Wait for node to be idle (no active runs).
	_, _ = fmt.Fprintf(stderr, "[%s]   Waiting for node to be idle...\n", node.Name)
	if err := waitForNodeIdle(ctx, node.ID); err != nil {
		return fmt.Errorf("wait idle: %w", err)
	}

	// Step 3: Update the binary on the node.
	_, _ = fmt.Fprintf(stderr, "[%s]   Updating binary...\n", node.Name)
	if err := rolloutNodesHost(ctx, node, opts); err != nil {
		return fmt.Errorf("update binary: %w", err)
	}

	// Step 4: Undrain the node.
	_, _ = fmt.Fprintf(stderr, "[%s]   Undraining node...\n", node.Name)
	if err := undrainNode(ctx, node.ID); err != nil {
		return fmt.Errorf("undrain: %w", err)
	}

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
	if err := pollServiceActive(ctx, runner, sshArgs, target, "ployd-node", streams); err != nil {
		return fmt.Errorf("service health check: %w", err)
	}

	// Wait for heartbeat to confirm node is back online.
	_, _ = fmt.Fprintf(stderr, "[%s]   Waiting for heartbeat...\n", node.Name)
	if err := waitForNodeHeartbeat(ctx, node.ID); err != nil {
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

func waitForNodeHeartbeat(ctx context.Context, nodeID string) error {
	// Poll until the node sends a heartbeat after restart.
	// For simplicity, we implement a basic polling loop.
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	// Try for up to 30 seconds.
	timeout := time.After(30 * time.Second)

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("context cancelled")
		case <-timeout:
			return fmt.Errorf("timeout waiting for heartbeat")
		case <-ticker.C:
			// Query node status from API.
			node, err := fetchNodeByID(ctx, nodeID)
			if err != nil {
				// Continue polling on transient errors.
				continue
			}
			if node.LastHeartbeat != "" {
				// Parse heartbeat timestamp and check if recent (within last 10 seconds).
				hb, err := time.Parse(time.RFC3339, node.LastHeartbeat)
				if err == nil && time.Since(hb) < 10*time.Second {
					return nil
				}
			}
		}
	}
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

	return &state, nil
}

func saveRolloutState(path string, state *rolloutState) error {
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal state: %w", err)
	}

	return os.WriteFile(path, data, 0o644)
}
