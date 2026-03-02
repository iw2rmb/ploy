package deploy

import (
	"context"
	"io"
	"strings"
	"testing"
)

// recordingRunner records command invocations without executing them.
type recordingRunner struct{ calls [][]string }

func (r *recordingRunner) Run(_ context.Context, command string, args []string, _ io.Reader, _ IOStreams) error {
	entry := append([]string{command}, append([]string(nil), args...)...)
	r.calls = append(r.calls, entry)
	return nil
}

func TestShellQuote(t *testing.T) {
	cases := map[string]string{
		"":      "''",
		"plain": "'plain'",
		"a'b":   `'a'"'"'b'`,
	}
	for in, want := range cases {
		if got := shellQuote(in); got != want {
			t.Fatalf("shellQuote(%q)=%q want %q", in, got, want)
		}
	}
}

func TestRenderBootstrapScriptMergesEnv(t *testing.T) {
	script := renderBootstrapScript(map[string]string{"FOO": "bar", "PLOY_BOOTSTRAP_VERSION": "x"})
	if !strings.Contains(script, "export FOO='bar'") {
		t.Fatalf("expected FOO export in script: %q", script)
	}
	// Default export should remain present even if overridden in input.
	if !strings.Contains(script, "PLOY_BOOTSTRAP_VERSION") {
		t.Fatalf("expected bootstrap version export in script: %q", script)
	}
}
