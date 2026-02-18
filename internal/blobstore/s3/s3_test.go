package s3

import (
	"testing"

	"github.com/iw2rmb/ploy/internal/server/config"
)

func TestNormalizeEndpoint(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		raw    string
		secure bool
		want   string
	}{
		{name: "keeps http", raw: "http://garage:3900", secure: false, want: "http://garage:3900"},
		{name: "adds http", raw: "garage:3900", secure: false, want: "http://garage:3900"},
		{name: "adds https", raw: "garage:3900", secure: true, want: "https://garage:3900"},
		{name: "trims slash", raw: "https://garage:3900/", secure: true, want: "https://garage:3900"},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := normalizeEndpoint(tt.raw, tt.secure)
			if err != nil {
				t.Fatalf("normalizeEndpoint() error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("normalizeEndpoint() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestNew_RequiresFields(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		cfg  config.ObjectStoreConfig
	}{
		{name: "missing endpoint", cfg: config.ObjectStoreConfig{Bucket: "ploy", AccessKey: "a", SecretKey: "b"}},
		{name: "missing bucket", cfg: config.ObjectStoreConfig{Endpoint: "http://garage:3900", AccessKey: "a", SecretKey: "b"}},
		{name: "missing access key", cfg: config.ObjectStoreConfig{Endpoint: "http://garage:3900", Bucket: "ploy", SecretKey: "b"}},
		{name: "missing secret key", cfg: config.ObjectStoreConfig{Endpoint: "http://garage:3900", Bucket: "ploy", AccessKey: "a"}},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := New(tt.cfg)
			if err == nil {
				t.Fatalf("New() error = nil, want non-nil")
			}
		})
	}
}

func TestNew_DefaultRegion(t *testing.T) {
	t.Parallel()

	cfg := config.ObjectStoreConfig{
		Endpoint:  "garage:3900",
		Bucket:    "ploy",
		AccessKey: "key",
		SecretKey: "secret",
		Secure:    false,
	}

	store, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if store == nil {
		t.Fatalf("New() returned nil store")
	}
	if store.bucket != "ploy" {
		t.Fatalf("bucket = %q, want %q", store.bucket, "ploy")
	}
}
