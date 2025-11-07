package config

import (
	"testing"
)

func TestValidateAddress_Good(t *testing.T) {
	t.Parallel()
	tests := []struct {
		in   string
		port uint16
	}{
		{":8443", 8443},
		{"127.0.0.1:0", 0},
		{"127.0.0.1:8080", 8080},
		{"[::1]:443", 443},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.in, func(t *testing.T) {
			t.Parallel()
			ap, err := ValidateAddress(tt.in)
			if err != nil {
				t.Fatalf("ValidateAddress(%q) unexpected error: %v", tt.in, err)
			}
			if ap.Port() != tt.port {
				t.Fatalf("port = %d, want %d", ap.Port(), tt.port)
			}
		})
	}
}

func TestValidateAddress_Bad(t *testing.T) {
	t.Parallel()
	bad := []string{
		"",
		"127.0.0.1",       // missing port
		"localhost:8080",  // hostnames not allowed; require IP literal
		"127.0.0.1:abc",   // invalid port
		"[::1]",           // missing port
		"127.0.0.1:70000", // port out of range
	}
	for _, in := range bad {
		in := in
		t.Run(in, func(t *testing.T) {
			t.Parallel()
			if _, err := ValidateAddress(in); err == nil {
				t.Fatalf("ValidateAddress(%q) = nil error, want failure", in)
			}
		})
	}
}
