package types

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

func TestDuration_TextAndJSONRoundTrip(t *testing.T) {
	var d Duration
	if err := d.UnmarshalText([]byte("  5m  ")); err != nil {
		t.Fatalf("unmarshal text: %v", err)
	}
	if time.Duration(d) != 5*time.Minute {
		t.Fatalf("parsed value = %v, want %v", time.Duration(d), 5*time.Minute)
	}
	// Text form is canonicalized by time.Duration.String().
	if b, err := d.MarshalText(); err != nil {
		t.Fatalf("marshal text: %v", err)
	} else if string(b) != "5m0s" { // canonical form
		t.Fatalf("text form = %q, want %q", string(b), "5m0s")
	}

	// JSON string roundtrip.
	data, err := json.Marshal(d)
	if err != nil {
		t.Fatalf("marshal json: %v", err)
	}
	var d2 Duration
	if err := json.Unmarshal(data, &d2); err != nil {
		t.Fatalf("unmarshal json: %v", err)
	}
	if time.Duration(d2) != time.Duration(d) {
		t.Fatalf("roundtrip mismatch: %v != %v", d2, d)
	}
}

func TestDuration_JSONRejectsNonString(t *testing.T) {
	var d Duration
	if err := json.Unmarshal([]byte("12345"), &d); err == nil {
		t.Fatalf("expected error for non-string JSON value")
	}
}

func TestDuration_InvalidString(t *testing.T) {
	var d Duration
	if err := d.UnmarshalText([]byte("not-a-duration")); !errors.Is(err, ErrInvalidDuration) {
		t.Fatalf("expected ErrInvalidDuration, got %v", err)
	}
	if err := d.UnmarshalText([]byte("   ")); !errors.Is(err, ErrEmpty) {
		t.Fatalf("expected ErrEmpty, got %v", err)
	}
}

func TestDuration_YAML(t *testing.T) {
	// Unmarshal from YAML string.
	type cfg struct {
		Value Duration `yaml:"value"`
	}
	var c cfg
	y := "value: 90s\n"
	if err := yaml.Unmarshal([]byte(y), &c); err != nil {
		t.Fatalf("yaml unmarshal: %v", err)
	}
	if time.Duration(c.Value) != 90*time.Second {
		t.Fatalf("yaml parsed = %v, want %v", time.Duration(c.Value), 90*time.Second)
	}

	// Marshal to YAML string.
	out, err := yaml.Marshal(cfg{Value: Duration(90 * time.Second)})
	if err != nil {
		t.Fatalf("yaml marshal: %v", err)
	}
	// Expect a scalar string with canonical form.
	if string(out) != "value: 1m30s\n" {
		t.Fatalf("yaml out = %q", string(out))
	}

	// Invalid YAML value type (e.g., number) should error via our unmarshaler.
	if err := yaml.Unmarshal([]byte("value: 123\n"), &c); err == nil {
		t.Fatalf("expected error for non-string YAML scalar")
	}
}

func TestDurationStdRoundtrip(t *testing.T) {
	d := Duration(90 * time.Second)
	if StdDuration(d) != 90*time.Second {
		t.Fatalf("StdDuration mismatch: got %v", StdDuration(d))
	}
	if FromStdDuration(2*time.Minute) != Duration(2*time.Minute) {
		t.Fatalf("FromStdDuration mismatch: got %v", FromStdDuration(2*time.Minute))
	}
}

func TestStringsAdapter(t *testing.T) {
	in := []Protocol{ProtocolTCP, ProtocolUDP}
	got := Strings(in)
	want := []string{"tcp", "udp"}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("Strings() = %#v, want %#v", got, want)
	}
}

func TestStringPtrAdapter(t *testing.T) {
	if p := StringPtr(RunID("")); p != nil {
		t.Fatalf("StringPtr(empty) = %#v, want nil", *p)
	}
	id := RunID("abc")
	p := StringPtr(id)
	if p == nil || *p != "abc" {
		t.Fatalf("StringPtr(%q) = %#v, want %q", id, p, "abc")
	}
}
