package nodeagent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

func TestParseActionSummary(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		payload string
		check   func(t *testing.T, got string)
	}{
		{
			name:    "valid",
			payload: `{"action_summary":"Applied retry-safe Gradle wrapper fix"}` + "\n",
			check: func(t *testing.T, got string) {
				t.Helper()
				if want := "Applied retry-safe Gradle wrapper fix"; got != want {
					t.Fatalf("parseActionSummary() = %q, want %q", got, want)
				}
			},
		},
		{
			name: "truncates_to_one_line",
			payload: func() string {
				long := strings.Repeat("a", 220) + "\nwith newline"
				body, _ := json.Marshal(map[string]string{"action_summary": long})
				return string(body) + "\n"
			}(),
			check: func(t *testing.T, got string) {
				t.Helper()
				if strings.Contains(got, "\n") {
					t.Fatalf("parseActionSummary() contains newline: %q", got)
				}
				if len([]rune(got)) != 200 {
					t.Fatalf("parseActionSummary() rune length = %d, want 200", len([]rune(got)))
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			outDir := t.TempDir()
			if err := os.WriteFile(filepath.Join(outDir, "heal.json"), []byte(tt.payload), 0o644); err != nil {
				t.Fatalf("write codex-last: %v", err)
			}
			tt.check(t, parseActionSummary(outDir))
		})
	}
}

func TestParseORWFailureMetadata(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		payload   *string // nil means no file written
		wantErr   bool
		errSubstr string
		check     func(t *testing.T, meta map[string]string)
	}{
		{
			name:    "unsupported",
			payload: strPtr(`{"success":false,"error_kind":"unsupported","reason":"type-attribution-unavailable","message":"Type attribution is unavailable for this repository"}`),
			check: func(t *testing.T, meta map[string]string) {
				t.Helper()
				if got := meta[orwStatsMetadataErrorKind]; got != "unsupported" {
					t.Fatalf("%s=%q, want unsupported", orwStatsMetadataErrorKind, got)
				}
				if got := meta[orwStatsMetadataReason]; got != "type-attribution-unavailable" {
					t.Fatalf("%s=%q, want type-attribution-unavailable", orwStatsMetadataReason, got)
				}
			},
		},
		{
			name:    "success_returns_nil",
			payload: strPtr(`{"success":true,"message":"ok"}`),
			check: func(t *testing.T, meta map[string]string) {
				t.Helper()
				if len(meta) != 0 {
					t.Fatalf("expected empty metadata for success report, got %#v", meta)
				}
			},
		},
		{
			name:    "missing_report_returns_nil",
			payload: nil,
			check: func(t *testing.T, meta map[string]string) {
				t.Helper()
				if len(meta) != 0 {
					t.Fatalf("expected empty metadata when report is missing, got %#v", meta)
				}
			},
		},
		{
			name:      "invalid_report_returns_error",
			payload:   strPtr(`{"success":false,"error_kind":"unsupported"}`),
			wantErr:   true,
			errSubstr: "parse report.json",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			outDir := t.TempDir()
			if tt.payload != nil {
				if err := os.WriteFile(filepath.Join(outDir, contracts.ORWCLIReportFileName), []byte(*tt.payload), 0o644); err != nil {
					t.Fatalf("write report.json: %v", err)
				}
			}

			meta, err := parseORWFailureMetadata(outDir)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if !strings.Contains(err.Error(), tt.errSubstr) {
					t.Fatalf("error=%q, want substring %q", err.Error(), tt.errSubstr)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseORWFailureMetadata() error: %v", err)
			}
			tt.check(t, meta)
		})
	}
}

func strPtr(s string) *string { return &s }
