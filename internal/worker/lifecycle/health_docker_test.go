package lifecycle

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/moby/moby/api/types/system"
	"github.com/moby/moby/client"
)

// fakeDocker implements dockerAPI for testing. When ctxAware is true,
// Ping and Info respect context cancellation before returning.
type fakeDocker struct {
	ping     client.PingResult
	pingErr  error
	info     system.Info
	infoErr  error
	ctxAware bool
}

func (f fakeDocker) Ping(ctx context.Context, _ client.PingOptions) (client.PingResult, error) {
	if f.ctxAware {
		select {
		case <-ctx.Done():
			return client.PingResult{}, ctx.Err()
		default:
		}
	}
	return f.ping, f.pingErr
}

func (f fakeDocker) Info(ctx context.Context, _ client.InfoOptions) (client.SystemInfoResult, error) {
	if f.ctxAware {
		select {
		case <-ctx.Done():
			return client.SystemInfoResult{}, ctx.Err()
		default:
		}
	}
	return client.SystemInfoResult{Info: f.info}, f.infoErr
}

func (f fakeDocker) Close() error { return nil }

var fixedClock = time.Date(2024, 12, 5, 10, 30, 0, 0, time.UTC)

func newTestChecker(t *testing.T, fd fakeDocker) *DockerChecker {
	t.Helper()
	c, err := NewDockerChecker(DockerCheckerOptions{
		Client:  fd,
		Timeout: 50 * time.Millisecond,
		Clock:   func() time.Time { return fixedClock },
	})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func assertDetails(t *testing.T, got map[string]any, wantAPI, wantOS, wantDriver string, wantRunning int) {
	t.Helper()
	if v, ok := got["api_version"]; !ok || v.(string) != wantAPI {
		t.Errorf("api_version: got %v, want %q", got["api_version"], wantAPI)
	}
	if v, ok := got["os_type"]; !ok || v.(string) != wantOS {
		t.Errorf("os_type: got %v, want %q", got["os_type"], wantOS)
	}
	if v, ok := got["driver"]; !ok || v.(string) != wantDriver {
		t.Errorf("driver: got %v, want %q", got["driver"], wantDriver)
	}
	if v, ok := got["containers_running"]; !ok || v.(int) != wantRunning {
		t.Errorf("containers_running: got %v, want %d", got["containers_running"], wantRunning)
	}
}

func TestDockerChecker_Check(t *testing.T) {
	tests := []struct {
		name        string
		fake        fakeDocker
		cancelCtx   bool
		wantState   ComponentState
		wantVersion string
		wantMsgSub  string
		wantAPI     string
		wantOS      string
		wantDriver  string
		wantRunning int
	}{
		// OK: cross-version and cross-platform.
		{
			name: "v28 linux overlay2",
			fake: fakeDocker{
				ping: client.PingResult{APIVersion: "1.44", OSType: "linux"},
				info: system.Info{ServerVersion: "28.0.0", Driver: "overlay2", ContainersRunning: 2},
			},
			wantState: StateOK, wantVersion: "28.0.0",
			wantAPI: "1.44", wantOS: "linux", wantDriver: "overlay2", wantRunning: 2,
		},
		{
			name: "v29 windows windowsfilter",
			fake: fakeDocker{
				ping: client.PingResult{APIVersion: "1.45", OSType: "windows"},
				info: system.Info{ServerVersion: "29.1.0", Driver: "windowsfilter", ContainersRunning: 5},
			},
			wantState: StateOK, wantVersion: "29.1.0",
			wantAPI: "1.45", wantOS: "windows", wantDriver: "windowsfilter", wantRunning: 5,
		},
		{
			name: "v28 linux vfs",
			fake: fakeDocker{
				ping: client.PingResult{APIVersion: "1.44", OSType: "linux"},
				info: system.Info{ServerVersion: "28.0.5", Driver: "vfs", ContainersRunning: 0},
			},
			wantState: StateOK, wantVersion: "28.0.5",
			wantAPI: "1.44", wantOS: "linux", wantDriver: "vfs", wantRunning: 0,
		},
		// OK: API version negotiation range.
		{
			name: "older API 1.43",
			fake: fakeDocker{
				ping: client.PingResult{APIVersion: "1.43", OSType: "linux"},
				info: system.Info{ServerVersion: "29.0.0", Driver: "overlay2", ContainersRunning: 1},
			},
			wantState: StateOK, wantVersion: "29.0.0",
			wantAPI: "1.43", wantOS: "linux", wantDriver: "overlay2", wantRunning: 1,
		},
		{
			name: "future API 1.46",
			fake: fakeDocker{
				ping: client.PingResult{APIVersion: "1.46", OSType: "linux"},
				info: system.Info{ServerVersion: "29.0.0", Driver: "overlay2", ContainersRunning: 1},
			},
			wantState: StateOK, wantVersion: "29.0.0",
			wantAPI: "1.46", wantOS: "linux", wantDriver: "overlay2", wantRunning: 1,
		},
		// OK: version string trimming.
		{
			name: "version with leading space",
			fake: fakeDocker{
				ping: client.PingResult{APIVersion: "1.45", OSType: "linux"},
				info: system.Info{ServerVersion: " 28.1.0", Driver: "overlay2", ContainersRunning: 1},
			},
			wantState: StateOK, wantVersion: "28.1.0",
			wantAPI: "1.45", wantOS: "linux", wantDriver: "overlay2", wantRunning: 1,
		},
		{
			name: "version and driver with trailing space",
			fake: fakeDocker{
				ping: client.PingResult{APIVersion: "1.45", OSType: "linux"},
				info: system.Info{ServerVersion: "29.0.0 ", Driver: "overlay2 ", ContainersRunning: 1},
			},
			wantState: StateOK, wantVersion: "29.0.0",
			wantAPI: "1.45", wantOS: "linux", wantDriver: "overlay2", wantRunning: 1,
		},
		// OK: build metadata in version preserved.
		{
			name: "version with build metadata",
			fake: fakeDocker{
				ping: client.PingResult{APIVersion: "1.44", OSType: "linux"},
				info: system.Info{ServerVersion: "28.0.0-ce", Driver: "overlay2", ContainersRunning: 0},
			},
			wantState: StateOK, wantVersion: "28.0.0-ce",
			wantAPI: "1.44", wantOS: "linux", wantDriver: "overlay2", wantRunning: 0,
		},
		// Degraded: ping OK, info fails.
		{
			name: "v28 degraded info error",
			fake: fakeDocker{
				ping:    client.PingResult{APIVersion: "1.44", OSType: "linux"},
				infoErr: errors.New("daemon busy"),
			},
			wantState: StateDegraded, wantMsgSub: "daemon busy",
			wantAPI: "1.44", wantOS: "linux",
		},
		{
			name: "v29 degraded info error",
			fake: fakeDocker{
				ping:    client.PingResult{APIVersion: "1.45", OSType: "linux"},
				infoErr: errors.New("info unavailable"),
			},
			wantState: StateDegraded, wantMsgSub: "info unavailable",
			wantAPI: "1.45", wantOS: "linux",
		},
		// Error: ping fails.
		{
			name: "v28 ping error",
			fake: fakeDocker{pingErr: errors.New("cannot connect")},
			wantState: StateError, wantMsgSub: "cannot connect",
		},
		{
			name: "v29 ping error",
			fake: fakeDocker{pingErr: errors.New("connection refused")},
			wantState: StateError, wantMsgSub: "connection refused",
		},
		// Error: context canceled before check.
		{
			name:      "context canceled",
			fake:      fakeDocker{ctxAware: true},
			cancelCtx: true,
			wantState: StateError, wantMsgSub: "context canceled",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c := newTestChecker(t, tc.fake)

			ctx := context.Background()
			if tc.cancelCtx {
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}

			st := c.Check(ctx)

			if st.State != tc.wantState {
				t.Fatalf("State: got %q, want %q", st.State, tc.wantState)
			}
			if !st.CheckedAt.Equal(fixedClock) {
				t.Errorf("CheckedAt: got %v, want %v", st.CheckedAt, fixedClock)
			}
			if tc.wantMsgSub != "" && !strings.Contains(st.Message, tc.wantMsgSub) {
				t.Errorf("Message %q missing substring %q", st.Message, tc.wantMsgSub)
			}

			switch tc.wantState {
			case StateOK:
				if st.Version != tc.wantVersion {
					t.Errorf("Version: got %q, want %q", st.Version, tc.wantVersion)
				}
				assertDetails(t, st.Details, tc.wantAPI, tc.wantOS, tc.wantDriver, tc.wantRunning)
				if len(st.Details) != 4 {
					t.Errorf("Details key count: got %d, want 4; keys: %v", len(st.Details), st.Details)
				}
			case StateDegraded:
				if v, ok := st.Details["api_version"]; !ok || v.(string) != tc.wantAPI {
					t.Errorf("degraded api_version: got %v, want %q", st.Details["api_version"], tc.wantAPI)
				}
				if v, ok := st.Details["os_type"]; !ok || v.(string) != tc.wantOS {
					t.Errorf("degraded os_type: got %v, want %q", st.Details["os_type"], tc.wantOS)
				}
			}
		})
	}
}

