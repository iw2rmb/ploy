package types

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

// sampleLevel is a stub proving JSON/Text and validation behavior.
type sampleLevel string

func (v sampleLevel) MarshalText() ([]byte, error) {
	s := Normalize(string(v))
	if IsEmpty(s) {
		return nil, ErrEmpty
	}
	return []byte(s), nil
}
func (v *sampleLevel) UnmarshalText(b []byte) error {
	s := Normalize(string(b))
	if IsEmpty(s) {
		return ErrEmpty
	}
	*v = sampleLevel(s)
	return nil
}
func (v sampleLevel) MarshalJSON() ([]byte, error)  { return MarshalJSONFromText(v) }
func (v *sampleLevel) UnmarshalJSON(b []byte) error { return UnmarshalJSONToText(b, v) }
func (v sampleLevel) Validate() error {
	if IsEmpty(string(v)) {
		return ErrEmpty
	}
	return nil
}

func TestLogging_TextAndJSONRoundTrip(t *testing.T) {
	var v sampleLevel
	if err := v.UnmarshalText([]byte("  info  ")); err != nil {
		t.Fatalf("unmarshal text: %v", err)
	}
	if string(v) != "info" {
		t.Fatalf("normalize failed: %q", string(v))
	}
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal json: %v", err)
	}
	var v2 sampleLevel
	if err := json.Unmarshal(b, &v2); err != nil {
		t.Fatalf("unmarshal json: %v", err)
	}
	if v2 != v {
		t.Fatalf("roundtrip mismatch")
	}
}

func TestLogging_Validation(t *testing.T) {
	var v sampleLevel
	if err := v.UnmarshalText([]byte("   ")); !errors.Is(err, ErrEmpty) {
		t.Fatalf("expected ErrEmpty, got %v", err)
	}
}

func TestLogLevel_AcceptKnown(t *testing.T) {
	cases := []string{"debug", "info", "warn", "error", "DEBUG", "Warn", " Error "}
	for _, in := range cases {
		var lvl LogLevel
		if err := lvl.UnmarshalText([]byte(in)); err != nil {
			t.Fatalf("UnmarshalText(%q) error: %v", in, err)
		}
		if s := string(lvl); s != strings.ToLower(strings.TrimSpace(in)) {
			t.Fatalf("canonical form mismatch: got %q for %q", s, in)
		}

		b, err := json.Marshal(lvl)
		if err != nil {
			t.Fatalf("marshal json: %v", err)
		}
		var lvl2 LogLevel
		if err := json.Unmarshal(b, &lvl2); err != nil {
			t.Fatalf("unmarshal json: %v", err)
		}
		if lvl2 != lvl {
			t.Fatalf("roundtrip mismatch: %v != %v", lvl2, lvl)
		}
		if err := lvl2.Validate(); err != nil {
			t.Fatalf("validate known level: %v", err)
		}
	}
}

func TestLogLevel_RejectUnknown(t *testing.T) {
	bad := []string{"", " ", "trace", "fatal", "warning", "notice", "err"}
	for _, in := range bad {
		var lvl LogLevel
		if err := lvl.UnmarshalText([]byte(in)); err == nil {
			t.Fatalf("expected error for %q", in)
		}
	}
}
