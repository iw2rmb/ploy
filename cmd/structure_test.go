package cmd_test

import (
	"os"
	"os/exec"
	"sort"
	"strings"
	"testing"
)

// TestNoCircularDependencies verifies that the import graph remains acyclic.
// Circular dependencies prevent compilation and indicate architectural issues.
// This test runs `go list` to enumerate all packages and detect import cycles.
func TestNoCircularDependencies(t *testing.T) {
	// Use `go list` to check all packages under ./...
	// The -e flag allows listing packages with errors (like import cycles)
	// so we can detect and report them instead of failing silently.
	cmd := exec.Command("go", "list", "-e", "-f", "{{if .Error}}{{.ImportPath}}: {{.Error.Err}}{{end}}", "./...")
	cmd.Dir = ".." // Run from repo root (one level up from cmd/)

	// Non-zero exit is expected if there are errors in packages; we inspect
	// the output content for import cycle messages instead of failing on exit.
	output, _ := cmd.CombinedOutput()

	outputStr := strings.TrimSpace(string(output))
	if outputStr == "" {
		// No errors found - all packages are clean
		return
	}

	// Check if any line contains "import cycle" or "circular" dependency errors
	lines := strings.Split(outputStr, "\n")
	var cycleErrors []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// Go reports import cycles with specific error text
		if strings.Contains(line, "import cycle") || strings.Contains(line, "circular") {
			cycleErrors = append(cycleErrors, line)
		}
	}

	if len(cycleErrors) > 0 {
		t.Errorf("circular dependencies detected:\n%s", strings.Join(cycleErrors, "\n"))
	}
}

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
	expected := []string{"ploy", "ployd", "ployd-node"}
	if len(dirs) != len(expected) {
		t.Fatalf("unexpected command directories: got %v, want %v", dirs, expected)
	}
	for i, name := range dirs {
		if name != expected[i] {
			t.Fatalf("unexpected command directory at position %d: got %q, want %q", i, name, expected[i])
		}
	}
}
