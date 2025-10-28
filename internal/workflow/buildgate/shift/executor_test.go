package shift_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"testing"

	"github.com/iw2rmb/ploy/internal/workflow/buildgate"
	"github.com/iw2rmb/ploy/internal/workflow/buildgate/shift"
)

type fakeRunner struct {
	cmd    []string
	env    map[string]string
	dir    string
	result shift.CommandResult
	err    error
}

func (f *fakeRunner) Run(ctx context.Context, cmd []string, env map[string]string, dir string) (shift.CommandResult, error) {
	_ = ctx
	f.cmd = append([]string{}, cmd...)
	f.env = env
	f.dir = dir
	return f.result, f.err
}

func TestExecutorSuccess(t *testing.T) {
	t.Helper()
	runner := &fakeRunner{
		result: shift.CommandResult{
			Stdout:   "shift succeeded\n",
			Stderr:   "",
			ExitCode: 0,
		},
	}

	exec, err := shift.NewExecutor(shift.Options{Runner: runner, Binary: "shift"})
	if err != nil {
		t.Fatalf("NewExecutor error: %v", err)
	}

	spec := buildgate.SandboxSpec{
		Workspace: "/tmp/workspace",
		Env: map[string]string{
			"PLOY_SHIFT_PROFILE": "gradle-linux",
		},
	}

	result, execErr := exec.Execute(context.Background(), spec)
	if execErr != nil {
		t.Fatalf("Execute returned error: %v", execErr)
	}
	if len(runner.cmd) == 0 {
		t.Fatalf("expected command invocation")
	}
	if runner.cmd[0] != "shift" {
		t.Fatalf("expected shift binary, got %q", runner.cmd[0])
	}
	if !result.Success {
		t.Fatalf("expected success true")
	}
	if runner.dir != spec.Workspace {
		t.Fatalf("expected working directory %q, got %q", spec.Workspace, runner.dir)
	}
	if profile := runner.env["PLOY_SHIFT_PROFILE"]; profile != "gradle-linux" {
		t.Fatalf("expected profile env propagated, got %q", profile)
	}
	digest := sha256.Sum256([]byte(runner.result.Stdout + runner.result.Stderr))
	expected := "sha256:" + hex.EncodeToString(digest[:])
	if result.LogDigest != expected {
		t.Fatalf("digest = %q, want %q", result.LogDigest, expected)
	}
}

func TestExecutorFailureExitCode(t *testing.T) {
	t.Helper()
	runner := &fakeRunner{
		result: shift.CommandResult{
			Stdout:   "",
			Stderr:   "tests failed",
			ExitCode: 17,
		},
	}

	exec, err := shift.NewExecutor(shift.Options{Runner: runner, Binary: "shift"})
	if err != nil {
		t.Fatalf("NewExecutor error: %v", err)
	}

	result, execErr := exec.Execute(context.Background(), buildgate.SandboxSpec{Workspace: "/repo"})
	if execErr != nil {
		t.Fatalf("Execute unexpected error: %v", execErr)
	}
	if result.Success {
		t.Fatalf("expected success false")
	}
	if result.FailureReason != "exit_code" {
		t.Fatalf("unexpected failure reason: %q", result.FailureReason)
	}
	if !strings.Contains(result.FailureDetail, "17") {
		t.Fatalf("expected failure detail to mention exit code, got %q", result.FailureDetail)
	}
}

func TestExecutorPropagatesRunnerError(t *testing.T) {
	t.Helper()
	runner := &fakeRunner{err: errors.New("failed to start shift")}

	exec, err := shift.NewExecutor(shift.Options{Runner: runner, Binary: "shift"})
	if err != nil {
		t.Fatalf("NewExecutor error: %v", err)
	}

	_, execErr := exec.Execute(context.Background(), buildgate.SandboxSpec{Workspace: "/repo"})
	if execErr == nil {
		t.Fatal("expected error from runner")
	}
	if !strings.Contains(execErr.Error(), "failed to start shift") {
		t.Fatalf("unexpected error: %v", execErr)
	}
}
