package store

import (
	"testing"

	"github.com/google/uuid"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/jackc/pgx/v5/pgtype"
)

func TestUUIDBridge_Roundtrip(t *testing.T) {
	t.Run("RunID", func(t *testing.T) {
		u := uuid.New()
		in := domaintypes.RunID(u.String())
		pu := ToPGUUID(in)
		if !pu.Valid {
			t.Fatalf("expected valid pgtype UUID")
		}
		out := FromPGUUID[domaintypes.RunID](pu)
		if out != in {
			t.Fatalf("roundtrip mismatch: got %q want %q", out, in)
		}
	})

	t.Run("StepID", func(t *testing.T) {
		u := uuid.New()
		in := domaintypes.StepID(u.String())
		out := FromPGUUID[domaintypes.StepID](ToPGUUID(in))
		if out != in {
			t.Fatalf("roundtrip mismatch: got %q want %q", out, in)
		}
	})
}

func TestUUIDBridge_InvalidInputs(t *testing.T) {
	tests := []struct {
		name string
		in   string
	}{
		{"empty", ""},
		{"spaces", "   "},
		{"nonnumeric", "zzzz"},
		{"short", "1234"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ToPGUUID(domaintypes.RunID(tt.in))
			if got != (pgtype.UUID{}) {
				t.Fatalf("expected zero-value pgtype.UUID, got %#v", got)
			}
		})
	}

	t.Run("from zero pgtype", func(t *testing.T) {
		var z pgtype.UUID
		out := FromPGUUID[domaintypes.RunID](z)
		if out != "" {
			t.Fatalf("expected empty ID, got %q", string(out))
		}
	})
}
