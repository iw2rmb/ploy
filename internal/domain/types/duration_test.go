package types

import (
	"encoding/json"
	"errors"
	"testing"
)

// sampleDuration is a stub proving JSON/Text and validation behavior.
type sampleDuration string

func (v sampleDuration) MarshalText() ([]byte, error) {
	s := Normalize(string(v))
	if IsEmpty(s) {
		return nil, ErrEmpty
	}
	return []byte(s), nil
}
func (v *sampleDuration) UnmarshalText(b []byte) error {
	s := Normalize(string(b))
	if IsEmpty(s) {
		return ErrEmpty
	}
	*v = sampleDuration(s)
	return nil
}
func (v sampleDuration) MarshalJSON() ([]byte, error)  { return MarshalJSONFromText(v) }
func (v *sampleDuration) UnmarshalJSON(b []byte) error { return UnmarshalJSONToText(b, v) }
func (v sampleDuration) Validate() error {
	if IsEmpty(string(v)) {
		return ErrEmpty
	}
	return nil
}

func TestDuration_TextAndJSONRoundTrip(t *testing.T) {
	var v sampleDuration
	if err := v.UnmarshalText([]byte("  5m  ")); err != nil {
		t.Fatalf("unmarshal text: %v", err)
	}
	if string(v) != "5m" {
		t.Fatalf("normalize failed: %q", string(v))
	}
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal json: %v", err)
	}
	var v2 sampleDuration
	if err := json.Unmarshal(b, &v2); err != nil {
		t.Fatalf("unmarshal json: %v", err)
	}
	if v2 != v {
		t.Fatalf("roundtrip mismatch")
	}
}

func TestDuration_Validation(t *testing.T) {
	var v sampleDuration
	if err := v.UnmarshalText([]byte("   ")); !errors.Is(err, ErrEmpty) {
		t.Fatalf("expected ErrEmpty, got %v", err)
	}
}
