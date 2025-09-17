package recipes

import (
	"bytes"
	"errors"
	"io"
	"os"
	"sort"
	"strings"
	"testing"
)

func captureStreams(t *testing.T, fn func()) (stdout, stderr string) {
	t.Helper()

	oldOut := os.Stdout
	oldErr := os.Stderr

	outR, outW, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe stdout: %v", err)
	}
	errR, errW, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe stderr: %v", err)
	}

	os.Stdout = outW
	os.Stderr = errW

	outCh := make(chan string)
	errCh := make(chan string)

	go func() {
		var buf bytes.Buffer
		_, _ = io.Copy(&buf, outR)
		_ = outR.Close()
		outCh <- buf.String()
	}()

	go func() {
		var buf bytes.Buffer
		_, _ = io.Copy(&buf, errR)
		_ = errR.Close()
		errCh <- buf.String()
	}()

	fn()

	if err := outW.Close(); err != nil {
		t.Fatalf("close stdout writer: %v", err)
	}
	if err := errW.Close(); err != nil {
		t.Fatalf("close stderr writer: %v", err)
	}

	os.Stdout = oldOut
	os.Stderr = oldErr

	return <-outCh, <-errCh
}

func TestHelpSystemShowGeneralHelp(t *testing.T) {
	hs := NewHelpSystem()

	stdout, _ := captureStreams(t, func() {
		if err := hs.ShowHelp(""); err != nil {
			t.Fatalf("ShowHelp general: %v", err)
		}
	})

	if !strings.Contains(stdout, "Ploy ARF Recipe Management") {
		t.Fatalf("general help missing header: %s", stdout)
	}
	if !strings.Contains(stdout, "Recipe Management") {
		t.Fatalf("general help missing category: %s", stdout)
	}
}

func TestHelpSystemShowSpecificHelp(t *testing.T) {
	hs := NewHelpSystem()

	stdout, _ := captureStreams(t, func() {
		if err := hs.ShowHelp("compose"); err != nil {
			t.Fatalf("ShowHelp compose: %v", err)
		}
	})

	if !strings.Contains(stdout, "Command: compose") {
		t.Fatalf("compose help missing command header: %s", stdout)
	}
	if !strings.Contains(stdout, "Execute recipes in parallel") {
		t.Fatalf("compose help missing example description: %s", stdout)
	}
}

func TestHelpSystemUnknownCommand(t *testing.T) {
	hs := NewHelpSystem()
	err := hs.ShowHelp("unknown-cmd")
	if err == nil {
		t.Fatalf("expected CLI error for unknown command")
	}

	cliErr, ok := err.(*CLIError)
	if !ok {
		t.Fatalf("expected CLIError, got %T", err)
	}
	if !strings.Contains(cliErr.Suggestion, "ploy recipe --help") {
		t.Fatalf("unexpected suggestion: %s", cliErr.Suggestion)
	}
}

func TestHelpSystemGetAvailableHelpSorted(t *testing.T) {
	hs := NewHelpSystem()
	topics := hs.GetAvailableHelp()
	if len(topics) == 0 {
		t.Fatalf("expected help topics")
	}

	sorted := append([]string(nil), topics...)
	sort.Strings(sorted)
	for i := range topics {
		if topics[i] != sorted[i] {
			t.Fatalf("topics not sorted at index %d: %s vs %s", i, topics[i], sorted[i])
		}
	}
}

func TestPrintErrorDetailed(t *testing.T) {
	t.Setenv("PLOY_VERBOSE", "true")
	err := NewCLIError("failed to import", 2).
		WithSuggestion("Check archive path and permissions").
		WithCause(errors.New("permission denied")).
		WithUsage()

	stdout, stderr := captureStreams(t, func() {
		PrintError(err)
	})

	if !strings.Contains(stderr, "failed to import") {
		t.Fatalf("stderr missing error message: %s", stderr)
	}
	if !strings.Contains(stderr, "Suggestion") {
		t.Fatalf("stderr missing suggestion: %s", stderr)
	}
	if !strings.Contains(stderr, "Details") {
		t.Fatalf("stderr missing details: %s", stderr)
	}
	if !strings.Contains(stdout, "Usage: ploy recipe") {
		t.Fatalf("stdout missing usage output: %s", stdout)
	}
}

func TestPrintSuccessWarningInfo(t *testing.T) {
	stdout, _ := captureStreams(t, func() {
		PrintSuccess("recipe uploaded")
		PrintWarning("overwrite in progress")
		PrintInfo("fetching recipes")
	})

	if !strings.Contains(stdout, "✅ recipe uploaded") {
		t.Fatalf("missing success output: %s", stdout)
	}
	if !strings.Contains(stdout, "⚠️  Warning: overwrite in progress") {
		t.Fatalf("missing warning output: %s", stdout)
	}
	if !strings.Contains(stdout, "ℹ️  fetching recipes") {
		t.Fatalf("missing info output: %s", stdout)
	}
}

func TestValidationHelpers(t *testing.T) {
	if err := ValidateRecipeID("java11-17"); err != nil {
		t.Fatalf("valid recipe id flagged: %v", err)
	}
	if err := ValidateRecipeID(" "); err == nil {
		t.Fatalf("expected validation error for blank id")
	}

	tmpFile, err := os.CreateTemp(t.TempDir(), "recipe-*.yaml")
	if err != nil {
		t.Fatalf("create temp: %v", err)
	}
	_ = tmpFile.Close()

	if err := ValidateFilePath(tmpFile.Name()); err != nil {
		t.Fatalf("valid file path flagged: %v", err)
	}
	if err := ValidateFilePath("/nonexistent/path.yaml"); err == nil {
		t.Fatalf("expected error for missing file")
	}

	if err := ValidateOutputFormat("json"); err != nil {
		t.Fatalf("valid format flagged: %v", err)
	}
	if err := ValidateOutputFormat("xml"); err == nil {
		t.Fatalf("expected error for invalid format")
	}

	if err := ValidateRequired("recipe", "value", "Recipe"); err != nil {
		t.Fatalf("valid required value flagged: %v", err)
	}
	if err := ValidateRequired("recipe", "", "Recipe"); err == nil {
		t.Fatalf("expected error for missing required value")
	}
}

func TestConfirmAction(t *testing.T) {
	if !ConfirmAction("delete", true) {
		t.Fatalf("force=true should bypass confirmation")
	}

	oldStdin := os.Stdin
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe stdin: %v", err)
	}
	os.Stdin = r
	_, _ = w.WriteString("y\n")
	_ = w.Close()

	stdout, _ := captureStreams(t, func() {
		if !ConfirmAction("delete", false) {
			t.Fatalf("expected confirmation to return true")
		}
	})
	os.Stdin = oldStdin

	if !strings.Contains(stdout, "Are you sure you want to delete?") {
		t.Fatalf("prompt not shown to user: %s", stdout)
	}
}
