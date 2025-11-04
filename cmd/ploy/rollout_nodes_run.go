package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/iw2rmb/ploy/internal/deploy"
)

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

type rolloutNodeOptions struct {
	User            string
	Port            int
	IdentityFile    string
	PloydBinaryPath string
	Stderr          io.Writer
	Logger          *slog.Logger
	Metrics         *RolloutMetrics
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
