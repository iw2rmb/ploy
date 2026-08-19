package pki

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/iw2rmb/ploy/internal/server/config"
)

// Rotator renews node certificates.
type Rotator interface {
	Renew(ctx context.Context, cfg config.PKIConfig) error
}

// Options configure the PKI renewal task.
type Options struct {
	Config  config.PKIConfig
	Rotator Rotator
	Logger  *slog.Logger
}

// RenewalTask checks the active certificate at the configured renewal interval.
type RenewalTask struct {
	cfg      config.PKIConfig
	rotator  Rotator
	logger   *slog.Logger
	interval time.Duration
}

// New constructs a PKI renewal task.
func New(opts Options) (*RenewalTask, error) {
	if opts.Rotator == nil {
		return nil, errors.New("pki: rotator required")
	}
	interval := opts.Config.RenewBefore
	if interval <= 0 {
		interval = time.Hour
	}
	if interval < 10*time.Millisecond {
		interval = 10 * time.Millisecond
	}
	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}
	return &RenewalTask{
		cfg:      opts.Config,
		rotator:  opts.Rotator,
		logger:   logger,
		interval: interval,
	}, nil
}

func (t *RenewalTask) Name() string {
	return "pki-renewal"
}

func (t *RenewalTask) Interval() time.Duration {
	return t.interval
}

func (t *RenewalTask) Run(ctx context.Context) error {
	if t == nil || t.rotator == nil {
		return nil
	}
	if err := t.rotator.Renew(ctx, t.cfg); err != nil {
		t.logger.Error("pki renew failed", "err", err)
		return err
	}
	return nil
}
