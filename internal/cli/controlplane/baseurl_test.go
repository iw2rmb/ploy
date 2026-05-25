package controlplane

import (
	"testing"

	"github.com/iw2rmb/ploy/internal/cli/config"
)

func TestBaseURLFromDescriptor(t *testing.T) {
	tests := []struct {
		name    string
		desc    config.Descriptor
		want    string
		wantErr bool
	}{
		{
			name: "full URL preserved",
			desc: config.Descriptor{Address: "https://cp.example:9443"},
			want: "https://cp.example:9443",
		},
		{
			name: "default scheme and port",
			desc: config.Descriptor{Address: "example.com"},
			want: "http://example.com:8080",
		},
		{
			name: "scheme override",
			desc: config.Descriptor{Address: "10.0.0.5", Scheme: "http"},
			want: "http://10.0.0.5:8080",
		},
		{
			name: "address with port",
			desc: config.Descriptor{Address: "control.example.com:9000"},
			want: "http://control.example.com:9000",
		},
		{
			name: "ipv6 default port",
			desc: config.Descriptor{Address: "2001:db8::1"},
			want: "http://[2001:db8::1]:8080",
		},
		{
			name:    "address required",
			desc:    config.Descriptor{},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := BaseURLFromDescriptor(tt.desc)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got url=%q", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("BaseURLFromDescriptor error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("BaseURLFromDescriptor() = %q, want %q", got, tt.want)
			}
		})
	}
}
