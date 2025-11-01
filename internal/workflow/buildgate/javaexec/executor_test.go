package javaexec

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/iw2rmb/ploy/internal/workflow/buildgate"
)

type fakeRunner struct {
	lastCmd []string
	lastDir string
	result  CommandResult
	err     error
}

func (f *fakeRunner) Run(ctx context.Context, cmd []string, env map[string]string, dir string) (CommandResult, error) {
	// Simulate a tiny run time so duration > 0 occasionally
	time.Sleep(2 * time.Millisecond)
	f.lastCmd = append([]string(nil), cmd...)
	f.lastDir = dir
	return f.result, f.err
}

func TestJavaExec_GradleWrapperSuccess(t *testing.T) {
	ws := t.TempDir()
	// Create a gradle wrapper file
	if err := os.WriteFile(filepath.Join(ws, "gradlew"), []byte("#!/bin/sh"), 0o755); err != nil {
		t.Fatalf("write gradlew: %v", err)
	}

	fr := &fakeRunner{result: CommandResult{Stdout: "ok", Stderr: "", ExitCode: 0}}
	exec, err := NewExecutor(Options{Runner: fr})
	if err != nil {
		t.Fatalf("NewExecutor() error: %v", err)
	}
	res, err := exec.Execute(context.Background(), buildgate.SandboxSpec{Workspace: ws})
	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected success, got failure: %+v", res)
	}
	if fr.lastDir != ws {
		t.Fatalf("expected run in workspace, got %s", fr.lastDir)
	}
	if len(fr.lastCmd) == 0 || fr.lastCmd[0] != "./gradlew" {
		t.Fatalf("expected gradle wrapper command, got %v", fr.lastCmd)
	}
	if res.LogDigest == "" {
		t.Fatalf("expected log digest")
	}
}

func TestJavaExec_MavenWrapperFallback(t *testing.T) {
	ws := t.TempDir()
	// Only mvnw present
	if err := os.WriteFile(filepath.Join(ws, "mvnw"), []byte("#!/bin/sh"), 0o755); err != nil {
		t.Fatalf("write mvnw: %v", err)
	}
	fr := &fakeRunner{result: CommandResult{Stdout: "", Stderr: "", ExitCode: 0}}
	exec, err := NewExecutor(Options{Runner: fr})
	if err != nil {
		t.Fatalf("NewExecutor() error: %v", err)
	}
	res, err := exec.Execute(context.Background(), buildgate.SandboxSpec{Workspace: ws})
	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected success, got failure: %+v", res)
	}
	if len(fr.lastCmd) == 0 || fr.lastCmd[0] != "./mvnw" {
		t.Fatalf("expected mvn wrapper command, got %v", fr.lastCmd)
	}
}

func TestJavaExec_DockerFallbackUsesImageEnv(t *testing.T) {
	ws := t.TempDir()
	// No wrappers present
	t.Setenv("PLOY_BUILDGATE_JAVA_IMAGE", "example/mvn:17")

	fr := &fakeRunner{result: CommandResult{Stdout: "", Stderr: "pulling", ExitCode: 0}}
	exec, err := NewExecutor(Options{Runner: fr})
	if err != nil {
		t.Fatalf("NewExecutor() error: %v", err)
	}
	res, err := exec.Execute(context.Background(), buildgate.SandboxSpec{Workspace: ws})
	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected success, got failure: %+v", res)
	}
	if len(fr.lastCmd) < 2 || fr.lastCmd[0] != "docker" {
		t.Fatalf("expected docker fallback, got %v", fr.lastCmd)
	}
	// The image should appear in the args
	found := false
	for _, a := range fr.lastCmd {
		if a == "example/mvn:17" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected image arg in command, got %v", fr.lastCmd)
	}
}

func TestJavaExec_FailureMapping(t *testing.T) {
	ws := t.TempDir()
	if err := os.WriteFile(filepath.Join(ws, "gradlew"), []byte("#!/bin/sh"), 0o755); err != nil {
		t.Fatalf("write gradlew: %v", err)
	}
	fr := &fakeRunner{result: CommandResult{Stdout: "", Stderr: "Tests failed", ExitCode: 1}}
	exec, err := NewExecutor(Options{Runner: fr})
	if err != nil {
		t.Fatalf("NewExecutor() error: %v", err)
	}
	res, err := exec.Execute(context.Background(), buildgate.SandboxSpec{Workspace: ws})
	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}
	if res.Success {
		t.Fatalf("expected failure, got success")
	}
	if !strings.Contains(res.FailureDetail, "Tests failed") {
		t.Fatalf("expected failure detail to include stderr, got: %s", res.FailureDetail)
	}
}

func TestJavaExec_WorkspaceRequired(t *testing.T) {
	exec, err := NewExecutor(Options{})
	if err != nil {
		t.Fatalf("NewExecutor() error: %v", err)
	}
	if _, err := exec.Execute(context.Background(), buildgate.SandboxSpec{}); err == nil {
		t.Fatalf("expected error for empty workspace")
	}
}
