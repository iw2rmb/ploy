package api

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
)

// TestMigSummaryDTO verifies JSON field names and roundtrip stability for MigSummary.
func TestMigSummaryDTO(t *testing.T) {
	t.Parallel()

	specID := domaintypes.SpecID("specAbCd")
	createdBy := "ci-bot"
	now := time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC)

	t.Run("roundtrip_full", func(t *testing.T) {
		t.Parallel()
		in := MigSummary{
			ID:        domaintypes.MigID("migAbc"),
			Name:      "my-mig",
			SpecID:    &specID,
			CreatedBy: &createdBy,
			Archived:  false,
			CreatedAt: now,
		}
		b, err := json.Marshal(in)
		if err != nil {
			t.Fatalf("Marshal: %v", err)
		}
		var out MigSummary
		if err := json.Unmarshal(b, &out); err != nil {
			t.Fatalf("Unmarshal: %v", err)
		}
		if out.ID != in.ID {
			t.Errorf("ID: got %q, want %q", out.ID, in.ID)
		}
		if out.Name != in.Name {
			t.Errorf("Name: got %q, want %q", out.Name, in.Name)
		}
		if out.SpecID == nil || *out.SpecID != specID {
			t.Errorf("SpecID: got %v, want %v", out.SpecID, specID)
		}
		if out.CreatedBy == nil || *out.CreatedBy != createdBy {
			t.Errorf("CreatedBy: got %v, want %v", out.CreatedBy, createdBy)
		}
		if !out.CreatedAt.Equal(in.CreatedAt) {
			t.Errorf("CreatedAt: got %v, want %v", out.CreatedAt, in.CreatedAt)
		}
	})

	t.Run("wire_field_names", func(t *testing.T) {
		t.Parallel()
		// Verify exact JSON field names to guard wire-shape stability.
		in := MigSummary{
			ID:        domaintypes.MigID("migAbc"),
			Name:      "wire-check",
			CreatedAt: now,
		}
		b, err := json.Marshal(in)
		if err != nil {
			t.Fatalf("Marshal: %v", err)
		}
		js := string(b)
		for _, want := range []string{`"id":`, `"name":`, `"archived":`, `"created_at":`} {
			if !strings.Contains(js, want) {
				t.Errorf("JSON missing field %s in %s", want, js)
			}
		}
		// spec_id and created_by are omitempty — must be absent when nil.
		for _, absent := range []string{`"spec_id"`, `"created_by"`} {
			if strings.Contains(js, absent) {
				t.Errorf("JSON must not contain %s when nil, got %s", absent, js)
			}
		}
	})

	t.Run("archived_true", func(t *testing.T) {
		t.Parallel()
		in := MigSummary{
			ID:        domaintypes.MigID("migAbc"),
			Name:      "archived-mig",
			Archived:  true,
			CreatedAt: now,
		}
		b, err := json.Marshal(in)
		if err != nil {
			t.Fatalf("Marshal: %v", err)
		}
		var out MigSummary
		if err := json.Unmarshal(b, &out); err != nil {
			t.Fatalf("Unmarshal: %v", err)
		}
		if !out.Archived {
			t.Error("Archived: got false, want true")
		}
	})
}

// TestMigListResponseDTO verifies the list envelope JSON field name.
func TestMigListResponseDTO(t *testing.T) {
	t.Parallel()

	resp := MigListResponse{
		Migs: []MigSummary{
			{ID: domaintypes.MigID("migAbc"), Name: "a", CreatedAt: time.Now()},
		},
	}
	b, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if !strings.Contains(string(b), `"migs":`) {
		t.Errorf("envelope field name must be \"migs\", got %s", string(b))
	}
}
