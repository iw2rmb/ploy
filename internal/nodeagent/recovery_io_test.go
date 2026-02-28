package nodeagent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExecuteHealingJob_ParseActionSummary(t *testing.T) {
	t.Parallel()

	outDir := t.TempDir()
	payload := `{"action_summary":"Applied retry-safe Gradle wrapper fix"}` + "\n"
	if err := os.WriteFile(filepath.Join(outDir, "codex-last.txt"), []byte(payload), 0o644); err != nil {
		t.Fatalf("write codex-last: %v", err)
	}

	got := parseActionSummary(outDir)
	want := "Applied retry-safe Gradle wrapper fix"
	if got != want {
		t.Fatalf("parseActionSummary() = %q, want %q", got, want)
	}
}

func TestExecuteHealingJob_ParseActionSummary_TruncatesToOneLine(t *testing.T) {
	t.Parallel()

	outDir := t.TempDir()
	long := strings.Repeat("a", 220) + "\nwith newline"
	body, err := json.Marshal(map[string]string{"action_summary": long})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	payload := string(body) + "\n"
	if err := os.WriteFile(filepath.Join(outDir, "codex-last.txt"), []byte(payload), 0o644); err != nil {
		t.Fatalf("write codex-last: %v", err)
	}

	got := parseActionSummary(outDir)
	if strings.Contains(got, "\n") {
		t.Fatalf("parseActionSummary() contains newline: %q", got)
	}
	if len([]rune(got)) != 200 {
		t.Fatalf("parseActionSummary() rune length = %d, want 200", len([]rune(got)))
	}
}
