package types

import (
	"encoding/json"
	"testing"
)

// Helper function to create RunStats from JSON string for test cases.
func mustParseRunStats(t *testing.T, jsonStr string) RunStats {
	t.Helper()
	var stats RunStats
	if err := json.Unmarshal([]byte(jsonStr), &stats); err != nil {
		t.Fatalf("failed to parse RunStats JSON: %v", err)
	}
	return stats
}

func TestRunStats_ExitCode(t *testing.T) {
	tests := []struct {
		name      string
		json      string
		wantCode  int
		wantFound bool
	}{
		{
			name:      "int exit code",
			json:      `{"exit_code": 0}`,
			wantCode:  0,
			wantFound: true,
		},
		{
			name:      "positive exit code",
			json:      `{"exit_code": 1}`,
			wantCode:  1,
			wantFound: true,
		},
		{
			name:      "large exit code",
			json:      `{"exit_code": 127}`,
			wantCode:  127,
			wantFound: true,
		},
		{
			name:      "missing exit code",
			json:      `{}`,
			wantCode:  0,
			wantFound: false,
		},
		{
			name:      "null exit code",
			json:      `{"exit_code": null}`,
			wantCode:  0,
			wantFound: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stats := mustParseRunStats(t, tt.json)
			code, found := stats.ExitCode()
			if code != tt.wantCode || found != tt.wantFound {
				t.Errorf("ExitCode() = (%d, %v), want (%d, %v)", code, found, tt.wantCode, tt.wantFound)
			}
		})
	}
}

