package buildgate

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
)

type recordingErrorProneRunner struct {
	output commandOutput
	err    error

	lastDir  string
	lastEnv  []string
	lastName string
	lastArgs []string
	calls    int
}

func (r *recordingErrorProneRunner) Run(ctx context.Context, dir string, env []string, name string, args ...string) (commandOutput, error) {
	r.calls++
	r.lastDir = dir
	r.lastEnv = append([]string(nil), env...)
	r.lastName = name
	r.lastArgs = append([]string(nil), args...)
	return r.output, r.err
}

// TestErrorProneAdapterParsesDiagnostics confirms Error Prone output is translated into structured failures.
func TestErrorProneAdapterParsesDiagnostics(t *testing.T) {
	runner := &recordingErrorProneRunner{
		err: errors.New("exit status 1"),
		output: commandOutput{stderr: `/workspace/src/Main.java:12: warning: [DeadException] Exception created but not thrown
	new Exception();
	^
/workspace/src/Main.java:20:7: error: [NullableDereference] dereference of @Nullable value
	value.toString();
	      ^
`},
	}
	adapter := NewErrorProneAdapter("/workspace", withErrorProneCommandRunner(runner))

	options := map[string]string{
		"targets":   "src/Main.java src/Util.java",
		"classpath": "build/classes:lib/*",
		"flags":     "-XepDisableAllChecks -Xep:NullableDereference=ERROR",
	}

	result, err := adapter.Run(context.Background(), StaticCheckRequest{Options: options})
	if err != nil {
		t.Fatalf("unexpected run error: %v", err)
	}

	if runner.calls != 1 {
		t.Fatalf("expected single invocation, got %d", runner.calls)
	}
	if runner.lastName != "javac" {
		t.Fatalf("expected javac binary, got %q", runner.lastName)
	}
	expectedArgs := []string{
		"-Xplugin:ErrorProne",
		"-classpath", "build/classes:lib/*",
		"-XepDisableAllChecks",
		"-Xep:NullableDereference=ERROR",
		"src/Main.java",
		"src/Util.java",
	}
	if !reflect.DeepEqual(runner.lastArgs, expectedArgs) {
		t.Fatalf("unexpected args: %#v", runner.lastArgs)
	}
	if runner.lastDir != "/workspace" {
		t.Fatalf("expected working directory propagated, got %q", runner.lastDir)
	}

	if len(result.Failures) != 2 {
		t.Fatalf("expected 2 failures, got %d", len(result.Failures))
	}
	first := result.Failures[0]
	if first.RuleID != "DeadException" {
		t.Fatalf("unexpected rule id: %q", first.RuleID)
	}
	if first.Severity != "warning" {
		t.Fatalf("expected warning severity, got %q", first.Severity)
	}
	if first.Line != 12 || first.Column != 0 {
		t.Fatalf("expected line 12 column 0, got %d:%d", first.Line, first.Column)
	}
	if !strings.HasSuffix(first.File, "src/Main.java") {
		t.Fatalf("unexpected file path: %q", first.File)
	}

	second := result.Failures[1]
	if second.RuleID != "NullableDereference" {
		t.Fatalf("unexpected rule id: %q", second.RuleID)
	}
	if second.Severity != "error" {
		t.Fatalf("expected error severity, got %q", second.Severity)
	}
	if second.Line != 20 || second.Column != 7 {
		t.Fatalf("expected line 20 column 7, got %d:%d", second.Line, second.Column)
	}
}

// TestErrorProneAdapterErrorsWhenCommandFailsWithoutDiagnostics verifies command errors propagate when no output exists.
func TestErrorProneAdapterErrorsWhenCommandFailsWithoutDiagnostics(t *testing.T) {
	runner := &recordingErrorProneRunner{err: errors.New("javac: not found")}
	adapter := NewErrorProneAdapter("/src", withErrorProneCommandRunner(runner))

	_, err := adapter.Run(context.Background(), StaticCheckRequest{Options: map[string]string{"targets": "./src/Main.java"}})
	if err == nil {
		t.Fatal("expected error when command fails without diagnostics")
	}
}

// TestErrorProneAdapterDefaultsTargets ensures a default target placeholder is supplied when none provided.
func TestErrorProneAdapterDefaultsTargets(t *testing.T) {
	runner := &recordingErrorProneRunner{}
	adapter := NewErrorProneAdapter("/workspace", withErrorProneCommandRunner(runner))

	if _, err := adapter.Run(context.Background(), StaticCheckRequest{}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expectedArgs := []string{"-Xplugin:ErrorProne", "."}
	if !reflect.DeepEqual(runner.lastArgs, expectedArgs) {
		t.Fatalf("unexpected default args: %#v", runner.lastArgs)
	}
}
