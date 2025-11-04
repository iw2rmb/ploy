package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"time"

	"github.com/iw2rmb/ploy/internal/deploy"
)

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
