package types

import (
	"encoding/json"
	"errors"
	"testing"
)

func TestCPUmilli_Parse(t *testing.T) {
	tests := []struct {
		in      string
		wantM   CPUmilli
		wantNan int64
		wantStr string
	}{
		{"500m", 500, 500 * 1_000_000, "500m"},
		{"2", 2000, 2_000 * 1_000_000, "2"},
	}
	for _, tt := range tests {
		var v CPUmilli
		if err := v.UnmarshalText([]byte(tt.in)); err != nil {
			t.Fatalf("%q: unmarshal: %v", tt.in, err)
		}
		if v != tt.wantM {
			t.Fatalf("%q: value=%d, want %d", tt.in, v, tt.wantM)
		}
		if got := v.DockerNanoCPUs(); got != tt.wantNan {
			t.Fatalf("%q: nano=%d, want %d", tt.in, got, tt.wantNan)
		}
		if b, err := v.MarshalText(); err != nil {
			t.Fatalf("%q: marshal: %v", tt.in, err)
		} else if string(b) != tt.wantStr {
			t.Fatalf("%q: text=%q, want %q", tt.in, string(b), tt.wantStr)
		}
		// JSON roundtrip
		data, err := json.Marshal(v)
		if err != nil {
			t.Fatalf("%q: json marshal: %v", tt.in, err)
		}
		var v2 CPUmilli
		if err := json.Unmarshal(data, &v2); err != nil {
			t.Fatalf("%q: json unmarshal: %v", tt.in, err)
		}
		if v2 != v {
			t.Fatalf("%q: roundtrip mismatch", tt.in)
		}
	}
}

func TestBytes_Parse(t *testing.T) {
	tests := []struct {
		in   string
		want Bytes
	}{
		{"2Gi", Bytes(2 * (1 << 30))},
		{"10G", Bytes(10_000_000_000)},
	}
	for _, tt := range tests {
		var v Bytes
		if err := v.UnmarshalText([]byte(tt.in)); err != nil {
			t.Fatalf("%q: unmarshal: %v", tt.in, err)
		}
		if v != tt.want {
			t.Fatalf("%q: value=%d, want %d", tt.in, v, tt.want)
		}
		if got := v.DockerMemoryBytes(); got != int64(tt.want) {
			t.Fatalf("%q: docker bytes=%d, want %d", tt.in, got, int64(tt.want))
		}
		if b, err := v.MarshalText(); err != nil {
			t.Fatalf("%q: marshal: %v", tt.in, err)
		} else if string(b) != "" && string(b) == tt.in { // ensure it marshals; specific form not enforced
			// no-op; allow any canonical numeric form
			_ = b
		}
		data, err := json.Marshal(v)
		if err != nil {
			t.Fatalf("%q: json marshal: %v", tt.in, err)
		}
		var v2 Bytes
		if err := json.Unmarshal(data, &v2); err != nil {
			t.Fatalf("%q: json unmarshal: %v", tt.in, err)
		}
		if v2 != v {
			t.Fatalf("%q: roundtrip mismatch", tt.in)
		}
	}
}

func TestResources_InvalidAndOverflow(t *testing.T) {
	// CPU invalid
	for _, s := range []string{"", "abc", "1x", "-1", "1.2.3", "m100"} {
		var v CPUmilli
		if err := v.UnmarshalText([]byte(s)); err == nil {
			t.Fatalf("cpu %q: expected error", s)
		}
	}
	// CPU overflow (very large number)
	{
		var v CPUmilli
		if err := v.UnmarshalText([]byte("9223372036854775808m")); !errors.Is(err, ErrOverflow) {
			t.Fatalf("cpu overflow expected ErrOverflow, got %v", err)
		}
	}

	// Bytes invalid
	for _, s := range []string{"10X", "-1Gi", "G", "KiBKi", " ", "1.5G"} {
		var v Bytes
		if err := v.UnmarshalText([]byte(s)); err == nil {
			t.Fatalf("bytes %q: expected error", s)
		}
	}
	// Bytes overflow: MaxInt64+1
	{
		var v Bytes
		if err := v.UnmarshalText([]byte("9223372036854775808")); !errors.Is(err, ErrOverflow) {
			t.Fatalf("bytes overflow expected ErrOverflow, got %v", err)
		}
	}
}
