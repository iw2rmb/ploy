package pki_test

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/iw2rmb/ploy/internal/server/config"
	"github.com/iw2rmb/ploy/internal/server/pki"
	"github.com/iw2rmb/ploy/internal/server/scheduler"
)

func TestRenewalTaskRunsUnderScheduler(t *testing.T) {
	rotator := &stubRotator{called: make(chan struct{}, 1)}
	task, err := pki.New(pki.Options{
		Config:  config.PKIConfig{RenewBefore: time.Hour},
		Rotator: rotator,
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if task.Name() != "pki-renewal" {
		t.Fatalf("Name() = %q, want %q", task.Name(), "pki-renewal")
	}
	sched := scheduler.New()
	sched.AddTask(task)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := sched.Start(ctx); err != nil {
		t.Fatalf("scheduler Start() error = %v", err)
	}
	select {
	case <-rotator.called:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("scheduler did not run PKI renewal task")
	}
	if err := sched.Stop(context.Background()); err != nil {
		t.Fatalf("scheduler Stop() error = %v", err)
	}
	if rotator.calls != 1 {
		t.Fatalf("rotator calls = %d, want 1", rotator.calls)
	}
}

func TestNewRequiresRotator(t *testing.T) {
	if _, err := pki.New(pki.Options{}); err == nil {
		t.Fatal("expected error when rotator is nil")
	}
}

func TestRenewalTaskNormalizesInterval(t *testing.T) {
	tests := []struct {
		name        string
		renewBefore time.Duration
		want        time.Duration
	}{
		{name: "zero uses default", want: time.Hour},
		{name: "negative uses default", renewBefore: -time.Second, want: time.Hour},
		{name: "very small uses minimum", renewBefore: 5 * time.Millisecond, want: 10 * time.Millisecond},
		{name: "configured interval", renewBefore: 20 * time.Millisecond, want: 20 * time.Millisecond},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			task, err := pki.New(pki.Options{
				Config:  config.PKIConfig{RenewBefore: tt.renewBefore},
				Rotator: &stubRotator{},
			})
			if err != nil {
				t.Fatalf("New() error = %v", err)
			}
			if got := task.Interval(); got != tt.want {
				t.Fatalf("Interval() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRenewalTaskLogsAndReturnsRotatorError(t *testing.T) {
	var logs bytes.Buffer
	wantErr := errors.New("renewal failed")
	task, err := pki.New(pki.Options{
		Rotator: &stubRotator{err: wantErr},
		Logger:  slog.New(slog.NewTextHandler(&logs, nil)),
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if err := task.Run(context.Background()); !errors.Is(err, wantErr) {
		t.Fatalf("Run() error = %v, want %v", err, wantErr)
	}
	if got := logs.String(); !strings.Contains(got, "pki renew failed") || !strings.Contains(got, wantErr.Error()) {
		t.Fatalf("log = %q, want renewal failure", got)
	}
}

type stubRotator struct {
	calls  int
	err    error
	called chan struct{}
}

func (s *stubRotator) Renew(context.Context, config.PKIConfig) error {
	s.calls++
	if s.called != nil {
		s.called <- struct{}{}
	}
	return s.err
}
