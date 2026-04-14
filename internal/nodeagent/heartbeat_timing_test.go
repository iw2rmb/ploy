package nodeagent

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	types "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/workflow/backoff"
)

// TestSendHeartbeatRespectsTimeout verifies heartbeat request respects configured timeout.
func TestSendHeartbeatRespectsTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	cfg := newAgentConfig(srv.URL, withHeartbeatTimeout(10*time.Millisecond))

	mgr, err := NewHeartbeatManager(cfg)
	if err != nil {
		t.Fatalf("NewHeartbeatManager error: %v", err)
	}

	ctx := context.Background()
	err = mgr.sendHeartbeat(ctx)
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
}

// TestBackoffOn5xxErrors verifies exponential backoff behavior on 5xx server errors.
// With shared StatefulBackoff, intervals include 50% jitter and grow exponentially.
func TestBackoffOn5xxErrors(t *testing.T) {
	tests := []struct {
		name           string
		statusCodes    []int
		wantBackoffs   []struct{ min, max time.Duration } // Jitter ranges for backoff durations
		wantFinalReset bool
	}{
		{
			name:        "single_5xx_starts_backoff",
			statusCodes: []int{500},
			// First backoff: 5s initial with 50% jitter => [2.5s, 7.5s]
			wantBackoffs: []struct{ min, max time.Duration }{{2500 * time.Millisecond, 7500 * time.Millisecond}},
		},
		{
			name:        "consecutive_5xx_increases_backoff",
			statusCodes: []int{500, 503, 502},
			// 1st: 5s ±50% => [2.5s, 7.5s], 2nd: 10s ±50% => [5s, 15s], 3rd: 20s ±50% => [10s, 30s]
			wantBackoffs: []struct{ min, max time.Duration }{
				{2500 * time.Millisecond, 7500 * time.Millisecond},
				{5 * time.Second, 15 * time.Second},
				{10 * time.Second, 30 * time.Second},
			},
		},
		{
			name:        "backoff_caps_at_max",
			statusCodes: []int{500, 500, 500, 500, 500, 500, 500, 500},
			// Grows: 5s, 10s, 20s, 40s, 80s, 160s, then caps at 5m (300s) ±50% => [150s, 450s]
			wantBackoffs: []struct{ min, max time.Duration }{
				{2500 * time.Millisecond, 7500 * time.Millisecond}, // 5s ±50%
				{5 * time.Second, 15 * time.Second},                // 10s ±50%
				{10 * time.Second, 30 * time.Second},               // 20s ±50%
				{20 * time.Second, 60 * time.Second},               // 40s ±50%
				{40 * time.Second, 120 * time.Second},              // 80s ±50%
				{80 * time.Second, 240 * time.Second},              // 160s ±50%
				{150 * time.Second, 450 * time.Second},             // 300s (5m) ±50%
				{150 * time.Second, 450 * time.Second},             // Still capped at 5m ±50%
			},
		},
		{
			name:        "success_after_5xx_resets_backoff",
			statusCodes: []int{500, 200},
			// First 5xx triggers backoff, then success resets to 0 (backoffActive=false).
			wantBackoffs: []struct{ min, max time.Duration }{
				{2500 * time.Millisecond, 7500 * time.Millisecond}, // 5s ±50%
				{0, 0}, // Reset on success
			},
			wantFinalReset: true,
		},
		{
			name:        "4xx_does_not_trigger_backoff",
			statusCodes: []int{400, 401, 404},
			// 4xx errors do not trigger backoff (backoffActive stays false).
			wantBackoffs: []struct{ min, max time.Duration }{{0, 0}, {0, 0}, {0, 0}},
		},
		{
			name:        "mixed_errors_only_backoff_on_5xx",
			statusCodes: []int{500, 400, 503},
			// 1st 5xx: backoff to 5s ±50%, 2nd 4xx: no change (still 5s ±50%), 3rd 5xx: advance to 10s ±50%
			wantBackoffs: []struct{ min, max time.Duration }{
				{2500 * time.Millisecond, 7500 * time.Millisecond}, // 5s ±50%
				{2500 * time.Millisecond, 7500 * time.Millisecond}, // No change (4xx doesn't trigger backoff)
				{5 * time.Second, 15 * time.Second},                // 10s ±50%
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			callCount := 0
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if callCount < len(tt.statusCodes) {
					w.WriteHeader(tt.statusCodes[callCount])
					callCount++
				} else {
					w.WriteHeader(http.StatusOK)
				}
			}))
			defer srv.Close()

			cfg := newAgentConfig(srv.URL, withHeartbeatTimeout(10*time.Second))

			mgr, err := NewHeartbeatManager(cfg)
			if err != nil {
				t.Fatalf("NewHeartbeatManager error: %v", err)
			}

			ctx := context.Background()

			for i, statusCode := range tt.statusCodes {
				err := mgr.sendHeartbeat(ctx)

				if statusCode >= 200 && statusCode < 300 {
					if err != nil {
						t.Errorf("request %d: unexpected error for success status %d: %v", i, statusCode, err)
					}
					mgr.resetBackoff()
				} else {
					if err == nil {
						t.Errorf("request %d: expected error for status %d, got nil", i, statusCode)
					}
					mgr.applyBackoff(err)
				}

				// Verify backoff duration falls within expected jitter range.
				wantRange := tt.wantBackoffs[i]
				var actualDuration time.Duration
				if mgr.backoffActive {
					actualDuration = time.Duration(mgr.backoff.GetDuration())
				} else {
					actualDuration = 0
				}

				if actualDuration < wantRange.min || actualDuration > wantRange.max {
					t.Errorf("request %d: backoff = %v, want in range [%v, %v]", i, actualDuration, wantRange.min, wantRange.max)
				}

				// Verify backoffActive state for reset case.
				if tt.wantFinalReset && i == len(tt.statusCodes)-1 {
					if mgr.backoffActive {
						t.Errorf("request %d: backoffActive = true, want false after reset", i)
					}
				}
			}
		})
	}
}

