package types

import (
	"encoding/json"
	"errors"
	"testing"
)

// sampleResource is a stub proving JSON/Text and validation behavior.
type sampleResource string

func (v sampleResource) MarshalText() ([]byte, error) {
	s := Normalize(string(v))
	if IsEmpty(s) {
		return nil, ErrEmpty
	}
	return []byte(s), nil
}
func (v *sampleResource) UnmarshalText(b []byte) error {
	s := Normalize(string(b))
	if IsEmpty(s) {
		return ErrEmpty
	}
	*v = sampleResource(s)
	return nil
}
func (v sampleResource) MarshalJSON() ([]byte, error)  { return MarshalJSONFromText(v) }
func (v *sampleResource) UnmarshalJSON(b []byte) error { return UnmarshalJSONToText(b, v) }
func (v sampleResource) Validate() error {
	if IsEmpty(string(v)) {
		return ErrEmpty
	}
	return nil
}

func TestResources_TextAndJSONRoundTrip(t *testing.T) {
	var v sampleResource
	if err := v.UnmarshalText([]byte("  500m  ")); err != nil {
		t.Fatalf("unmarshal text: %v", err)
	}
	if string(v) != "500m" {
		t.Fatalf("normalize failed: %q", string(v))
	}
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal json: %v", err)
	}
	var v2 sampleResource
	if err := json.Unmarshal(b, &v2); err != nil {
		t.Fatalf("unmarshal json: %v", err)
	}
	if v2 != v {
		t.Fatalf("roundtrip mismatch")
	}
}

func TestResources_Validation(t *testing.T) {
	var v sampleResource
	if err := v.UnmarshalText([]byte("   ")); !errors.Is(err, ErrEmpty) {
		t.Fatalf("expected ErrEmpty, got %v", err)
	}
}
