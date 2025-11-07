package types

import (
	"encoding/json"
	"errors"
	"testing"
)

// sampleCID is a stub proving JSON/Text and validation behavior.
type sampleCID string

func (v sampleCID) MarshalText() ([]byte, error) {
	s := Normalize(string(v))
	if IsEmpty(s) {
		return nil, ErrEmpty
	}
	return []byte(s), nil
}
func (v *sampleCID) UnmarshalText(b []byte) error {
	s := Normalize(string(b))
	if IsEmpty(s) {
		return ErrEmpty
	}
	*v = sampleCID(s)
	return nil
}
func (v sampleCID) MarshalJSON() ([]byte, error)  { return MarshalJSONFromText(v) }
func (v *sampleCID) UnmarshalJSON(b []byte) error { return UnmarshalJSONToText(b, v) }
func (v sampleCID) Validate() error {
	if IsEmpty(string(v)) {
		return ErrEmpty
	}
	return nil
}

func TestArtifacts_TextAndJSONRoundTrip(t *testing.T) {
	var v sampleCID
	if err := v.UnmarshalText([]byte("  bafkreiabc  ")); err != nil {
		t.Fatalf("unmarshal text: %v", err)
	}
	if string(v) != "bafkreiabc" {
		t.Fatalf("normalize failed: %q", string(v))
	}
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal json: %v", err)
	}
	var v2 sampleCID
	if err := json.Unmarshal(b, &v2); err != nil {
		t.Fatalf("unmarshal json: %v", err)
	}
	if v2 != v {
		t.Fatalf("roundtrip mismatch")
	}
}

func TestArtifacts_Validation(t *testing.T) {
	var v sampleCID
	if err := v.UnmarshalText([]byte("   ")); !errors.Is(err, ErrEmpty) {
		t.Fatalf("expected ErrEmpty, got %v", err)
	}
}
