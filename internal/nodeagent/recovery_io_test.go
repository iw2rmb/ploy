package nodeagent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/iw2rmb/ploy/internal/workflow/contracts"
	"gopkg.in/yaml.v3"
)

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

func TestStructuredErrorsYAML(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		raw       json.RawMessage
		wantErr   bool
		errSubstr string
		check     func(t *testing.T, raw []byte)
	}{
		{
			name: "object",
			raw:  json.RawMessage(`{"task":"compile"}`),
			check: func(t *testing.T, raw []byte) {
				t.Helper()
				var payload map[string]any
				if err := yaml.Unmarshal(raw, &payload); err != nil {
					t.Fatalf("yaml.Unmarshal() error: %v", err)
				}
				if got := payload["task"]; got != "compile" {
					t.Fatalf("task=%v want compile", got)
				}
			},
		},
		{
			name:      "invalid",
			raw:       json.RawMessage(`{"task":`),
			wantErr:   true,
			errSubstr: "unexpected end of JSON input",
		},
		{
			name:      "scalar rejected",
			raw:       json.RawMessage(`"oops"`),
			wantErr:   true,
			errSubstr: "expected object or array",
		},
		{
			name: "empty returns nil",
			raw:  nil,
			check: func(t *testing.T, raw []byte) {
				t.Helper()
				if len(raw) != 0 {
					t.Fatalf("expected nil yaml, got %q", string(raw))
				}
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := structuredErrorsYAML(tt.raw)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if !strings.Contains(err.Error(), tt.errSubstr) {
					t.Fatalf("error=%q want substring %q", err.Error(), tt.errSubstr)
				}
				return
			}
			if err != nil {
				t.Fatalf("structuredErrorsYAML() error: %v", err)
			}
			tt.check(t, got)
		})
	}
}

func TestMaterializeParentGateLineageArtifacts(t *testing.T) {
	t.Parallel()

	outDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(outDir, "re_build-gate-1.log"), []byte("child-1\n"), 0o644); err != nil {
		t.Fatalf("write re_build-gate-1.log: %v", err)
	}
	if err := os.WriteFile(filepath.Join(outDir, "errors-1.yaml"), []byte("step: child-1\n"), 0o644); err != nil {
		t.Fatalf("write errors-1.yaml: %v", err)
	}
	if err := os.WriteFile(filepath.Join(outDir, "re_build-gate-3.log"), []byte("child-3\n"), 0o644); err != nil {
		t.Fatalf("write re_build-gate-3.log: %v", err)
	}
	if err := os.WriteFile(filepath.Join(outDir, "errors-3.yaml"), []byte("step: child-3\n"), 0o644); err != nil {
		t.Fatalf("write errors-3.yaml: %v", err)
	}

	recoveryCtx := &contracts.RecoveryClaimContext{
		BuildGateLog: "baseline\n",
		Errors:       json.RawMessage(`{"task":"baseline"}`),
	}
	if err := materializeParentGateLineageArtifacts(outDir, recoveryCtx); err != nil {
		t.Fatalf("materializeParentGateLineageArtifacts() error: %v", err)
	}

	if got, want := readFileString(t, filepath.Join(outDir, "re_build-gate-1.log")), "baseline\n"; got != want {
		t.Fatalf("re_build-gate-1.log=%q want %q", got, want)
	}
	rawErrors1, err := os.ReadFile(filepath.Join(outDir, "errors-1.yaml"))
	if err != nil {
		t.Fatalf("read errors-1.yaml: %v", err)
	}
	var baselinePayload map[string]any
	if err := yaml.Unmarshal(rawErrors1, &baselinePayload); err != nil {
		t.Fatalf("decode errors-1.yaml: %v", err)
	}
	if got := baselinePayload["task"]; got != "baseline" {
		t.Fatalf("errors-1.yaml task=%v want baseline", got)
	}

	if got, want := readFileString(t, filepath.Join(outDir, "re_build-gate-2.log")), "child-1\n"; got != want {
		t.Fatalf("re_build-gate-2.log=%q want %q", got, want)
	}
	if got, want := readFileString(t, filepath.Join(outDir, "errors-2.yaml")), "step: child-1\n"; got != want {
		t.Fatalf("errors-2.yaml=%q want %q", got, want)
	}
	if got, want := readFileString(t, filepath.Join(outDir, "re_build-gate-3.log")), "child-3\n"; got != want {
		t.Fatalf("re_build-gate-3.log=%q want %q", got, want)
	}
	if got, want := readFileString(t, filepath.Join(outDir, "errors-3.yaml")), "step: child-3\n"; got != want {
		t.Fatalf("errors-3.yaml=%q want %q", got, want)
	}
	if _, err := os.Stat(filepath.Join(outDir, "re_build-gate-4.log")); !os.IsNotExist(err) {
		t.Fatalf("re_build-gate-4.log should not exist, err=%v", err)
	}
}

func TestMaterializeParentGateLineageArtifacts_IdempotentBaseline(t *testing.T) {
	t.Parallel()

	outDir := t.TempDir()
	recoveryCtx := &contracts.RecoveryClaimContext{
		BuildGateLog: "baseline\n",
		Errors:       json.RawMessage(`{"task":"baseline"}`),
	}
	if err := materializeParentGateLineageArtifacts(outDir, recoveryCtx); err != nil {
		t.Fatalf("seed materializeParentGateLineageArtifacts() error: %v", err)
	}
	if err := os.WriteFile(filepath.Join(outDir, "re_build-gate-2.log"), []byte("child-2\n"), 0o644); err != nil {
		t.Fatalf("write re_build-gate-2.log: %v", err)
	}
	if err := os.WriteFile(filepath.Join(outDir, "errors-2.yaml"), []byte("step: child-2\n"), 0o644); err != nil {
		t.Fatalf("write errors-2.yaml: %v", err)
	}

	if err := materializeParentGateLineageArtifacts(outDir, recoveryCtx); err != nil {
		t.Fatalf("materializeParentGateLineageArtifacts() error: %v", err)
	}

	if got, want := readFileString(t, filepath.Join(outDir, "re_build-gate-1.log")), "baseline\n"; got != want {
		t.Fatalf("re_build-gate-1.log=%q want %q", got, want)
	}
	if got, want := readFileString(t, filepath.Join(outDir, "re_build-gate-2.log")), "child-2\n"; got != want {
		t.Fatalf("re_build-gate-2.log=%q want %q", got, want)
	}
}

func readFileString(t *testing.T, path string) string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(raw)
}
