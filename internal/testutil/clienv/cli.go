package clienv

import (
	"bytes"
	"io"
	"strings"
	"testing"
)

// RunFn is the common signature for cmd/ploy CLI entry points: any function that
// takes argv-style arguments and writes user-facing output to a writer.
type RunFn func([]string, io.Writer) error

// RunExpectError invokes run with args and asserts that it returns a non-nil
// error whose message contains wantErrSubstr. Returns the writer contents so
// callers can make additional assertions on usage output.
func RunExpectError(t testing.TB, run RunFn, args []string, wantErrSubstr string) string {
	t.Helper()
	var buf bytes.Buffer
	err := run(args, &buf)
	if err == nil {
		t.Fatalf("expected error containing %q, got nil (output: %q)", wantErrSubstr, buf.String())
	}
	if wantErrSubstr != "" && !strings.Contains(err.Error(), wantErrSubstr) {
		t.Fatalf("expected error containing %q, got %q", wantErrSubstr, err.Error())
	}
	return buf.String()
}

// RunExpectOK invokes run with args and asserts that it returns no error.
// Returns the writer contents so callers can assert on stdout/stderr output.
func RunExpectOK(t testing.TB, run RunFn, args []string) string {
	t.Helper()
	var buf bytes.Buffer
	if err := run(args, &buf); err != nil {
		t.Fatalf("unexpected error: %v (output: %q)", err, buf.String())
	}
	return buf.String()
}

// RunHelp asserts that both `--help` and `-h` invocations of baseArgs succeed
// and emit output containing each wantContains substring.
func RunHelp(t testing.TB, run RunFn, baseArgs []string, wantContains ...string) {
	t.Helper()
	for _, flag := range []string{"--help", "-h"} {
		args := append(append([]string{}, baseArgs...), flag)
		out := RunExpectOK(t, run, args)
		for _, want := range wantContains {
			if !strings.Contains(out, want) {
				t.Fatalf("%v: expected output containing %q, got %q", args, want, out)
			}
		}
	}
}