func TestRunStats_ResumeCount(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		json string
		want int
	}{
		{
			name: "resume count present",
			json: `{"resume_count": 3}`,
			want: 3,
		},
		{
			name: "resume count zero",
			json: `{"resume_count": 0}`,
			want: 0,
		},
		{
			name: "missing resume count",
			json: `{}`,
			want: 0,
		},
		{
			name: "null resume count",
			json: `{"resume_count": null}`,
			want: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			stats := mustParseRunStats(t, tt.json)
			got := stats.ResumeCount()
			if got != tt.want {
				t.Errorf("ResumeCount() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestRunStats_LastResumedAt(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		json string
		want string
	}{
		{
			name: "timestamp present",
			json: `{"last_resumed_at": "2025-01-15T10:30:00Z"}`,
			want: "2025-01-15T10:30:00Z",
		},
		{
			name: "timestamp with whitespace",
			json: `{"last_resumed_at": "  2025-01-15T10:30:00Z  "}`,
			want: "2025-01-15T10:30:00Z",
		},
		{
			name: "missing timestamp",
			json: `{}`,
			want: "",
		},
		{
			name: "null timestamp",
			json: `{"last_resumed_at": null}`,
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			stats := mustParseRunStats(t, tt.json)
			got := stats.LastResumedAt()
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
		name string
		json string
		want string
	}{
		{
			name: "MR URL present",
			json: `{"metadata": {"mr_url": "https://gitlab.com/org/repo/-/merge_requests/42"}}`,
			want: "https://gitlab.com/org/repo/-/merge_requests/42",
		},
		{
			name: "MR URL with whitespace",
			json: `{"metadata": {"mr_url": "  https://gitlab.com/org/repo/-/merge_requests/99  "}}`,
			want: "https://gitlab.com/org/repo/-/merge_requests/99",
		},
		{
			name: "no metadata",
			json: `{}`,
			want: "",
		},
		{
			name: "metadata missing mr_url",
			json: `{"metadata": {"other_field": "value"}}`,
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stats := mustParseRunStats(t, tt.json)
			got := stats.MRURL()
			if got != tt.want {
				t.Errorf("MRURL() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestRunStats_GateSummary(t *testing.T) {
	tests := []struct {
		name string
		json string
		want string
	}{
		{
			name: "final gate passed",
			json: `{"gate": {"final_gate": {"passed": true, "duration_ms": 1234}}}`,
			want: "passed duration=1234ms",
		},
		{
			name: "final gate failed",
			json: `{"gate": {"final_gate": {"passed": false, "duration_ms": 567}}}`,
			want: "failed final-gate duration=567ms",
		},
		{
			name: "pre gate passed (no final gate)",
			json: `{"gate": {"pre_gate": {"passed": true, "duration_ms": 890}}}`,
			want: "passed pre-gate duration=890ms",
		},
		{
			name: "pre gate failed",
			json: `{"gate": {"pre_gate": {"passed": false, "duration_ms": 456}}}`,
			want: "failed pre-gate duration=456ms",
		},
		{
			name: "re-gate last run passed",
			json: `{"gate": {"re_gates": [{"passed": false, "duration_ms": 100}, {"passed": true, "duration_ms": 200}]}}`,
			want: "passed re-gate duration=200ms",
		},
		{
			name: "final gate takes precedence over re-gates",
			json: `{"gate": {"re_gates": [{"passed": true, "duration_ms": 100}], "final_gate": {"passed": true, "duration_ms": 300}}}`,
			want: "passed duration=300ms",
		},
		{
			name: "float64 duration from JSON",
			json: `{"gate": {"final_gate": {"passed": true, "duration_ms": 1500}}}`,
			want: "passed duration=1500ms",
		},
		{
			name: "no gate data",
			json: `{}`,
			want: "",
		},
		{
			name: "gate field is not an object",
			json: `{"gate": "invalid"}`,
			want: "",
		},
		{
			// Note: With json.RawMessage-backed types, missing "passed" field
			// defaults to false (Go zero value for bool). The old map[string]any
			// implementation returned "" for missing passed. The new behavior
			// returns "failed" since passed=false is a valid gate result.
			name: "gate phase missing passed field (defaults to false)",
			json: `{"gate": {"final_gate": {"duration_ms": 100}}}`,
			want: "failed final-gate duration=100ms",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stats := mustParseRunStats(t, tt.json)
			got := stats.GateSummary()
			if got != tt.want {
				t.Errorf("GateSummary() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestRunStats_GateSummary_FinalGateFromPreMigFallback verifies that when final_gate
// is populated from a pre-mig gate fallback (runs with no migs executed), GateSummary
// returns the final_gate content, not the pre_gate directly.
func TestRunStats_GateSummary_FinalGateFromPreMigFallback(t *testing.T) {
	// Simulate stats from a run where no migs executed: pre_gate and final_gate both
	// present with same content (final_gate populated as fallback from pre-mig gate).
	stats := mustParseRunStats(t, `{
		"gate": {
			"pre_gate": {"passed": true, "duration_ms": 500},
			"final_gate": {"passed": true, "duration_ms": 500}
		}
	}`)

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

func TestRunStats_IsEmpty(t *testing.T) {
	tests := []struct {
		name string
		json string
		want bool
	}{
		{
			name: "nil stats",
			json: "",
			want: true,
		},
		{
			name: "empty object",
			json: `{}`,
			want: true,
		},
		{
			name: "null",
			json: `null`,
			want: true,
		},
		{
			name: "non-empty",
			json: `{"exit_code": 0}`,
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stats RunStats
			if tt.json != "" {
				_ = json.Unmarshal([]byte(tt.json), &stats)
			}
			got := stats.IsEmpty()
			if got != tt.want {
				t.Errorf("IsEmpty() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRunStatsBuilder(t *testing.T) {
	t.Run("basic fields", func(t *testing.T) {
		stats := NewRunStatsBuilder().
			ExitCode(0).
			DurationMs(1234).
			MustBuild()

		code, found := stats.ExitCode()
		if !found || code != 0 {
			t.Errorf("ExitCode() = (%d, %v), want (0, true)", code, found)
		}
	})

	t.Run("with metadata", func(t *testing.T) {
		stats := NewRunStatsBuilder().
			MetadataEntry("mr_url", "https://example.com/mr/1").
			MustBuild()

		mrURL := stats.MRURL()
		if mrURL != "https://example.com/mr/1" {
			t.Errorf("MRURL() = %q, want %q", mrURL, "https://example.com/mr/1")
		}
	})

	t.Run("with timings", func(t *testing.T) {
		stats := NewRunStatsBuilder().
			ExitCode(0).
			TimingsFromDurations(100, 200, 50, 400).
			MustBuild()

		// Verify the stats round-trips correctly.
		code, found := stats.ExitCode()
		if !found || code != 0 {
			t.Errorf("ExitCode() = (%d, %v), want (0, true)", code, found)
		}
	})

	t.Run("with gate", func(t *testing.T) {
		stats := NewRunStatsBuilder().
			Gate(true, 500).
			MustBuild()

		// Gate summary should work for simple gate stats.
		summary := stats.GateSummary()
		if summary == "" {
			t.Error("GateSummary() returned empty for gate stats")
		}
	})

	t.Run("with healing warning", func(t *testing.T) {
		stats := NewRunStatsBuilder().
			ExitCode(1).
			HealingWarning("no_workspace_changes").
			MustBuild()

		// Verify the stats is non-empty and contains the exit code.
		code, found := stats.ExitCode()
		if !found || code != 1 {
			t.Errorf("ExitCode() = (%d, %v), want (1, true)", code, found)
		}
	})

	t.Run("empty builder", func(t *testing.T) {
		stats := NewRunStatsBuilder().Build()
		if stats != nil {
			t.Errorf("Build() with no fields should return nil, got %v", string(stats))
		}
	})

	t.Run("JSON roundtrip", func(t *testing.T) {
		original := NewRunStatsBuilder().
			ExitCode(42).
			DurationMs(5000).
			MetadataEntry("key", "value").
			MustBuild()

		// Marshal to JSON.
		data, err := json.Marshal(original)
		if err != nil {
			t.Fatalf("json.Marshal() failed: %v", err)
		}

		// Unmarshal back.
		var parsed RunStats
		if err := json.Unmarshal(data, &parsed); err != nil {
			t.Fatalf("json.Unmarshal() failed: %v", err)
		}

		// Verify fields.
		code, found := parsed.ExitCode()
		if !found || code != 42 {
			t.Errorf("ExitCode() = (%d, %v), want (42, true)", code, found)
		}

		meta := parsed.Metadata()
		if meta["key"] != "value" {
			t.Errorf("Metadata()[key] = %q, want %q", meta["key"], "value")
		}
	})
}
