package buildgate

import "testing"

func TestMetadataSanitizeTrimsAndFilters(t *testing.T) {
	meta := Metadata{
		LogDigest: "  sha256:ABC  ",
		StaticChecks: []StaticCheckReport{
			{
				Language: " Go ",
				Tool:     "  go/ vet  ",
				Passed:   false,
				Failures: []StaticCheckFailure{
					{RuleID: "  GOVET001  ", File: "  ./main.go  ", Line: -1, Column: -2, Severity: "ERROR", Message: "  unused import  "},
					{RuleID: "", File: "", Message: "", Severity: "", Line: 0, Column: 0},
				},
			},
			{
				Language: "   ",
				Tool:     " ",
				Passed:   true,
			},
		},
	}

	sanitized := Sanitize(meta)

	if sanitized.LogDigest != "sha256:ABC" {
		t.Fatalf("expected trimmed digest, got %q", sanitized.LogDigest)
	}
	if len(sanitized.StaticChecks) != 1 {
		t.Fatalf("expected single static check after filtering, got %d", len(sanitized.StaticChecks))
	}
	check := sanitized.StaticChecks[0]
	if check.Language != "Go" {
		t.Fatalf("expected trimmed language, got %q", check.Language)
	}
	if check.Tool != "go/ vet" {
		t.Fatalf("expected trimmed tool, got %q", check.Tool)
	}
	if check.Passed {
		t.Fatal("expected static check to remain failed")
	}
	if len(check.Failures) != 1 {
		t.Fatalf("expected single failure after filtering, got %d", len(check.Failures))
	}
	failure := check.Failures[0]
	if failure.RuleID != "GOVET001" {
		t.Fatalf("expected trimmed rule ID, got %q", failure.RuleID)
	}
	if failure.File != "./main.go" {
		t.Fatalf("expected trimmed file, got %q", failure.File)
	}
	if failure.Line != 0 {
		t.Fatalf("expected clamped line 0, got %d", failure.Line)
	}
	if failure.Column != 0 {
		t.Fatalf("expected clamped column 0, got %d", failure.Column)
	}
	if failure.Severity != "error" {
		t.Fatalf("expected severity lower-cased error, got %q", failure.Severity)
	}
	if failure.Message != "unused import" {
		t.Fatalf("expected trimmed message, got %q", failure.Message)
	}
}

func TestMetadataSanitizeAllowsEmptyDigest(t *testing.T) {
	empty := Metadata{}
	sanitized := Sanitize(empty)
	if sanitized.LogDigest != "" {
		t.Fatalf("expected empty digest to remain empty, got %q", sanitized.LogDigest)
	}
	if len(sanitized.StaticChecks) != 0 {
		t.Fatalf("expected no static checks, got %d", len(sanitized.StaticChecks))
	}
}
