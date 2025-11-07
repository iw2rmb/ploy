package types

import (
	"encoding/json"
	"errors"
	"testing"
)

// sampleProtocol is a stub proving JSON/Text and validation behavior.
type sampleProtocol string

func (v sampleProtocol) MarshalText() ([]byte, error) {
	s := Normalize(string(v))
	if IsEmpty(s) {
		return nil, ErrEmpty
	}
	return []byte(s), nil
}
func (v *sampleProtocol) UnmarshalText(b []byte) error {
	s := Normalize(string(b))
	if IsEmpty(s) {
		return ErrEmpty
	}
	*v = sampleProtocol(s)
	return nil
}
func (v sampleProtocol) MarshalJSON() ([]byte, error)  { return MarshalJSONFromText(v) }
func (v *sampleProtocol) UnmarshalJSON(b []byte) error { return UnmarshalJSONToText(b, v) }
func (v sampleProtocol) Validate() error {
	if IsEmpty(string(v)) {
		return ErrEmpty
	}
	return nil
}

func TestNetwork_TextAndJSONRoundTrip(t *testing.T) {
	var v sampleProtocol
	if err := v.UnmarshalText([]byte("  tcp  ")); err != nil {
		t.Fatalf("unmarshal text: %v", err)
	}
	if string(v) != "tcp" {
		t.Fatalf("normalize failed: %q", string(v))
	}
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal json: %v", err)
	}
	var v2 sampleProtocol
	if err := json.Unmarshal(b, &v2); err != nil {
		t.Fatalf("unmarshal json: %v", err)
	}
	if v2 != v {
		t.Fatalf("roundtrip mismatch")
	}
}

func TestNetwork_Validation(t *testing.T) {
	var v sampleProtocol
	if err := v.UnmarshalText([]byte("   ")); !errors.Is(err, ErrEmpty) {
		t.Fatalf("expected ErrEmpty, got %v", err)
	}
}
