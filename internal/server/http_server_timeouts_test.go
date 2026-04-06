package server

import (
	"context"
	"testing"
	"time"

	"github.com/iw2rmb/ploy/internal/server/config"
)

func TestHTTPServer_Timeouts(t *testing.T) {
	tests := []struct {
		name         string
		readTimeout  time.Duration
		writeTimeout time.Duration
		idleTimeout  time.Duration
	}{
		{"default_timeouts", 15 * time.Second, 15 * time.Second, 60 * time.Second},
		{"custom_timeouts", 5 * time.Second, 10 * time.Second, 60 * time.Second},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := newTestServer(t, config.HTTPConfig{
				Listen:       "127.0.0.1:0",
				ReadTimeout:  tt.readTimeout,
				WriteTimeout: tt.writeTimeout,
				IdleTimeout:  tt.idleTimeout,
			})

			ctx := context.Background()
			if err := srv.Start(ctx); err != nil {
				t.Fatalf("Start() error = %v", err)
			}
			defer func() { _ = srv.Stop(ctx) }()

			srv.mu.Lock()
			httpSrv := srv.httpServer
			srv.mu.Unlock()

			if httpSrv.ReadHeaderTimeout != httpReadHeaderTimeout {
				t.Errorf("ReadHeaderTimeout = %v, want %v", httpSrv.ReadHeaderTimeout, httpReadHeaderTimeout)
			}
			if httpSrv.ReadTimeout != tt.readTimeout {
				t.Errorf("ReadTimeout = %v, want %v", httpSrv.ReadTimeout, tt.readTimeout)
			}
			if httpSrv.WriteTimeout != tt.writeTimeout {
				t.Errorf("WriteTimeout = %v, want %v", httpSrv.WriteTimeout, tt.writeTimeout)
			}
			if httpSrv.IdleTimeout != tt.idleTimeout {
				t.Errorf("IdleTimeout = %v, want %v", httpSrv.IdleTimeout, tt.idleTimeout)
			}
		})
	}
}
