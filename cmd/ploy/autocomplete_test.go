package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/iw2rmb/ploy/internal/autocomplete"
)

func TestAutocompleteArtifactsUpToDate(t *testing.T) {
	t.Helper()
	generated := autocomplete.GenerateAll()
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
			data, err := os.ReadFile(filepath.Join("autocomplete", tc.filename))
			if err != nil {
				t.Fatalf("read %s completion: %v", tc.shell, err)
			}
			expected, ok := generated[tc.shell]
			if !ok {
				t.Fatalf("missing generated completions for %s", tc.shell)
			}
			if diff := diffStrings(expected, string(data)); diff != "" {
				t.Fatalf("completion mismatch for %s:\n%s", tc.shell, diff)
			}
		})
	}
}
