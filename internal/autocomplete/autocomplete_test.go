package autocomplete

import "testing"

func TestGenerateAllReturnsShells(t *testing.T) {
	t.Helper()

	m := GenerateAll()
	for _, shell := range []string{"bash", "zsh", "fish"} {
		content, ok := m[shell]
		if !ok {
			t.Fatalf("missing %s completion", shell)
		}
		if len(content) == 0 {
			t.Fatalf("%s completion is empty", shell)
		}
	}
}
