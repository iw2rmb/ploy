package pki_test

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/iw2rmb/ploy/internal/server/config"
	"github.com/iw2rmb/ploy/internal/server/pki"
)

func TestDefaultRotatorRenew(t *testing.T) {
	rotator := pki.NewDefaultRotator(slog.Default())
	cfg := config.PKIConfig{
		BundleDir:   "/etc/ploy/pki",
		Certificate: "/etc/ploy/pki/node.pem",
		Key:         "/etc/ploy/pki/node-key.pem",
		RenewBefore: time.Hour,
	}
	ctx := context.Background()
	// Should not return error (stub implementation).
	if err := rotator.Renew(ctx, cfg); err != nil {
		t.Fatalf("Renew() error = %v, want nil", err)
	}
}

func TestDefaultRotatorWithNilLogger(t *testing.T) {
	rotator := pki.NewDefaultRotator(nil)
	if rotator == nil {
		t.Fatal("NewDefaultRotator(nil) returned nil, expected valid rotator")
	}
	cfg := config.PKIConfig{
		BundleDir:   "/etc/ploy/pki",
		RenewBefore: time.Hour,
	}
	ctx := context.Background()
	if err := rotator.Renew(ctx, cfg); err != nil {
		t.Fatalf("Renew() with nil logger error = %v, want nil", err)
	}
}
