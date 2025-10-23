package pki_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/iw2rmb/ploy/internal/ployd/config"
	"github.com/iw2rmb/ploy/internal/ployd/pki"
)

func TestManagerTriggersRenewal(t *testing.T) {
	rotator := &stubRotator{ch: make(chan struct{}, 1)}
	cfg := config.PKIConfig{
		BundleDir:   "/etc/ploy/pki",
		Certificate: "/etc/ploy/pki/node.pem",
		Key:         "/etc/ploy/pki/node-key.pem",
		RenewBefore: 20 * time.Millisecond,
	}
	manager, err := pki.New(pki.Options{
		Config:  cfg,
		Rotator: rotator,
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := manager.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	select {
	case <-rotator.ch:
	case <-time.After(200 * time.Millisecond):
		cancel()
		t.Fatal("expected rotation trigger")
	}
	if err := manager.Stop(context.Background()); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
}

func TestManagerReloadUpdatesConfig(t *testing.T) {
	manager, err := pki.New(pki.Options{
		Config: config.PKIConfig{
			BundleDir: "/etc/ploy/pki",
			RenewBefore: 10 * time.Minute,
		},
		Rotator: &stubRotator{ch: make(chan struct{}, 1)},
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	updated := config.PKIConfig{BundleDir: "/var/lib/ploy/pki", RenewBefore: time.Hour}
	if err := manager.Reload(context.Background(), updated); err != nil {
		t.Fatalf("Reload() error = %v", err)
	}
	if manager.Config().BundleDir != "/var/lib/ploy/pki" {
		t.Fatalf("expected bundle dir updated, got %s", manager.Config().BundleDir)
	}
}

type stubRotator struct {
	mu sync.Mutex
	ch chan struct{}
}

func (s *stubRotator) Renew(ctx context.Context, cfg config.PKIConfig) error {
	_ = cfg
	select {
	case <-ctx.Done():
	default:
	}
	if s.ch != nil {
		select {
		case s.ch <- struct{}{}:
		default:
		}
	}
	return nil
}
