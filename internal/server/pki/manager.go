package pki

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/iw2rmb/ploy/internal/server/config"
)

// Rotator renews node certificates.
type Rotator interface {
	Renew(ctx context.Context, cfg config.PKIConfig) error
}

// Options configure the PKI manager.
type Options struct {
	Config  config.PKIConfig
	Rotator Rotator
}

// Manager supervises certificate renewal.
type Manager struct {
	mu      sync.Mutex
	cfg     config.PKIConfig
	rotator Rotator
	ctx     context.Context
	cancel  context.CancelFunc
	group   sync.WaitGroup
	running bool
}

// New constructs the PKI manager instance.
func New(opts Options) (*Manager, error) {
	if opts.Rotator == nil {
		return nil, errors.New("pki: rotator required")
	}
	return &Manager{
		cfg:     opts.Config,
		rotator: opts.Rotator,
	}, nil
}

// Start begins renewal monitoring.
func (m *Manager) Start(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.running {
		return errors.New("pki: already running")
	}
	m.ctx, m.cancel = context.WithCancel(ctx)
	m.running = true
	m.group.Add(1)
	go m.loop()
	return nil
}

// Stop terminates monitoring.
func (m *Manager) Stop(ctx context.Context) error {
	m.mu.Lock()
	if !m.running {
		m.mu.Unlock()
		return nil
	}
	cancel := m.cancel
	m.cancel = nil
	m.running = false
	m.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	done := make(chan struct{})
	go func() {
		m.group.Wait()
		close(done)
	}()

	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Config returns the active configuration snapshot.
func (m *Manager) Config() config.PKIConfig {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.cfg
}

func (m *Manager) loop() {
	defer m.group.Done()
	for {
		cfg, ctx := m.state()
		if ctx.Err() != nil {
			return
		}
		if err := m.rotator.Renew(ctx, cfg); err != nil {
			// Background task: log at the edge.
			slog.Error("pki renew failed", "err", err)
		}

		delay := cfg.RenewBefore
		if delay <= 0 {
			delay = time.Hour
		}
		if delay < 10*time.Millisecond {
			delay = 10 * time.Millisecond
		}
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}

func (m *Manager) state() (config.PKIConfig, context.Context) {
	m.mu.Lock()
	cfg := m.cfg
	ctx := m.ctx
	m.mu.Unlock()
	return cfg, ctx
}
