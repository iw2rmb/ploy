package app

import (
	"bytes"
	"io"
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
//	err := executeCmd([]string{"mig", "run", "status", "run-123"}, buf)
//
// The stderr parameter receives all CLI output (both success and error messages).
func executeCmd(args []string, stderr io.Writer) error {
	rootCmd := NewRootCmd(stderr)
	rootCmd.SetArgs(args)
	return rootCmd.Execute()
}

// TestExecuteHelpMatchesGolden verifies that "ploy help" produces the expected golden output.
// Cobra routes "help" through the custom help command we defined in root.go.
func TestExecuteHelpMatchesGolden(t *testing.T) {
	buf := &bytes.Buffer{}
	rootCmd := NewRootCmd(buf)
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
	buf := &bytes.Buffer{}
	rootCmd := NewRootCmd(buf)
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

func diffStrings(expect, actual string) string {
	if expect == actual {
		return ""
	}
	return "expected:\n" + expect + "\nactual:\n" + actual
}
