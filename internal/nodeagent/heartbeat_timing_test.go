package nodeagent

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// TestSendHeartbeatRespectsTimeout verifies heartbeat request respects configured timeout.
func TestSendHeartbeatRespectsTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	cfg := Config{
		NodeID:    "test-node",
		ServerURL: srv.URL,
		HTTP: HTTPConfig{
			TLS: TLSConfig{
				Enabled: false,
			},
		},
		Heartbeat: HeartbeatConfig{
			Timeout: 10 * time.Millisecond,
		},
	}

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
func TestBackoffOn5xxErrors(t *testing.T) {
	tests := []struct {
		name           string
		statusCodes    []int
		wantBackoffs   []time.Duration
		wantFinalReset bool
	}{
		{
			name:         "single_5xx_starts_backoff",
			statusCodes:  []int{500},
			wantBackoffs: []time.Duration{5 * time.Second},
		},
		{
			name:         "consecutive_5xx_increases_backoff",
			statusCodes:  []int{500, 503, 502},
			wantBackoffs: []time.Duration{5 * time.Second, 10 * time.Second, 20 * time.Second},
		},
		{
			name:         "backoff_caps_at_max",
			statusCodes:  []int{500, 500, 500, 500, 500, 500, 500, 500},
			wantBackoffs: []time.Duration{5 * time.Second, 10 * time.Second, 20 * time.Second, 40 * time.Second, 80 * time.Second, 160 * time.Second, 5 * time.Minute, 5 * time.Minute},
		},
		{
			name:           "success_after_5xx_resets_backoff",
			statusCodes:    []int{500, 200},
			wantBackoffs:   []time.Duration{5 * time.Second, 0},
			wantFinalReset: true,
		},
		{
			name:         "4xx_does_not_trigger_backoff",
			statusCodes:  []int{400, 401, 404},
			wantBackoffs: []time.Duration{0, 0, 0},
		},
		{
			name:         "mixed_errors_only_backoff_on_5xx",
			statusCodes:  []int{500, 400, 503},
			wantBackoffs: []time.Duration{5 * time.Second, 5 * time.Second, 10 * time.Second},
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

			cfg := Config{
				NodeID:    "test-node",
				ServerURL: srv.URL,
				HTTP: HTTPConfig{
					TLS: TLSConfig{
						Enabled: false,
					},
				},
				Heartbeat: HeartbeatConfig{
					Timeout: 10 * time.Second,
				},
			}

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

				if mgr.backoffDuration != tt.wantBackoffs[i] {
					t.Errorf("request %d: backoff = %v, want %v", i, mgr.backoffDuration, tt.wantBackoffs[i])
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

	cfg := Config{
		NodeID:    "test-node",
		ServerURL: srv.URL,
		HTTP: HTTPConfig{
			TLS: TLSConfig{
				Enabled: false,
			},
		},
		Heartbeat: HeartbeatConfig{
			Timeout: 10 * time.Second,
		},
	}

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
	mgr := &HeartbeatManager{
		backoffDuration: 0,
		maxBackoff:      5 * time.Minute,
	}

	// Apply backoff with a non-5xx error.
	nonServerErr := fmt.Errorf("network timeout")
	mgr.applyBackoff(nonServerErr)

	if mgr.backoffDuration != 0 {
		t.Errorf("backoff = %v, want 0 for non-5xx error", mgr.backoffDuration)
	}

	// Apply backoff with a 4xx error.
	err4xx := fmt.Errorf("heartbeat failed with status 400")
	mgr.applyBackoff(err4xx)

	if mgr.backoffDuration != 0 {
		t.Errorf("backoff = %v, want 0 for 4xx error", mgr.backoffDuration)
	}
}
