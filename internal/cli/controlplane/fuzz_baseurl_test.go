package controlplane

import (
	"strings"
	"testing"

	"github.com/iw2rmb/ploy/internal/cli/config"
)

// FuzzBaseURLFromDescriptor checks that various address shapes produce a URL
// or return an error only when address is empty.
func FuzzBaseURLFromDescriptor(f *testing.F) {
	seeds := []string{
		"203.0.113.10",
		"203.0.113.10:9000",
		"control.example.com",
		"control.example.com:1234",
		"https://control.example.com:8443",
		"http://[2001:db8::2]:8080",
		"[2001:db8::1]",
		"2001:db8::3",
		"",
	}
	for _, s := range seeds {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, address string) {
		desc := config.Descriptor{Address: address}
		url, err := BaseURLFromDescriptor(desc)
		if strings.TrimSpace(address) == "" {
			if err == nil {
				t.Fatalf("expected error for empty address, got url=%q", url)
			}
			return
		}
		if err != nil || strings.TrimSpace(url) == "" {
			t.Fatalf("unexpected error/url for %q: url=%q err=%v", address, url, err)
		}
		// If input was already a URL, expect it to be returned as-is.
		if strings.HasPrefix(address, "http://") || strings.HasPrefix(address, "https://") {
			if url != address {
				t.Fatalf("full URL should be preserved: %q -> %q", address, url)
			}
		}
	})
}
