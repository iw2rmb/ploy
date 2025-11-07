package types

import (
	"encoding/json"
	"errors"
	"testing"
)

// sampleRepo is a stub proving JSON/Text and validation behavior.
type sampleRepo string

func (v sampleRepo) MarshalText() ([]byte, error) {
	s := Normalize(string(v))
	if IsEmpty(s) {
		return nil, ErrEmpty
	}
	return []byte(s), nil
}
func (v *sampleRepo) UnmarshalText(b []byte) error {
	s := Normalize(string(b))
	if IsEmpty(s) {
		return ErrEmpty
	}
	*v = sampleRepo(s)
	return nil
}
func (v sampleRepo) MarshalJSON() ([]byte, error)  { return MarshalJSONFromText(v) }
func (v *sampleRepo) UnmarshalJSON(b []byte) error { return UnmarshalJSONToText(b, v) }
func (v sampleRepo) Validate() error {
	if IsEmpty(string(v)) {
		return ErrEmpty
	}
	return nil
}

func TestVCS_TextAndJSONRoundTrip(t *testing.T) {
	var v sampleRepo
	if err := v.UnmarshalText([]byte("  https://example/repo.git  ")); err != nil {
		t.Fatalf("unmarshal text: %v", err)
	}
	if string(v) != "https://example/repo.git" {
		t.Fatalf("normalize failed: %q", string(v))
	}
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal json: %v", err)
	}
	var v2 sampleRepo
	if err := json.Unmarshal(b, &v2); err != nil {
		t.Fatalf("unmarshal json: %v", err)
	}
	if v2 != v {
		t.Fatalf("roundtrip mismatch")
	}
}

func TestVCS_Validation(t *testing.T) {
	var v sampleRepo
	if err := v.UnmarshalText([]byte("   ")); !errors.Is(err, ErrEmpty) {
		t.Fatalf("expected ErrEmpty, got %v", err)
	}
}
