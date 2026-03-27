package types

import (
	"encoding/json"
	"testing"
)

// Helper function to create DiffSummary from JSON string for test cases.
func mustParseDiffSummary(t *testing.T, jsonStr string) DiffSummary {
	t.Helper()
	var summary DiffSummary
	if err := json.Unmarshal([]byte(jsonStr), &summary); err != nil {
		t.Fatalf("failed to parse DiffSummary JSON: %v", err)
	}
	return summary
}

func TestDiffSummary_ExitCode(t *testing.T) {
	tests := []struct {
		name      string
		json      string
		wantCode  int
		wantFound bool
	}{
		{
			name:      "exit code present",
			json:      `{"exit_code": 0}`,
			wantCode:  0,
			wantFound: true,
		},
		{
			name:      "non-zero exit code",
			json:      `{"exit_code": 1}`,
			wantCode:  1,
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
			summary := mustParseDiffSummary(t, tt.json)
			code, found := summary.ExitCode()
			if code != tt.wantCode || found != tt.wantFound {
				t.Errorf("ExitCode() = (%d, %v), want (%d, %v)", code, found, tt.wantCode, tt.wantFound)
			}
		})
	}
}

func TestDiffSummary_FilesChanged(t *testing.T) {
	tests := []struct {
		name      string
		json      string
		wantCount int
		wantFound bool
	}{
		{
			name:      "files changed present",
			json:      `{"files_changed": 5}`,
			wantCount: 5,
			wantFound: true,
		},
		{
			name:      "zero files changed",
			json:      `{"files_changed": 0}`,
			wantCount: 0,
			wantFound: true,
		},
		{
			name:      "missing files changed",
			json:      `{}`,
			wantCount: 0,
			wantFound: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			summary := mustParseDiffSummary(t, tt.json)
			count, found := summary.FilesChanged()
			if count != tt.wantCount || found != tt.wantFound {
				t.Errorf("FilesChanged() = (%d, %v), want (%d, %v)", count, found, tt.wantCount, tt.wantFound)
			}
		})
	}
}

