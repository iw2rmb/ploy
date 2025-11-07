package types

import (
	"encoding/json"
	"errors"
	"strings"
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

func TestProtocol_AcceptKnown(t *testing.T) {
	cases := []string{"tcp", "udp", "TCP", "Udp"}
	for _, in := range cases {
		var p Protocol
		if err := p.UnmarshalText([]byte(in)); err != nil {
			t.Fatalf("UnmarshalText(%q) error: %v", in, err)
		}
		if s := string(p); s != strings.ToLower(strings.TrimSpace(in)) {
			t.Fatalf("canonical form mismatch: got %q for %q", s, in)
		}

		b, err := json.Marshal(p)
		if err != nil {
			t.Fatalf("marshal json: %v", err)
		}
		var p2 Protocol
		if err := json.Unmarshal(b, &p2); err != nil {
			t.Fatalf("unmarshal json: %v", err)
		}
		if p2 != p {
			t.Fatalf("roundtrip mismatch: %v != %v", p2, p)
		}
		if err := p2.Validate(); err != nil {
			t.Fatalf("validate known protocol: %v", err)
		}
	}
}

func TestProtocol_RejectUnknown(t *testing.T) {
	bad := []string{"", " ", "http", "icmp", "tcp/udp", "TLS"}
	for _, in := range bad {
		var p Protocol
		if err := p.UnmarshalText([]byte(in)); err == nil {
			t.Fatalf("expected error for %q", in)
		}
	}
}
