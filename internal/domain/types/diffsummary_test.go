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

func TestDiffSummary_StepIndex(t *testing.T) {
	tests := []struct {
		name      string
		json      string
		wantIdx   StepIndex
		wantFound bool
	}{
		{
			name:      "step index present",
			json:      `{"next_id": 2}`,
			wantIdx:   2,
			wantFound: true,
		},
		{
			name:      "zero step index",
			json:      `{"next_id": 0}`,
			wantIdx:   0,
			wantFound: true,
		},
		{
			name:      "missing step index",
			json:      `{}`,
			wantIdx:   0,
			wantFound: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			summary := mustParseDiffSummary(t, tt.json)
			idx, found := summary.StepIndex()
			if idx != tt.wantIdx || found != tt.wantFound {
				t.Errorf("StepIndex() = (%v, %v), want (%v, %v)", idx, found, tt.wantIdx, tt.wantFound)
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
			name: "mod type present",
			json: `{"job_type": "mod"}`,
			want: "mod",
		},
		{
			name: "healing mod type",
			json: `{"job_type": "healing"}`,
			want: "healing",
		},
		{
			name: "missing mod type",
			json: `{}`,
			want: "",
		},
		{
			name: "null mod type",
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
		"next_id": 1,
		"job_type": "mod",
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

	// Verify step index.
	idx, found := summary.StepIndex()
	if !found || idx != 1 {
		t.Errorf("StepIndex() = (%v, %v), want (1, true)", idx, found)
	}

	// Verify mod type.
	modType := summary.JobType()
	if modType != "mod" {
		t.Errorf("JobType() = %q, want %q", modType, "mod")
	}
}

func TestDiffSummaryBuilder(t *testing.T) {
	t.Run("basic fields", func(t *testing.T) {
		summary := NewDiffSummaryBuilder().
			ExitCode(0).
			StepIndex(StepIndex(1)).
			JobType("mod").
			MustBuild()

		code, found := summary.ExitCode()
		if !found || code != 0 {
			t.Errorf("ExitCode() = (%d, %v), want (0, true)", code, found)
		}

		idx, found := summary.StepIndex()
		if !found || idx != 1 {
			t.Errorf("StepIndex() = (%v, %v), want (1, true)", idx, found)
		}

		modType := summary.JobType()
		if modType != "mod" {
			t.Errorf("JobType() = %q, want %q", modType, "mod")
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
			StepIndex(StepIndex(2)).
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

		idx, found := parsed.StepIndex()
		if !found || idx != 2 {
			t.Errorf("StepIndex() = (%v, %v), want (2, true)", idx, found)
		}

		modType := parsed.JobType()
		if modType != "healing" {
			t.Errorf("JobType() = %q, want %q", modType, "healing")
		}

		files, found := parsed.FilesChanged()
		if !found || files != 5 {
			t.Errorf("FilesChanged() = (%d, %v), want (5, true)", files, found)
		}
	})
}
