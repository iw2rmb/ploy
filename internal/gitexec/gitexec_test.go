package gitexec

import (
	"context"
	"errors"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestExecutePreservesRequestInputsAndCapturesOutput(t *testing.T) {
	repo := t.TempDir()
	if result, err := Execute(t.Context(), Request{Dir: repo, Args: []string{"init"}}); err != nil {
		t.Fatalf("git init: %v (stderr=%s)", err, result.Stderr)
	}

	t.Run("working directory", func(t *testing.T) {
		result, err := Execute(t.Context(), Request{Dir: repo, Args: []string{"rev-parse", "--show-toplevel"}})
		if err != nil {
			t.Fatalf("Execute() error = %v (stderr=%s)", err, result.Stderr)
		}
		want, err := filepath.EvalSymlinks(repo)
		if err != nil {
			t.Fatalf("EvalSymlinks(%q): %v", repo, err)
		}
		if got := strings.TrimSpace(string(result.Stdout)); got != filepath.Clean(want) {
			t.Fatalf("stdout = %q, want %q", got, filepath.Clean(want))
		}
		if len(result.Stderr) != 0 {
			t.Fatalf("stderr = %q, want empty", result.Stderr)
		}
	})

	t.Run("environment additions", func(t *testing.T) {
		result, err := Execute(t.Context(), Request{
			Dir: repo,
			Env: []string{
				"GIT_CONFIG_COUNT=1",
				"GIT_CONFIG_KEY_0=ploy.testValue",
				"GIT_CONFIG_VALUE_0=from-request",
			},
			Args: []string{"config", "--get", "ploy.testValue"},
		})
		if err != nil {
			t.Fatalf("Execute() error = %v (stderr=%s)", err, result.Stderr)
		}
		if got := strings.TrimSpace(string(result.Stdout)); got != "from-request" {
			t.Fatalf("stdout = %q, want from-request", got)
		}
	})

	t.Run("stdin", func(t *testing.T) {
		result, err := Execute(t.Context(), Request{Dir: repo, Args: []string{"hash-object", "--stdin"}, Stdin: []byte("payload")})
		if err != nil {
			t.Fatalf("Execute() error = %v (stderr=%s)", err, result.Stderr)
		}
		if got := strings.TrimSpace(string(result.Stdout)); got == "" {
			t.Fatal("stdout is empty, want object hash")
		}
	})
}

func TestExecuteReturnsRawExitErrorAndSeparateStderr(t *testing.T) {
	result, err := Execute(t.Context(), Request{Args: []string{"not-a-real-subcommand"}})
	if err == nil {
		t.Fatal("Execute() error = nil, want exit error")
	}
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("Execute() error type = %T, want *exec.ExitError", err)
	}
	if len(result.Stdout) != 0 {
		t.Fatalf("stdout = %q, want empty", result.Stdout)
	}
	if len(result.Stderr) == 0 {
		t.Fatal("stderr is empty, want Git diagnostic")
	}
}

func TestExecuteAlwaysDisablesInteractivePrompts(t *testing.T) {
	result, err := Execute(t.Context(), Request{
		Env: []string{"GIT_TERMINAL_PROMPT=1", "GIT_ASKPASS=custom-askpass"},
		Args: []string{
			"-c",
			`alias.print-prompt=!printf '%s|%s' "$GIT_TERMINAL_PROMPT" "$GIT_ASKPASS"`,
			"print-prompt",
		},
	})
	if err != nil {
		t.Fatalf("Execute() error = %v (stderr=%s)", err, result.Stderr)
	}
	if got := strings.TrimSpace(string(result.Stdout)); got != "0|echo" {
		t.Fatalf("prompt environment = %q, want 0|echo", got)
	}
}

func TestExecuteBoundsChildProcessCancellation(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("process group cancellation uses Unix signals")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, err := Execute(ctx, Request{Args: []string{"-c", "alias.wait=!sleep 5 & wait", "wait"}})
	if err == nil {
		t.Fatal("Execute() error = nil, want cancellation error")
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("Execute() elapsed = %s, want bounded cancellation", elapsed)
	}
}
