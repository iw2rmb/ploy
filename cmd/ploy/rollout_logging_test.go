package main

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/iw2rmb/ploy/internal/deploy"
)

func TestRolloutLoggingServerSteps(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping rollout logging integration test in short mode")
	}

	var logBuf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logBuf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	metrics := NewRolloutMetrics()

	// Create a recording runner to track command execution.
	recorder := &recordingRunner{
		results: map[string]error{
			"scp":                                   nil,
			"ssh install":                           nil,
			"ssh systemctl restart ployd":           nil,
			"ssh systemctl is-active --quiet ployd": nil,
			"ssh ss -tlnp | grep :8443 || netstat -tlnp | grep :8443": nil,
		},
	}
	oldRunner := rolloutRunner
	rolloutRunner = recorder
	defer func() { rolloutRunner = oldRunner }()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	opts := rolloutServerOptions{
		Address:         "10.0.0.1",
		User:            "testuser",
		Port:            22,
		IdentityFile:    "/tmp/fake-key",
		PloydBinaryPath: "/tmp/fake-ployd",
		Stdout:          &bytes.Buffer{},
		Stderr:          &bytes.Buffer{},
		Logger:          logger,
		Metrics:         metrics,
	}

	err := executeRolloutServer(ctx, opts)
	if err != nil {
		t.Fatalf("executeRolloutServer failed: %v", err)
	}

	// Verify structured logs were emitted.
	logOutput := logBuf.String()
	expectedSteps := []string{
		"upload_binary",
		"install_binary",
		"restart_service",
		"health_check",
		"verify_port",
	}

	for _, step := range expectedSteps {
		if !strings.Contains(logOutput, step) {
			t.Errorf("expected log output to contain step %q, got: %s", step, logOutput)
		}
	}

	// Verify log contains status fields.
	if !strings.Contains(logOutput, "started") {
		t.Errorf("expected log output to contain 'started' status")
	}
	if !strings.Contains(logOutput, "completed") {
		t.Errorf("expected log output to contain 'completed' status")
	}

	// Verify metrics were recorded.
	var summaryBuf bytes.Buffer
	metrics.PrintSummary(&summaryBuf)
	summary := summaryBuf.String()
	if !strings.Contains(summary, "Steps:") {
		t.Errorf("expected metrics summary to contain 'Steps:', got: %s", summary)
	}
	if !strings.Contains(summary, "Duration:") {
		t.Errorf("expected metrics summary to contain 'Duration:', got: %s", summary)
	}
}

func TestRolloutMetricsTracking(t *testing.T) {
	metrics := NewRolloutMetrics()

	// Record some steps.
	metrics.RecordStep("upload_binary", "completed")
	metrics.RecordStep("install_binary", "completed")
	metrics.RecordStep("restart_service", "failed")
	metrics.RecordStep("restart_service", "completed")

	// Record some nodes.
	metrics.RecordNode(true)
	metrics.RecordNode(true)
	metrics.RecordNode(false)

	// Record some attempts.
	metrics.RecordAttempt("service_active_poll")
	metrics.RecordAttempt("service_active_poll")
	metrics.RecordAttempt("node_heartbeat_poll")

	// Verify summary output.
	var summaryBuf bytes.Buffer
	metrics.PrintSummary(&summaryBuf)
	summary := summaryBuf.String()

	expectedStrings := []string{
		"Nodes: 3 total, 2 succeeded, 1 failed",
		"Steps: 4 total, 3 succeeded, 1 failed",
		"Attempts: 3 total",
		"Duration:",
	}

	for _, expected := range expectedStrings {
		if !strings.Contains(summary, expected) {
			t.Errorf("expected summary to contain %q, got: %s", expected, summary)
		}
	}
}

// recordingRunner records all Run calls for test verification.
type recordingRunner struct {
	calls   []string
	results map[string]error
}

func (r *recordingRunner) Run(ctx context.Context, cmd string, args []string, stdin io.Reader, streams deploy.IOStreams) error {
	key := cmd
	if len(args) > 0 {
		argsStr := strings.Join(args, " ")
		// Simplify key matching by using command type.
		switch {
		case strings.Contains(argsStr, "install"):
			key = "ssh install"
		case strings.Contains(argsStr, "restart"):
			key = "ssh " + strings.Join(args[len(args)-2:], " ")
		case strings.Contains(argsStr, "is-active"):
			key = "ssh " + strings.Join(args[len(args)-3:], " ")
		case strings.Contains(argsStr, "grep"):
			key = "ssh " + args[len(args)-1]
		}
	}
	r.calls = append(r.calls, key)
	if result, ok := r.results[key]; ok {
		return result
	}
	return nil
}
