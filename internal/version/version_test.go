package version

import "testing"

func TestDefaults(t *testing.T) {
	if Version == "" {
		t.Fatal("Version must be non-empty")
	}
	if Commit == "" {
		t.Fatal("Commit must be non-empty")
	}
	// BuiltAt may be empty in unit tests; it is injected at build time.
}