// TestServerErrorType verifies 5xx errors are wrapped in serverError type for backoff logic.
func TestServerErrorType(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	cfg := newAgentConfig(srv.URL, withHeartbeatTimeout(10*time.Second))

	mgr, err := NewHeartbeatManager(cfg)
	if err != nil {
		t.Fatalf("NewHeartbeatManager error: %v", err)
	}

	ctx := context.Background()
	err = mgr.sendHeartbeat(ctx)

	if err == nil {
		t.Fatal("expected error for 500 status, got nil")
	}

	var srvErr *serverError
	if !errors.As(err, &srvErr) {
		t.Errorf("error type = %T, want *serverError", err)
	}

	if srvErr.statusCode != 500 {
		t.Errorf("statusCode = %d, want 500", srvErr.statusCode)
	}
}

// TestBackoffDoesNotApplyToNon5xxErrors verifies backoff only applies to 5xx server errors.
func TestBackoffDoesNotApplyToNon5xxErrors(t *testing.T) {
	cfg := newAgentConfig("")
	mgr, err := NewHeartbeatManager(cfg)
	if err != nil {
		t.Fatalf("NewHeartbeatManager error: %v", err)
	}

	// Apply backoff with a non-5xx error (no serverError wrapper).
	nonServerErr := fmt.Errorf("network timeout")
	mgr.applyBackoff(nonServerErr)

	// Backoff should remain inactive (not triggered by non-5xx errors).
	if mgr.backoffActive {
		t.Errorf("backoffActive = true, want false for non-5xx error")
	}

	// Apply backoff with a 4xx error (no serverError wrapper).
	err4xx := fmt.Errorf("heartbeat failed with status 400")
	mgr.applyBackoff(err4xx)

	// Backoff should still be inactive (4xx does not trigger backoff).
	if mgr.backoffActive {
		t.Errorf("backoffActive = true, want false for 4xx error")
	}
}

func TestHeartbeatStart_BackoffOverridesInterval(t *testing.T) {
	t.Parallel()

	var (
		first  time.Time
		second time.Time
		n      int
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n++
		switch n {
		case 1:
			first = time.Now()
			w.WriteHeader(http.StatusInternalServerError)
		case 2:
			second = time.Now()
			w.WriteHeader(http.StatusOK)
			cancel()
		default:
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer srv.Close()

	cfg := newAgentConfig(srv.URL,
		withHeartbeatInterval(2*time.Second),
		withHeartbeatTimeout(5*time.Second))

	mgr, err := NewHeartbeatManager(cfg)
	if err != nil {
		t.Fatalf("NewHeartbeatManager error: %v", err)
	}

	mgr.backoff = backoff.NewStatefulBackoff(backoff.Policy{
		InitialInterval: types.Duration(10 * time.Millisecond),
		MaxInterval:     types.Duration(10 * time.Millisecond),
		Multiplier:      1.0,
		MaxElapsedTime:  0,
		MaxAttempts:     0,
	})

	err = mgr.Start(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Start error = %v, want context.Canceled", err)
	}

	if n < 2 {
		t.Fatalf("heartbeat requests = %d, want >= 2", n)
	}

	delta := second.Sub(first)
	if delta >= time.Second {
		t.Fatalf("second heartbeat delay = %v, want < 1s (interval=%v)", delta, cfg.Heartbeat.Interval)
	}
}
