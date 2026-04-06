package pki_test

import (
	"context"
	"testing"
	"time"

	"github.com/iw2rmb/ploy/internal/server/config"
	"github.com/iw2rmb/ploy/internal/server/pki"
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

type stubRotator struct {
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

func TestNewRequiresRotator(t *testing.T) {
	_, err := pki.New(pki.Options{
		Config: config.PKIConfig{BundleDir: "/etc/ploy/pki"},
	})
	if err == nil {
		t.Fatal("expected error when rotator is nil")
	}
}

func TestStartAlreadyRunning(t *testing.T) {
	manager, err := pki.New(pki.Options{
		Config:  config.PKIConfig{BundleDir: "/etc/ploy/pki", RenewBefore: time.Hour},
		Rotator: &stubRotator{ch: make(chan struct{}, 1)},
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	ctx := context.Background()
	if err := manager.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer func() {
		_ = manager.Stop(context.Background())
	}()
	if err := manager.Start(ctx); err == nil {
		t.Fatal("expected error when starting already running manager")
	}
}

func TestStopNotRunning(t *testing.T) {
	manager, err := pki.New(pki.Options{
		Config:  config.PKIConfig{BundleDir: "/etc/ploy/pki"},
		Rotator: &stubRotator{ch: make(chan struct{}, 1)},
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if err := manager.Stop(context.Background()); err != nil {
		t.Fatalf("Stop() when not running should not error, got %v", err)
	}
}

func TestManagerLoop_EdgeCaseRenewBefore(t *testing.T) {
	tests := []struct {
		name        string
		renewBefore time.Duration
	}{
		{"zero", 0},
		{"negative", -10 * time.Millisecond},
		{"very small", 5 * time.Millisecond},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rotator := &stubRotator{ch: make(chan struct{}, 1)}
			manager, err := pki.New(pki.Options{
				Config: config.PKIConfig{
					BundleDir:   "/etc/ploy/pki",
					RenewBefore: tt.renewBefore,
				},
				Rotator: rotator,
			})
			if err != nil {
				t.Fatalf("New() error = %v", err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
			defer cancel()
			if err := manager.Start(ctx); err != nil {
				t.Fatalf("Start() error = %v", err)
			}
			select {
			case <-rotator.ch:
			case <-time.After(100 * time.Millisecond):
				t.Fatalf("expected rotation trigger with RenewBefore=%v", tt.renewBefore)
			}
			if err := manager.Stop(context.Background()); err != nil {
				t.Fatalf("Stop() error = %v", err)
			}
		})
	}
}
