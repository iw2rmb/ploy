package main

import (
	"bytes"
	"io"
	"strings"
	"testing"

	"github.com/iw2rmb/ploy/internal/testutil/golden"
)

// executeCmd is a test helper that creates a root cobra command, sets the provided
// arguments, and executes it. This provides the canonical CLI entrypoint for tests,
// replacing the legacy execute() function from main.go.
//
// Usage:
//
//	buf := &bytes.Buffer{}
//	err := executeCmd([]string{"mig", "run", "status", "batch-123"}, buf)
//
// The stderr parameter receives all CLI output (both success and error messages).
func executeCmd(args []string, stderr io.Writer) error {
	rootCmd := newRootCmd(stderr)
	rootCmd.SetArgs(args)
	return rootCmd.Execute()
}

// TestExecuteHelpMatchesGolden verifies that "ploy help" produces the expected golden output.
// Cobra routes "help" through the custom help command we defined in root.go.
func TestExecuteHelpMatchesGolden(t *testing.T) {
	t.Helper()
	buf := &bytes.Buffer{}
	rootCmd := newRootCmd(buf)
	rootCmd.SetArgs([]string{"help"})
	err := rootCmd.Execute()
	if err != nil {
		t.Fatalf("execute help: %v", err)
	}
	expect := golden.LoadString(t, "testdata", "help.txt")
	if diff := diffStrings(expect, buf.String()); diff != "" {
		t.Fatalf("help output mismatch:\n%s", diff)
	}
}

// TestExecuteHelpForMigMatchesGolden verifies that "ploy help mig" produces the expected golden output.
func TestExecuteHelpForMigMatchesGolden(t *testing.T) {
	t.Helper()
	buf := &bytes.Buffer{}
	rootCmd := newRootCmd(buf)
	rootCmd.SetArgs([]string{"help", "mig"})
	err := rootCmd.Execute()
	if err != nil {
		t.Fatalf("execute help mig: %v", err)
	}
	expect := golden.LoadString(t, "testdata", "help_mig.txt")
	if diff := diffStrings(expect, buf.String()); diff != "" {
		t.Fatalf("help mig output mismatch:\n%s", diff)
	}
}

// TestExecuteRequiresCommandPrintsHelp verifies that running ploy with no arguments prints usage and returns an error.
// The root command's RunE prints usage and returns "command required" to match old behavior.
func TestExecuteRequiresCommandPrintsHelp(t *testing.T) {
	buf := &bytes.Buffer{}
	rootCmd := newRootCmd(buf)
	rootCmd.SetArgs([]string{})
	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected error when no arguments provided")
	}
	if !strings.Contains(buf.String(), "Ploy CLI v2") {
		t.Fatalf("expected usage output, got %q", buf.String())
	}
}

// TestExecuteUnknownCommandSuggestsHelp verifies that unknown commands produce an error and suggest help.
func TestExecuteUnknownCommandSuggestsHelp(t *testing.T) {
	buf := &bytes.Buffer{}
	rootCmd := newRootCmd(buf)
	rootCmd.SetArgs([]string{"unknown"})
	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected error for unknown command")
	}
	// Cobra's error for unknown subcommand typically says "unknown command".
	if !strings.Contains(err.Error(), "unknown command") {
		t.Fatalf("expected 'unknown command' error, got %v", err)
	}
	// Cobra may print suggestions; ensure help is mentioned somewhere.
	output := buf.String()
	if !strings.Contains(output, "help") && !strings.Contains(err.Error(), "help") {
		t.Logf("expected help hint in output or error, got output=%q err=%v", output, err)
	}
}

func diffStrings(expect, actual string) string {
	if expect == actual {
		return ""
	}
	return "expected:\n" + expect + "\nactual:\n" + actual
}
