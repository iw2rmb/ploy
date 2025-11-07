package types

import (
	"encoding/json"
	"errors"
	"testing"
)

// sampleID is a stub string-based value type proving the JSON/Text pattern.
type sampleID string

func (v sampleID) MarshalText() ([]byte, error) {
	s := Normalize(string(v))
	if IsEmpty(s) {
		return nil, ErrEmpty
	}
	return []byte(s), nil
}

func (v *sampleID) UnmarshalText(b []byte) error {
	s := Normalize(string(b))
	if IsEmpty(s) {
		return ErrEmpty
	}
	*v = sampleID(s)
	return nil
}

func (v sampleID) MarshalJSON() ([]byte, error)  { return MarshalJSONFromText(v) }
func (v *sampleID) UnmarshalJSON(b []byte) error { return UnmarshalJSONToText(b, v) }
func (v sampleID) Validate() error {
	if IsEmpty(string(v)) {
		return ErrEmpty
	}
	return nil
}

func TestIDs_TextAndJSONRoundTrip(t *testing.T) {
	var v sampleID

	if err := v.UnmarshalText([]byte("  run-42  ")); err != nil {
		t.Fatalf("unmarshal text: %v", err)
	}
	if string(v) != "run-42" {
		t.Fatalf("normalize failed: got %q", string(v))
	}

	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal json: %v", err)
	}
	if string(b) != "\"run-42\"" {
		t.Fatalf("marshal json mismatch: %s", string(b))
	}

	var v2 sampleID
	if err := json.Unmarshal(b, &v2); err != nil {
		t.Fatalf("unmarshal json: %v", err)
	}
	if v2 != v {
		t.Fatalf("roundtrip mismatch: %q != %q", v2, v)
	}
}

func TestIDs_Validation(t *testing.T) {
	var v sampleID
	if err := v.UnmarshalText([]byte("   ")); !errors.Is(err, ErrEmpty) {
		t.Fatalf("expected ErrEmpty, got %v", err)
	}
	v = sampleID("ok")
	if err := v.Validate(); err != nil {
		t.Fatalf("validate ok: %v", err)
	}
}