func TestDockerChecker_NilGuards(t *testing.T) {
	tests := []struct {
		name    string
		checker *DockerChecker
		wantMsg string
	}{
		{
			name:    "nil receiver",
			checker: nil,
			wantMsg: "docker client unavailable",
		},
		{
			name:    "nil client",
			checker: &DockerChecker{client: nil, timeout: 50 * time.Millisecond},
			wantMsg: "docker client unavailable",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			st := tc.checker.Check(context.Background())
			if st.State != StateUnknown {
				t.Fatalf("State: got %q, want %q", st.State, StateUnknown)
			}
			if st.Message != tc.wantMsg {
				t.Errorf("Message: got %q, want %q", st.Message, tc.wantMsg)
			}
		})
	}
}

func TestDockerChecker_Close(t *testing.T) {
	c := newTestChecker(t, fakeDocker{})
	if err := c.Close(); err != nil {
		t.Fatalf("unexpected close error: %v", err)
	}
}

func TestDockerChecker_ConstructorDefaults(t *testing.T) {
	base := fakeDocker{
		ping: client.PingResult{APIVersion: "1.44"},
		info: system.Info{ServerVersion: "29.0.0"},
	}
	tests := []struct {
		name  string
		check func(t *testing.T, c *DockerChecker)
	}{
		{
			name: "default timeout is 3s",
			check: func(t *testing.T, c *DockerChecker) {
				if c.timeout != 3*time.Second {
					t.Fatalf("timeout: got %v, want 3s", c.timeout)
				}
			},
		},
		{
			name: "default clock is non-zero",
			check: func(t *testing.T, c *DockerChecker) {
				if c.now().IsZero() {
					t.Fatal("expected non-zero clock time")
				}
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c, err := NewDockerChecker(DockerCheckerOptions{Client: base})
			if err != nil {
				t.Fatal(err)
			}
			tc.check(t, c)
		})
	}
}
