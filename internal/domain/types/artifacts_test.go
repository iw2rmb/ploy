package types

import (
	"encoding/json"
	"errors"
	"testing"
)

func TestCID_TextAndJSONRoundTrip(t *testing.T) {
	var v CID
	if err := v.UnmarshalText([]byte("  bafkreiabc  ")); err != nil {
		t.Fatalf("unmarshal text: %v", err)
	}
	if string(v) != "bafkreiabc" {
		t.Fatalf("normalize failed: %q", string(v))
	}
	if err := v.Validate(); err != nil {
		t.Fatalf("validate: %v", err)
	}
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal json: %v", err)
	}
	var v2 CID
	if err := json.Unmarshal(b, &v2); err != nil {
		t.Fatalf("unmarshal json: %v", err)
	}
	if v2 != v {
		t.Fatalf("roundtrip mismatch")
	}
}

func TestCID_EmptyRejected(t *testing.T) {
	var v CID
	if err := v.UnmarshalText([]byte("   ")); !errors.Is(err, ErrEmpty) {
		t.Fatalf("expected ErrEmpty, got %v", err)
	}
}

func TestSha256Digest_ValidAndJSON(t *testing.T) {
	const hex64 = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	var d Sha256Digest
	if err := d.UnmarshalText([]byte("  sha256:" + hex64 + "  ")); err != nil {
		t.Fatalf("unmarshal text: %v", err)
	}
	if err := d.Validate(); err != nil {
		t.Fatalf("validate: %v", err)
	}
	b, err := json.Marshal(d)
	if err != nil {
		t.Fatalf("marshal json: %v", err)
	}
	var d2 Sha256Digest
	if err := json.Unmarshal(b, &d2); err != nil {
		t.Fatalf("unmarshal json: %v", err)
	}
	if d2 != d {
		t.Fatalf("roundtrip mismatch")
	}
}

func TestSha256Digest_Invalid(t *testing.T) {
	cases := map[string]string{
		"empty":         "",
		"no-prefix":     "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		"bad-prefix":    "sha512:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		"short":         "sha256:abc",
		"long":          "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef00",
		"bad-chars":     "sha256:zzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzz",
		"uppercase-hex": "sha256:ABCDEF0123456789ABCDEF0123456789ABCDEF0123456789ABCDEF0123456789",
	}
	for name, input := range cases {
		t.Run(name, func(t *testing.T) {
			var d Sha256Digest
			err := d.UnmarshalText([]byte(input))
			if input == "" {
				if !errors.Is(err, ErrEmpty) {
					t.Fatalf("expected ErrEmpty, got %v", err)
				}
				return
			}
			if !errors.Is(err, ErrInvalidDigest) {
				t.Fatalf("expected ErrInvalidDigest, got %v", err)
			}
		})
	}
}
