package types

import (
	"encoding/json"
	"errors"
	"testing"
)

func TestDiffID_UnmarshalText(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		var id DiffID
		if err := id.UnmarshalText([]byte("550e8400-e29b-41d4-a716-446655440000")); err != nil {
			t.Fatalf("UnmarshalText() error = %v", err)
		}
		if id.String() != "550e8400-e29b-41d4-a716-446655440000" {
			t.Fatalf("got %q", id.String())
		}
	})

	t.Run("empty", func(t *testing.T) {
		var id DiffID
		if err := id.UnmarshalText([]byte("   ")); !errors.Is(err, ErrEmpty) {
			t.Fatalf("expected ErrEmpty, got %v", err)
		}
	})

	t.Run("invalid", func(t *testing.T) {
		var id DiffID
		if err := id.UnmarshalText([]byte("not-a-uuid")); !errors.Is(err, ErrInvalidDiffID) {
			t.Fatalf("expected ErrInvalidDiffID, got %v", err)
		}
	})
}

func TestDiffID_JSONRoundtrip(t *testing.T) {
	type payload struct {
		ID DiffID `json:"id"`
	}

	in := payload{ID: DiffID("550e8400-e29b-41d4-a716-446655440000")}
	data, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}

	var out payload
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if out.ID != in.ID {
		t.Fatalf("roundtrip mismatch: got %q want %q", out.ID, in.ID)
	}
}
