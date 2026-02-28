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

func TestParseRouterDecision_ParsesStructuredFields(t *testing.T) {
	t.Parallel()

	outDir := t.TempDir()
	payload := `{"bug_summary":"cannot compile","error_kind":"infra","strategy_id":"infra-default","confidence":0.7,"reason":"docker unavailable","expectations":{"artifacts":[{"path":"/out/gate-profile-candidate.json","schema":"gate_profile_v1"}]}}` + "\n"
	if err := os.WriteFile(filepath.Join(outDir, "codex-last.txt"), []byte(payload), 0o644); err != nil {
		t.Fatalf("write codex-last: %v", err)
	}

	got := parseRouterDecision(outDir)
	if got == nil {
		t.Fatal("parseRouterDecision() returned nil")
	}
	if got.LoopKind != "healing" {
		t.Fatalf("LoopKind = %q, want %q", got.LoopKind, "healing")
	}
	if got.ErrorKind != "infra" {
		t.Fatalf("ErrorKind = %q, want %q", got.ErrorKind, "infra")
	}
	if got.StrategyID != "infra-default" {
		t.Fatalf("StrategyID = %q, want %q", got.StrategyID, "infra-default")
	}
	if got.Confidence == nil || *got.Confidence != 0.7 {
		t.Fatalf("Confidence = %#v, want %v", got.Confidence, 0.7)
	}
	if got.Reason != "docker unavailable" {
		t.Fatalf("Reason = %q, want %q", got.Reason, "docker unavailable")
	}
	if len(got.Expectations) == 0 {
		t.Fatal("Expectations is empty, want JSON object")
	}
}

func TestParseRouterDecision_DefaultsToUnknownOnParseFailure(t *testing.T) {
	t.Parallel()

	outDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(outDir, "codex-last.txt"), []byte("not-json"), 0o644); err != nil {
		t.Fatalf("write codex-last: %v", err)
	}

	got := parseRouterDecision(outDir)
	if got == nil {
		t.Fatal("parseRouterDecision() returned nil")
	}
	if got.LoopKind != "healing" {
		t.Fatalf("LoopKind = %q, want %q", got.LoopKind, "healing")
	}
	if got.ErrorKind != "unknown" {
		t.Fatalf("ErrorKind = %q, want %q", got.ErrorKind, "unknown")
	}
}

func TestParseRouterDecision_InvalidErrorKindDefaultsToUnknown(t *testing.T) {
	t.Parallel()

	outDir := t.TempDir()
	payload := `{"error_kind":"routing"}` + "\n"
	if err := os.WriteFile(filepath.Join(outDir, "codex-last.txt"), []byte(payload), 0o644); err != nil {
		t.Fatalf("write codex-last: %v", err)
	}

	got := parseRouterDecision(outDir)
	if got == nil {
		t.Fatal("parseRouterDecision() returned nil")
	}
	if got.ErrorKind != "unknown" {
		t.Fatalf("ErrorKind = %q, want %q", got.ErrorKind, "unknown")
	}
}

func TestParseRouterDecision_CustomErrorKindDefaultsToUnknown(t *testing.T) {
	t.Parallel()

	outDir := t.TempDir()
	payload := `{"error_kind":"custom"}` + "\n"
	if err := os.WriteFile(filepath.Join(outDir, "codex-last.txt"), []byte(payload), 0o644); err != nil {
		t.Fatalf("write codex-last: %v", err)
	}

	got := parseRouterDecision(outDir)
	if got == nil {
		t.Fatal("parseRouterDecision() returned nil")
	}
	if got.ErrorKind != "unknown" {
		t.Fatalf("ErrorKind = %q, want %q", got.ErrorKind, "unknown")
	}
}
