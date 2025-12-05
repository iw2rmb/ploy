package types

import (
	"encoding/json"
	"testing"
)

func TestRunStats_ExitCode(t *testing.T) {
	tests := []struct {
		name      string
		stats     RunStats
		wantCode  int
		wantFound bool
	}{
		{
			name:      "int exit code",
			stats:     RunStats{"exit_code": 0},
			wantCode:  0,
			wantFound: true,
		},
		{
			name:      "int64 exit code",
			stats:     RunStats{"exit_code": int64(1)},
			wantCode:  1,
			wantFound: true,
		},
		{
			name:      "float64 exit code from JSON",
			stats:     RunStats{"exit_code": float64(2)},
			wantCode:  2,
			wantFound: true,
		},
		{
			name:      "missing exit code",
			stats:     RunStats{},
			wantCode:  0,
			wantFound: false,
		},
		{
			name:      "nil exit code",
			stats:     RunStats{"exit_code": nil},
			wantCode:  0,
			wantFound: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code, found := tt.stats.ExitCode()
			if code != tt.wantCode || found != tt.wantFound {
				t.Errorf("ExitCode() = (%d, %v), want (%d, %v)", code, found, tt.wantCode, tt.wantFound)
			}
		})
	}
}

func TestRunStats_ResumeCount(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		stats RunStats
		want  int
	}{
		{
			name:  "int resume count",
			stats: RunStats{"resume_count": 3},
			want:  3,
		},
		{
			name:  "int64 resume count",
			stats: RunStats{"resume_count": int64(5)},
			want:  5,
		},
		{
			name:  "float64 resume count from JSON",
			stats: RunStats{"resume_count": float64(2)},
			want:  2,
		},
		{
			name:  "missing resume count",
			stats: RunStats{},
			want:  0,
		},
		{
			name:  "nil resume count",
			stats: RunStats{"resume_count": nil},
			want:  0,
		},
		{
			name:  "invalid type (string)",
			stats: RunStats{"resume_count": "not a number"},
			want:  0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := tt.stats.ResumeCount()
			if got != tt.want {
				t.Errorf("ResumeCount() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestRunStats_LastResumedAt(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		stats RunStats
		want  string
	}{
		{
			name:  "timestamp present",
			stats: RunStats{"last_resumed_at": "2025-01-15T10:30:00Z"},
			want:  "2025-01-15T10:30:00Z",
		},
		{
			name:  "timestamp with whitespace",
			stats: RunStats{"last_resumed_at": "  2025-01-15T10:30:00Z  "},
			want:  "2025-01-15T10:30:00Z",
		},
		{
			name:  "missing timestamp",
			stats: RunStats{},
			want:  "",
		},
		{
			name:  "nil timestamp",
			stats: RunStats{"last_resumed_at": nil},
			want:  "",
		},
		{
			name:  "invalid type (int)",
			stats: RunStats{"last_resumed_at": 12345},
			want:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := tt.stats.LastResumedAt()
			if got != tt.want {
				t.Errorf("LastResumedAt() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestRunStats_ResumeMetadata_FromJSON(t *testing.T) {
	// Test that resume metadata works with real JSON-decoded data.
	t.Parallel()
	jsonData := `{
		"resume_count": 2,
		"last_resumed_at": "2025-01-15T10:30:00Z",
		"metadata": {
			"repo_base_ref": "main"
		}
	}`

	var stats RunStats
	if err := json.Unmarshal([]byte(jsonData), &stats); err != nil {
		t.Fatalf("json.Unmarshal() failed: %v", err)
	}

	// Test resume count extraction.
	gotCount := stats.ResumeCount()
	if gotCount != 2 {
		t.Errorf("ResumeCount() = %d, want 2", gotCount)
	}

	// Test last_resumed_at extraction.
	gotTimestamp := stats.LastResumedAt()
	wantTimestamp := "2025-01-15T10:30:00Z"
	if gotTimestamp != wantTimestamp {
		t.Errorf("LastResumedAt() = %q, want %q", gotTimestamp, wantTimestamp)
	}
}

func TestRunStats_MRURL(t *testing.T) {
	tests := []struct {
		name  string
		stats RunStats
		want  string
	}{
		{
			name: "MR URL present",
			stats: RunStats{
				"metadata": map[string]any{
					"mr_url": "https://gitlab.com/org/repo/-/merge_requests/42",
				},
			},
			want: "https://gitlab.com/org/repo/-/merge_requests/42",
		},
		{
			name: "MR URL with whitespace",
			stats: RunStats{
				"metadata": map[string]any{
					"mr_url": "  https://gitlab.com/org/repo/-/merge_requests/99  ",
				},
			},
			want: "https://gitlab.com/org/repo/-/merge_requests/99",
		},
		{
			name:  "no metadata",
			stats: RunStats{},
			want:  "",
		},
		{
			name: "metadata missing mr_url",
			stats: RunStats{
				"metadata": map[string]any{
					"other_field": "value",
				},
			},
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.stats.MRURL()
			if got != tt.want {
				t.Errorf("MRURL() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestRunStats_GateSummary(t *testing.T) {
	tests := []struct {
		name  string
		stats RunStats
		want  string
	}{
		{
			name: "final gate passed",
			stats: RunStats{
				"gate": map[string]any{
					"final_gate": map[string]any{
						"passed":      true,
						"duration_ms": int64(1234),
					},
				},
			},
			want: "passed duration=1234ms",
		},
		{
			name: "final gate failed",
			stats: RunStats{
				"gate": map[string]any{
					"final_gate": map[string]any{
						"passed":      false,
						"duration_ms": int64(567),
					},
				},
			},
			want: "failed final-gate duration=567ms",
		},
		{
			name: "pre gate passed (no final gate)",
			stats: RunStats{
				"gate": map[string]any{
					"pre_gate": map[string]any{
						"passed":      true,
						"duration_ms": int64(890),
					},
				},
			},
			want: "passed pre-gate duration=890ms",
		},
		{
			name: "pre gate failed",
			stats: RunStats{
				"gate": map[string]any{
					"pre_gate": map[string]any{
						"passed":      false,
						"duration_ms": int64(456),
					},
				},
			},
			want: "failed pre-gate duration=456ms",
		},
		{
			name: "re-gate last run passed",
			stats: RunStats{
				"gate": map[string]any{
					"re_gates": []any{
						map[string]any{
							"passed":      false,
							"duration_ms": int64(100),
						},
						map[string]any{
							"passed":      true,
							"duration_ms": int64(200),
						},
					},
				},
			},
			want: "passed re-gate duration=200ms",
		},
		{
			name: "final gate takes precedence over re-gates",
			stats: RunStats{
				"gate": map[string]any{
					"re_gates": []any{
						map[string]any{
							"passed":      true,
							"duration_ms": int64(100),
						},
					},
					"final_gate": map[string]any{
						"passed":      true,
						"duration_ms": int64(300),
					},
				},
			},
			want: "passed duration=300ms",
		},
		{
			name: "float64 duration from JSON",
			stats: RunStats{
				"gate": map[string]any{
					"final_gate": map[string]any{
						"passed":      true,
						"duration_ms": float64(1500),
					},
				},
			},
			want: "passed duration=1500ms",
		},
		{
			name:  "no gate data",
			stats: RunStats{},
			want:  "",
		},
		{
			name: "gate field is not a map",
			stats: RunStats{
				"gate": "invalid",
			},
			want: "",
		},
		{
			name: "gate phase missing passed field",
			stats: RunStats{
				"gate": map[string]any{
					"final_gate": map[string]any{
						"duration_ms": int64(100),
					},
				},
			},
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.stats.GateSummary()
			if got != tt.want {
				t.Errorf("GateSummary() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestRunStats_GateSummary_FinalGateFromPreModFallback verifies that when final_gate
// is populated from a pre-mod gate fallback (runs with no mods executed), GateSummary
// returns the final_gate content, not the pre_gate directly.
func TestRunStats_GateSummary_FinalGateFromPreModFallback(t *testing.T) {
	// Simulate stats from a run where no mods executed: pre_gate and final_gate both
	// present with same content (final_gate populated as fallback from pre-mod gate).
	stats := RunStats{
		"gate": map[string]any{
			"pre_gate": map[string]any{
				"passed":      true,
				"duration_ms": int64(500),
			},
			"final_gate": map[string]any{
				"passed":      true,
				"duration_ms": int64(500),
			},
		},
	}

	// GateSummary should use final_gate (which now exists), not fall back to pre_gate.
	got := stats.GateSummary()
	want := "passed duration=500ms" // final_gate passed, no "pre-gate" label.
	if got != want {
		t.Errorf("GateSummary() = %q, want %q", got, want)
	}
}

func TestRunStats_GateSummary_FromJSON(t *testing.T) {
	// Test that GateSummary works with real JSON-decoded data.
	jsonData := `{
		"exit_code": 0,
		"duration_ms": 5000,
		"gate": {
			"pre_gate": {
				"passed": true,
				"duration_ms": 1234,
				"resources": {
					"limits": {"nano_cpus": 2000000000, "memory_bytes": 536870912},
					"usage": {"cpu_total_ns": 1234567890, "mem_usage_bytes": 12345678}
				}
			},
			"final_gate": {
				"passed": false,
				"duration_ms": 2345,
				"resources": {
					"limits": {"nano_cpus": 2000000000, "memory_bytes": 536870912},
					"usage": {"cpu_total_ns": 2345678901, "mem_usage_bytes": 23456789}
				}
			}
		},
		"metadata": {
			"mr_url": "https://gitlab.com/org/repo/-/merge_requests/1"
		}
	}`

	var stats RunStats
	if err := json.Unmarshal([]byte(jsonData), &stats); err != nil {
		t.Fatalf("json.Unmarshal() failed: %v", err)
	}

	// Test gate summary extraction (final_gate takes precedence).
	got := stats.GateSummary()
	want := "failed final-gate duration=2345ms"
	if got != want {
		t.Errorf("GateSummary() = %q, want %q", got, want)
	}

	// Test MR URL extraction.
	gotMR := stats.MRURL()
	wantMR := "https://gitlab.com/org/repo/-/merge_requests/1"
	if gotMR != wantMR {
		t.Errorf("MRURL() = %q, want %q", gotMR, wantMR)
	}

	// Test exit code extraction.
	gotCode, gotFound := stats.ExitCode()
	wantCode, wantFound := 0, true
	if gotCode != wantCode || gotFound != wantFound {
		t.Errorf("ExitCode() = (%d, %v), want (%d, %v)", gotCode, gotFound, wantCode, wantFound)
	}
}
