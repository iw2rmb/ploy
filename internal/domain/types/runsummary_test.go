package types

import (
	"encoding/json"
	"testing"
	"time"
)

// TestRunSummaryJSON verifies that RunSummary JSON marshaling and unmarshaling
// correctly handles typed ID fields and rejects empty IDs.
func TestRunSummaryJSON(t *testing.T) {
	t.Parallel()

	t.Run("roundtrip_valid", func(t *testing.T) {
		t.Parallel()

		now := time.Now().Truncate(time.Second).UTC()
		started := now.Add(time.Second)
		finished := now.Add(time.Minute)

		original := RunSummary{
			ID:         RunID("2NQPoBfVkc8dFmGAQqJnUwMu9jR"),
			Status:     "Started",
			MigID:      MigID("mig-x1"),
			SpecID:     SpecID("spec-y2Z"),
			CreatedBy:  ptr("test-user"),
			CreatedAt:  now,
			StartedAt:  &started,
			FinishedAt: &finished,
			Counts: &RunRepoCounts{
				Total:         10,
				Queued:        2,
				Running:       3,
				Success:       4,
				Fail:          1,
				Cancelled:     0,
				DerivedStatus: "running",
			},
		}

		b, err := json.Marshal(original)
		if err != nil {
			t.Fatalf("Marshal: %v", err)
		}

		var decoded RunSummary
		if err := json.Unmarshal(b, &decoded); err != nil {
			t.Fatalf("Unmarshal: %v", err)
		}

		if decoded.ID != original.ID {
			t.Errorf("ID mismatch: got %q, want %q", decoded.ID, original.ID)
		}
		if decoded.MigID != original.MigID {
			t.Errorf("MigID mismatch: got %q, want %q", decoded.MigID, original.MigID)
		}
		if decoded.SpecID != original.SpecID {
			t.Errorf("SpecID mismatch: got %q, want %q", decoded.SpecID, original.SpecID)
		}
		if decoded.Status != original.Status {
			t.Errorf("Status mismatch: got %q, want %q", decoded.Status, original.Status)
		}
	})

	t.Run("rejects_empty_mod_id", func(t *testing.T) {
		t.Parallel()

		// JSON with empty mig_id should fail to unmarshal.
		jsonData := `{
			"id": "2NQPoBfVkc8dFmGAQqJnUwMu9jR",
			"status": "Started",
			"mig_id": "",
			"spec_id": "spec-y2Z",
			"created_at": "2024-01-01T00:00:00Z"
		}`

		var s RunSummary
		err := json.Unmarshal([]byte(jsonData), &s)
		if err == nil {
			t.Fatal("expected error for empty mig_id, got nil")
		}
	})

	t.Run("rejects_empty_spec_id", func(t *testing.T) {
		t.Parallel()

		// JSON with empty spec_id should fail to unmarshal.
		jsonData := `{
			"id": "2NQPoBfVkc8dFmGAQqJnUwMu9jR",
			"status": "Started",
			"mig_id": "mig-x1",
			"spec_id": "",
			"created_at": "2024-01-01T00:00:00Z"
		}`

		var s RunSummary
		err := json.Unmarshal([]byte(jsonData), &s)
		if err == nil {
			t.Fatal("expected error for empty spec_id, got nil")
		}
	})

	t.Run("rejects_whitespace_mod_id", func(t *testing.T) {
		t.Parallel()

		// JSON with whitespace-only mig_id should fail to unmarshal.
		jsonData := `{
			"id": "2NQPoBfVkc8dFmGAQqJnUwMu9jR",
			"status": "Started",
			"mig_id": "   ",
			"spec_id": "spec-y2Z",
			"created_at": "2024-01-01T00:00:00Z"
		}`

		var s RunSummary
		err := json.Unmarshal([]byte(jsonData), &s)
		if err == nil {
			t.Fatal("expected error for whitespace mig_id, got nil")
		}
	})

	t.Run("rejects_whitespace_spec_id", func(t *testing.T) {
		t.Parallel()

		// JSON with whitespace-only spec_id should fail to unmarshal.
		jsonData := `{
			"id": "2NQPoBfVkc8dFmGAQqJnUwMu9jR",
			"status": "Started",
			"mig_id": "mig-x1",
			"spec_id": "   ",
			"created_at": "2024-01-01T00:00:00Z"
		}`

		var s RunSummary
		err := json.Unmarshal([]byte(jsonData), &s)
		if err == nil {
			t.Fatal("expected error for whitespace spec_id, got nil")
		}
	})

	t.Run("rejects_empty_run_id", func(t *testing.T) {
		t.Parallel()

		// JSON with empty id should fail to unmarshal.
		jsonData := `{
			"id": "",
			"status": "Started",
			"mig_id": "mig-x1",
			"spec_id": "spec-y2",
			"created_at": "2024-01-01T00:00:00Z"
		}`

		var s RunSummary
		err := json.Unmarshal([]byte(jsonData), &s)
		if err == nil {
			t.Fatal("expected error for empty run id, got nil")
		}
	})

	t.Run("typed_fields_compile", func(t *testing.T) {
		// This test verifies at compile time that MigID and SpecID fields
		// are typed as types.MigID and types.SpecID, not raw strings.
		// If these fields were strings, this code would not compile.
		s := RunSummary{
			ID:        RunID("run-1"),
			MigID:     MigID("mig-1"),
			SpecID:    SpecID("spec-1"),
			Status:    "Started",
			CreatedAt: time.Now(),
		}

		// Verify typed fields can be passed to functions expecting domain types.
		_ = s.MigID.String()
		_ = s.SpecID.String()
		_ = s.MigID.IsZero()
		_ = s.SpecID.IsZero()
	})
}

func ptr[T any](v T) *T {
	return &v
}
