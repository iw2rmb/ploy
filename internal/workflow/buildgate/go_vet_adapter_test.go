package buildgate

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

type recordingCommandRunner struct {
	output commandOutput
	err    error

	lastDir  string
	lastEnv  []string
	lastName string
	lastArgs []string
	calls    int
}

func (r *recordingCommandRunner) Run(ctx context.Context, dir string, env []string, name string, args ...string) (commandOutput, error) {
	r.calls++
	r.lastDir = dir
	r.lastEnv = append([]string(nil), env...)
	r.lastName = name
	r.lastArgs = append([]string(nil), args...)
	return r.output, r.err
}

func TestGoVetAdapterParsesFailures(t *testing.T) {
	runner := &recordingCommandRunner{
		output: commandOutput{stderr: "# example.com/vettest\n./main.go:6:1: fmt.Printf format %d has arg \"oops\" of wrong type string\n"},
		err:    errors.New("exit status 2"),
	}
	adapter := NewGoVetAdapter("/workspace/repo", withGoVetCommandRunner(runner))

	result, err := adapter.Run(context.Background(), StaticCheckRequest{})
	if err != nil {
		t.Fatalf("go vet run: %v", err)
	}

	if runner.calls != 1 {
		t.Fatalf("expected runner invoked once, got %d", runner.calls)
	}
	if runner.lastName != "go" {
		t.Fatalf("expected go binary, got %q", runner.lastName)
	}
	expectedArgs := []string{"vet", "./..."}
	if !reflect.DeepEqual(runner.lastArgs, expectedArgs) {
		t.Fatalf("unexpected args: %#v", runner.lastArgs)
	}
	if runner.lastDir != "/workspace/repo" {
		t.Fatalf("expected working dir /workspace/repo, got %q", runner.lastDir)
	}

	if len(result.Failures) != 1 {
		t.Fatalf("expected 1 failure, got %d", len(result.Failures))
	}
	failure := result.Failures[0]
	if failure.RuleID != "govet" {
		t.Fatalf("expected govet rule id, got %q", failure.RuleID)
	}
	if failure.File != "main.go" {
		t.Fatalf("expected cleaned file path main.go, got %q", failure.File)
	}
	if failure.Line != 6 || failure.Column != 1 {
		t.Fatalf("expected position 6:1, got %d:%d", failure.Line, failure.Column)
	}
	if failure.Severity != "error" {
		t.Fatalf("expected severity error, got %q", failure.Severity)
	}
	if failure.Message != "fmt.Printf format %d has arg \"oops\" of wrong type string" {
		t.Fatalf("unexpected message: %q", failure.Message)
	}
}

func TestGoVetAdapterSuccessNoFailures(t *testing.T) {
	runner := &recordingCommandRunner{}
	adapter := NewGoVetAdapter("", withGoVetCommandRunner(runner))

	result, err := adapter.Run(context.Background(), StaticCheckRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Failures) != 0 {
		t.Fatalf("expected no failures, got %d", len(result.Failures))
	}
}

func TestGoVetAdapterReturnsErrorWhenNoDiagnostics(t *testing.T) {
	runner := &recordingCommandRunner{err: errors.New("go: not found")}
	adapter := NewGoVetAdapter("", withGoVetCommandRunner(runner))

	_, err := adapter.Run(context.Background(), StaticCheckRequest{})
	if err == nil {
		t.Fatal("expected error when command fails without diagnostics")
	}
}

func TestGoVetAdapterPackagesAndTags(t *testing.T) {
	runner := &recordingCommandRunner{}
	adapter := NewGoVetAdapter("/workspace/project", withGoVetCommandRunner(runner))

	options := map[string]string{
		"packages": "./cmd ./internal,./pkg\n", // includes whitespace and comma separators
		"tags":     "integration unit",
	}

	_, err := adapter.Run(context.Background(), StaticCheckRequest{Options: options})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expectedArgs := []string{"vet", "-tags", "integration unit", "./cmd", "./internal", "./pkg"}
	if !reflect.DeepEqual(runner.lastArgs, expectedArgs) {
		t.Fatalf("unexpected args: %#v", runner.lastArgs)
	}
	if runner.lastDir != "/workspace/project" {
		t.Fatalf("expected working dir propagated, got %q", runner.lastDir)
	}
}
