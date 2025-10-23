package cmd_test

import (
	"os"
	"sort"
	"testing"
)

func TestOnlyWorkflowBinaryRemains(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read cmd directory: %v", err)
	}

	var dirs []string
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		if len(name) == 0 || name[0] == '.' {
			continue
		}
		if name == "testdata" {
			continue
		}
		dirs = append(dirs, name)
	}

	sort.Strings(dirs)
	expected := []string{"ploy", "ployd"}
	if len(dirs) != len(expected) {
		t.Fatalf("unexpected command directories: got %v, want %v", dirs, expected)
	}
	for i, name := range dirs {
		if name != expected[i] {
			t.Fatalf("unexpected command directory at position %d: got %q, want %q", i, name, expected[i])
		}
	}
}
