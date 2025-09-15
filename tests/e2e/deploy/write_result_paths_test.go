//go:build e2e

package deploy

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestWriteResultPaths verifies results are written to repo-root-relative paths,
// regardless of the package working directory under `go test`.
func TestWriteResultPaths(t *testing.T) {
	// Ensure repo root resolution is active
	// Optionally allow overriding via E2E_REPO_ROOT in CI; here we just rely on resolveRepoPath.

	// Write a synthetic result entry
	metrics := map[string]any{
		"build": map[string]any{"duration_ms": float64(1234)},
	}
	writeResult(t, "T", "test://dummy-repo", "dummy-app", metrics)

	// Verify JSONL path
	jsonl := resolveRepoPath(filepath.Join("tests", "e2e", "deploy", "results.jsonl"))
	b, err := os.ReadFile(jsonl)
	if err != nil {
		t.Fatalf("failed to read results.jsonl: %v", err)
	}
	if !strings.Contains(string(b), "dummy-app") || !strings.Contains(string(b), "test://dummy-repo") {
		t.Fatalf("results.jsonl missing appended entry")
	}

	// Verify Markdown path
	md := resolveRepoPath(filepath.Join("tests", "e2e", "deploy", "results.md"))
	if _, err := os.Stat(md); err != nil {
		t.Fatalf("results.md not created: %v", err)
	}
	// Ensure the row contains our lane and repo markers
	mb, _ := os.ReadFile(md)
	if !strings.Contains(string(mb), "| T |") {
		t.Fatalf("results.md missing lane row")
	}

	// Sanity: the last JSON line should be valid JSON
	lines := strings.Split(strings.TrimSpace(string(b)), "\n")
	var tmp map[string]any
	if err := json.Unmarshal([]byte(lines[len(lines)-1]), &tmp); err != nil {
		t.Fatalf("invalid JSON line appended: %v", err)
	}
}
