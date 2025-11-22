package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// TestAutocompleteArtifactsUpToDate validates that the checked-in completion files
// match what Cobra generates from the current command tree.
// This ensures that shell completions remain in sync with the cobra tree.
// If this test fails, run `make build && go run tools/gencomp/main.go` to regenerate completions.
func TestAutocompleteArtifactsUpToDate(t *testing.T) {
	t.Helper()

	// Generate completions directly from the Cobra command tree.
	// We construct the root command in-process to avoid depending on the binary.
	var stderr bytes.Buffer
	rootCmd := newRootCmd(&stderr)

	// Cobra provides built-in completion generation methods.
	// We call them directly to generate the expected output.
	generated := make(map[string]string)

	// Generate bash completion using Cobra's GenBashCompletionV2.
	// This produces the modern bash-completion format with dynamic completion support.
	var bashBuf bytes.Buffer
	if err := rootCmd.GenBashCompletionV2(&bashBuf, true); err != nil {
		t.Fatalf("failed to generate bash completion: %v", err)
	}
	generated["bash"] = bashBuf.String()

	// Generate zsh completion using Cobra's GenZshCompletion.
	var zshBuf bytes.Buffer
	if err := rootCmd.GenZshCompletion(&zshBuf); err != nil {
		t.Fatalf("failed to generate zsh completion: %v", err)
	}
	generated["zsh"] = zshBuf.String()

	// Generate fish completion using Cobra's GenFishCompletion.
	var fishBuf bytes.Buffer
	if err := rootCmd.GenFishCompletion(&fishBuf, true); err != nil {
		t.Fatalf("failed to generate fish completion: %v", err)
	}
	generated["fish"] = fishBuf.String()

	// Define the shells and filenames to check.
	cases := []struct {
		shell    string
		filename string
	}{
		{shell: "bash", filename: "ploy.bash"},
		{shell: "zsh", filename: "ploy.zsh"},
		{shell: "fish", filename: "ploy.fish"},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.shell, func(t *testing.T) {
			t.Helper()
			// Read the checked-in completion file from cmd/ploy/autocomplete/.
			data, err := os.ReadFile(filepath.Join("autocomplete", tc.filename))
			if err != nil {
				t.Fatalf("read %s completion: %v", tc.shell, err)
			}
			// Verify that the checked-in file matches the generated output.
			expected, ok := generated[tc.shell]
			if !ok {
				t.Fatalf("missing generated completions for %s", tc.shell)
			}
			if diff := diffStrings(expected, string(data)); diff != "" {
				t.Fatalf("completion mismatch for %s:\n%s\nRun 'make build && go run tools/gencomp/main.go' to regenerate completions.", tc.shell, diff)
			}
		})
	}
}
