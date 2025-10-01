package buildgate

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
)

type recordingESLintRunner struct {
	output commandOutput
	err    error

	lastDir  string
	lastEnv  []string
	lastName string
	lastArgs []string
	calls    int
}

// Run records the invocation and returns the preconfigured output.
func (r *recordingESLintRunner) Run(ctx context.Context, dir string, env []string, name string, args ...string) (commandOutput, error) {
	r.calls++
	r.lastDir = dir
	r.lastEnv = append([]string(nil), env...)
	r.lastName = name
	r.lastArgs = append([]string(nil), args...)
	return r.output, r.err
}

// TestESLintAdapterParsesDiagnostics confirms ESLint output is translated into structured failures.
func TestESLintAdapterParsesDiagnostics(t *testing.T) {
	t.Parallel()

	stdout := `[
        {
            "filePath": "src/app.js",
            "messages": [
                {"ruleId": "no-console", "message": "Unexpected console.log", "line": 10, "column": 5, "severity": 2},
                {"ruleId": null, "message": "Parsing error: Unexpected token <", "line": 2, "column": 8, "severity": 0, "fatal": true}
            ]
        },
        {
            "filePath": "src/utils/helpers.js",
            "messages": [
                {"ruleId": "eqeqeq", "message": "Expected '==='", "line": 14, "column": 7, "severity": 1}
            ]
        }
    ]`

	runner := &recordingESLintRunner{
		err:    errors.New("exit status 1"),
		output: commandOutput{stdout: stdout},
	}
	adapter := NewESLintAdapter("/workspace", withESLintCommandRunner(runner))

	options := map[string]string{
		"targets":        "src/app.js, src/utils/helpers.js\n", // ensure comma/newline parsing
		"config":         ".eslintrc.cjs",
		"rule_overrides": "no-debugger:ERROR no-alert:warning",
		"binary":         "npx eslint",
	}

	result, err := adapter.Run(context.Background(), StaticCheckRequest{Options: options})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if runner.calls != 1 {
		t.Fatalf("expected single invocation, got %d", runner.calls)
	}
	if runner.lastName != "npx" {
		t.Fatalf("expected binary override to use npx, got %q", runner.lastName)
	}
	expectedArgs := []string{
		"eslint",
		"--format", "json",
		"--no-color",
		"--no-error-on-unmatched-pattern",
		"--config", ".eslintrc.cjs",
		"--rule", "no-debugger:error",
		"--rule", "no-alert:warn",
		"src/app.js",
		"src/utils/helpers.js",
	}
	if !reflect.DeepEqual(runner.lastArgs, expectedArgs) {
		t.Fatalf("unexpected args: %#v", runner.lastArgs)
	}
	if runner.lastDir != "/workspace" {
		t.Fatalf("expected working directory propagated, got %q", runner.lastDir)
	}

	if len(result.Failures) != 3 {
		t.Fatalf("expected 3 failures, got %d", len(result.Failures))
	}

	first := result.Failures[0]
	if first.RuleID != "no-console" {
		t.Fatalf("unexpected rule id for first failure: %q", first.RuleID)
	}
	if first.Severity != "error" {
		t.Fatalf("expected first severity error, got %q", first.Severity)
	}
	if first.File != "src/app.js" {
		t.Fatalf("unexpected first file: %q", first.File)
	}
	if first.Line != 10 || first.Column != 5 {
		t.Fatalf("unexpected first position: %d:%d", first.Line, first.Column)
	}

	second := result.Failures[1]
	if second.RuleID != "eslint" {
		t.Fatalf("expected fallback rule id eslint, got %q", second.RuleID)
	}
	if second.Severity != "error" {
		t.Fatalf("expected fatal severity to map to error, got %q", second.Severity)
	}
	if !strings.Contains(second.Message, "Parsing error") {
		t.Fatalf("expected fatal message preserved, got %q", second.Message)
	}

	third := result.Failures[2]
	if third.RuleID != "eqeqeq" {
		t.Fatalf("unexpected third rule id: %q", third.RuleID)
	}
	if third.Severity != "warning" {
		t.Fatalf("expected third severity warning, got %q", third.Severity)
	}
}

// TestESLintAdapterErrorsWhenCommandFailsWithoutDiagnostics ensures errors bubble when no diagnostics are produced.
func TestESLintAdapterErrorsWhenCommandFailsWithoutDiagnostics(t *testing.T) {
	t.Parallel()

	runner := &recordingESLintRunner{err: errors.New("eslint: command failed")}
	adapter := NewESLintAdapter("/repo", withESLintCommandRunner(runner))

	if _, err := adapter.Run(context.Background(), StaticCheckRequest{}); err == nil {
		t.Fatal("expected error when ESLint exits without diagnostics")
	}
}

// TestESLintAdapterDefaultsTargets verifies a default target placeholder is used when none provided.
func TestESLintAdapterDefaultsTargets(t *testing.T) {
	t.Parallel()

	runner := &recordingESLintRunner{}
	adapter := NewESLintAdapter("/repo", withESLintCommandRunner(runner))

	if _, err := adapter.Run(context.Background(), StaticCheckRequest{Options: map[string]string{"binary": "eslint"}}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := []string{"--format", "json", "--no-color", "--no-error-on-unmatched-pattern", "."}
	if !reflect.DeepEqual(runner.lastArgs, expected) {
		t.Fatalf("unexpected default args: %#v", runner.lastArgs)
	}
}

// TestParseESLintRuleOverrides normalises rule override severities.
func TestParseESLintRuleOverrides(t *testing.T) {
	t.Parallel()

	overrides := parseESLintRuleOverrides("no-console:ERROR, no-alert:warning\n eqeqeq:off extra-rule")
	expected := []string{
		"no-console:error",
		"no-alert:warn",
		"eqeqeq:off",
		"extra-rule",
	}
	if !reflect.DeepEqual(overrides, expected) {
		t.Fatalf("unexpected overrides: %#v", overrides)
	}
}

// TestParseESLintDiagnosticsHandlesEmpty ensures no failures returned for empty stdout.
func TestParseESLintDiagnosticsHandlesEmpty(t *testing.T) {
	t.Parallel()

	failures, err := parseESLintDiagnostics("\n\n")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(failures) != 0 {
		t.Fatalf("expected no failures for empty stdout, got %d", len(failures))
	}
}
