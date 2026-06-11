package controlplane

import (
	"testing"
)

func TestBaseURLFromServerURL(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		want    string
		wantErr bool
	}{
		{
			name: "full URL preserved",
			raw:  "https://cp.example:9443",
			want: "https://cp.example:9443",
		},
		{
			name: "default scheme and port",
			raw:  "example.com",
			want: "http://example.com:8080",
		},
		{
			name: "host default scheme",
			raw:  "10.0.0.5",
			want: "http://10.0.0.5:8080",
		},
		{
			name: "address with port",
			raw:  "control.example.com:9000",
			want: "http://control.example.com:9000",
		},
		{
			name: "ipv6 default port",
			raw:  "2001:db8::1",
			want: "http://[2001:db8::1]:8080",
		},
		{
			name:    "env required",
			raw:     "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := BaseURLFromServerURL(tt.raw)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got url=%q", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("BaseURLFromServerURL error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("BaseURLFromServerURL() = %q, want %q", got, tt.want)
			}
		})
	}
}