func TestDiffSummary_JobType(t *testing.T) {
	tests := []struct {
		name string
		json string
		want string
	}{
		{
			name: "mig type present",
			json: `{"job_type": "mig"}`,
			want: "mig",
		},
		{
			name: "healing mig type",
			json: `{"job_type": "healing"}`,
			want: "healing",
		},
		{
			name: "missing mig type",
			json: `{}`,
			want: "",
		},
		{
			name: "null mig type",
			json: `{"job_type": null}`,
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			summary := mustParseDiffSummary(t, tt.json)
			got := summary.JobType()
			if got != tt.want {
				t.Errorf("JobType() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestDiffSummary_IsEmpty(t *testing.T) {
	tests := []struct {
		name string
		json string
		want bool
	}{
		{
			name: "nil summary",
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
			var summary DiffSummary
			if tt.json != "" {
				_ = json.Unmarshal([]byte(tt.json), &summary)
			}
			got := summary.IsEmpty()
			if got != tt.want {
				t.Errorf("IsEmpty() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDiffSummary_FromJSON(t *testing.T) {
	// Test full JSON decode with all fields.
	jsonData := `{
		"exit_code": 0,
		"files_changed": 3,
		"job_type": "mig",
		"timings": {
			"hydration_duration_ms": 100,
			"execution_duration_ms": 200,
			"diff_duration_ms": 50,
			"total_duration_ms": 400
		}
	}`

	var summary DiffSummary
	if err := json.Unmarshal([]byte(jsonData), &summary); err != nil {
		t.Fatalf("json.Unmarshal() failed: %v", err)
	}

	// Verify exit code.
	code, found := summary.ExitCode()
	if !found || code != 0 {
		t.Errorf("ExitCode() = (%d, %v), want (0, true)", code, found)
	}

	// Verify files changed.
	files, found := summary.FilesChanged()
	if !found || files != 3 {
		t.Errorf("FilesChanged() = (%d, %v), want (3, true)", files, found)
	}

	// Verify mig type.
	jobType := summary.JobType()
	if jobType != "mig" {
		t.Errorf("JobType() = %q, want %q", jobType, "mig")
	}
}

func TestDiffSummaryBuilder(t *testing.T) {
	t.Run("basic fields", func(t *testing.T) {
		summary := NewDiffSummaryBuilder().
			ExitCode(0).
			JobType("mig").
			MustBuild()

		code, found := summary.ExitCode()
		if !found || code != 0 {
			t.Errorf("ExitCode() = (%d, %v), want (0, true)", code, found)
		}

		jobType := summary.JobType()
		if jobType != "mig" {
			t.Errorf("JobType() = %q, want %q", jobType, "mig")
		}
	})

	t.Run("with timings", func(t *testing.T) {
		summary := NewDiffSummaryBuilder().
			ExitCode(0).
			Timings(100, 200, 50, 400).
			MustBuild()

		// Verify the summary is non-empty.
		if summary.IsEmpty() {
			t.Error("IsEmpty() should return false for summary with timings")
		}

		code, found := summary.ExitCode()
		if !found || code != 0 {
			t.Errorf("ExitCode() = (%d, %v), want (0, true)", code, found)
		}
	})

	t.Run("empty builder", func(t *testing.T) {
		summary := NewDiffSummaryBuilder().Build()
		if summary != nil {
			t.Errorf("Build() with no fields should return nil, got %v", string(summary))
		}
	})

	t.Run("JSON roundtrip", func(t *testing.T) {
		original := NewDiffSummaryBuilder().
			ExitCode(0).
			JobType("healing").
			FilesChanged(5).
			MustBuild()

		// Marshal to JSON.
		data, err := json.Marshal(original)
		if err != nil {
			t.Fatalf("json.Marshal() failed: %v", err)
		}

		// Unmarshal back.
		var parsed DiffSummary
		if err := json.Unmarshal(data, &parsed); err != nil {
			t.Fatalf("json.Unmarshal() failed: %v", err)
		}

		// Verify fields.
		code, found := parsed.ExitCode()
		if !found || code != 0 {
			t.Errorf("ExitCode() = (%d, %v), want (0, true)", code, found)
		}

		jobType := parsed.JobType()
		if jobType != "healing" {
			t.Errorf("JobType() = %q, want %q", jobType, "healing")
		}

		files, found := parsed.FilesChanged()
		if !found || files != 5 {
			t.Errorf("FilesChanged() = (%d, %v), want (5, true)", files, found)
		}
	})
}

func TestDiffSummary_LinesAdded(t *testing.T) {
	tests := []struct {
		name      string
		json      string
		wantCount int
		wantFound bool
	}{
		{"present", `{"lines_added": 42}`, 42, true},
		{"zero", `{"lines_added": 0}`, 0, true},
		{"missing", `{}`, 0, false},
		{"null", `{"lines_added": null}`, 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			summary := mustParseDiffSummary(t, tt.json)
			count, found := summary.LinesAdded()
			if count != tt.wantCount || found != tt.wantFound {
				t.Errorf("LinesAdded() = (%d, %v), want (%d, %v)", count, found, tt.wantCount, tt.wantFound)
			}
		})
	}
}

func TestDiffSummary_LinesRemoved(t *testing.T) {
	tests := []struct {
		name      string
		json      string
		wantCount int
		wantFound bool
	}{
		{"present", `{"lines_removed": 7}`, 7, true},
		{"zero", `{"lines_removed": 0}`, 0, true},
		{"missing", `{}`, 0, false},
		{"null", `{"lines_removed": null}`, 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			summary := mustParseDiffSummary(t, tt.json)
			count, found := summary.LinesRemoved()
			if count != tt.wantCount || found != tt.wantFound {
				t.Errorf("LinesRemoved() = (%d, %v), want (%d, %v)", count, found, tt.wantCount, tt.wantFound)
			}
		})
	}
}

func TestDiffSummaryBuilder_LineDeltaRoundtrip(t *testing.T) {
	summary := NewDiffSummaryBuilder().
		LinesAdded(15).
		LinesRemoved(3).
		FilesChanged(2).
		MustBuild()

	added, ok := summary.LinesAdded()
	if !ok || added != 15 {
		t.Errorf("LinesAdded() = (%d, %v), want (15, true)", added, ok)
	}
	removed, ok := summary.LinesRemoved()
	if !ok || removed != 3 {
		t.Errorf("LinesRemoved() = (%d, %v), want (3, true)", removed, ok)
	}
	files, ok := summary.FilesChanged()
	if !ok || files != 2 {
		t.Errorf("FilesChanged() = (%d, %v), want (2, true)", files, ok)
	}
}
