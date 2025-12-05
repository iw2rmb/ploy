package lifecycle

import (
	"context"
	"errors"
	"testing"
	"time"

	// Docker Engine v29 SDK modules (moby). These replace the deprecated
	// github.com/docker/docker imports with supported Engine v29 equivalents.
	// See ROADMAP.md "Migrate worker lifecycle packages to moby client and types".
	"github.com/moby/moby/api/types/system"
	"github.com/moby/moby/client"
)

// fakeDocker implements the dockerAPI interface for testing DockerChecker.
// It uses moby Engine v29 SDK types (client.PingResult, client.SystemInfoResult)
// to match the production dockerAPI interface.
type fakeDocker struct {
	ping    client.PingResult
	pingErr error
	info    system.Info
	infoErr error
}

// Ping returns the configured PingResult or error. Matches moby client.Ping signature.
func (f fakeDocker) Ping(ctx context.Context, opts client.PingOptions) (client.PingResult, error) {
	return f.ping, f.pingErr
}

// Info returns SystemInfoResult wrapping system.Info or error. Matches moby client.Info signature.
func (f fakeDocker) Info(ctx context.Context, opts client.InfoOptions) (client.SystemInfoResult, error) {
	return client.SystemInfoResult{Info: f.info}, f.infoErr
}

// Close is a no-op for the fake client.
func (f fakeDocker) Close() error { return nil }

func TestDockerChecker_PingError(t *testing.T) {
	c, err := NewDockerChecker(DockerCheckerOptions{
		Client:  fakeDocker{pingErr: errors.New("boom")},
		Timeout: 50 * time.Millisecond,
		Clock:   func() time.Time { return time.Unix(10, 0).UTC() },
	})
	if err != nil {
		t.Fatal(err)
	}
	st := c.Check(context.Background())
	if st.State != stateError {
		t.Fatalf("want error, got %s", st.State)
	}
}

func TestDockerChecker_InfoError(t *testing.T) {
	c, err := NewDockerChecker(DockerCheckerOptions{
		Client:  fakeDocker{infoErr: errors.New("no info")},
		Timeout: 50 * time.Millisecond,
		Clock:   func() time.Time { return time.Unix(11, 0).UTC() },
	})
	if err != nil {
		t.Fatal(err)
	}
	st := c.Check(context.Background())
	if st.State != stateDegraded {
		t.Fatalf("want degraded, got %s", st.State)
	}
}

func TestDockerChecker_OK(t *testing.T) {
	// Test uses moby Engine v29 SDK types (client.PingResult, system.Info)
	// to verify successful health check returns OK state with correct details.
	c, err := NewDockerChecker(DockerCheckerOptions{
		Client: fakeDocker{
			ping: client.PingResult{APIVersion: "1.44"},
			info: system.Info{ServerVersion: "25.0.0", Driver: "overlay2", ContainersRunning: 3},
		},
		Timeout: 50 * time.Millisecond,
		Clock:   func() time.Time { return time.Unix(12, 0).UTC() },
	})
	if err != nil {
		t.Fatal(err)
	}
	st := c.Check(context.Background())
	if st.State != stateOK {
		t.Fatalf("want ok, got %s", st.State)
	}
	if st.Version != "25.0.0" {
		t.Fatalf("unexpected version: %s", st.Version)
	}
	if v, ok := st.Details["api_version"]; !ok || v.(string) != "1.44" {
		t.Fatalf("unexpected api_version: %#v", st.Details)
	}
}

// TestDockerChecker_Close verifies Close behaviour with moby client.
// The real moby client.Client implements Close(); the fake does too.
func TestDockerChecker_Close(t *testing.T) {
	c, err := NewDockerChecker(DockerCheckerOptions{
		Client:  fakeDocker{},
		Timeout: 50 * time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}
	// Close should succeed on fake client.
	if err := c.Close(); err != nil {
		t.Fatalf("unexpected close error: %v", err)
	}
}

// TestDockerChecker_NilClient verifies Check handles nil client gracefully.
func TestDockerChecker_NilClient(t *testing.T) {
	// Manually construct a checker with nil client to test nil guard.
	c := &DockerChecker{client: nil, timeout: 50 * time.Millisecond}
	st := c.Check(context.Background())
	if st.State != stateUnknown {
		t.Fatalf("want unknown for nil client, got %s", st.State)
	}
	if st.Message != "docker client unavailable" {
		t.Fatalf("unexpected message: %s", st.Message)
	}
}

// TestDockerChecker_NilChecker verifies Check handles nil receiver gracefully.
func TestDockerChecker_NilChecker(t *testing.T) {
	var c *DockerChecker
	st := c.Check(context.Background())
	if st.State != stateUnknown {
		t.Fatalf("want unknown for nil checker, got %s", st.State)
	}
}

// TestDockerChecker_ContextCanceled verifies Check respects context cancellation.
// The moby client propagates context to Ping/Info calls; a canceled context
// should result in an error state.
func TestDockerChecker_ContextCanceled(t *testing.T) {
	// Create a fake that returns context error when context is cancelled.
	fake := &fakeDockerWithContext{}
	c, err := NewDockerChecker(DockerCheckerOptions{
		Client:  fake,
		Timeout: 5 * time.Second, // Long timeout; we cancel immediately.
		Clock:   func() time.Time { return time.Unix(20, 0).UTC() },
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately before check.
	st := c.Check(ctx)
	// Should get error state from canceled context.
	if st.State != stateError {
		t.Fatalf("want error for canceled context, got %s", st.State)
	}
}

// fakeDockerWithContext is a fake that respects context cancellation.
type fakeDockerWithContext struct{}

// Ping returns context error if context is done.
func (f *fakeDockerWithContext) Ping(ctx context.Context, opts client.PingOptions) (client.PingResult, error) {
	select {
	case <-ctx.Done():
		return client.PingResult{}, ctx.Err()
	default:
		return client.PingResult{APIVersion: "1.44"}, nil
	}
}

// Info returns context error if context is done.
func (f *fakeDockerWithContext) Info(ctx context.Context, opts client.InfoOptions) (client.SystemInfoResult, error) {
	select {
	case <-ctx.Done():
		return client.SystemInfoResult{}, ctx.Err()
	default:
		return client.SystemInfoResult{Info: system.Info{ServerVersion: "29.0.0"}}, nil
	}
}

// Close is a no-op for the fake client.
func (f *fakeDockerWithContext) Close() error { return nil }

// TestDockerChecker_DefaultTimeout verifies default timeout is applied.
func TestDockerChecker_DefaultTimeout(t *testing.T) {
	c, err := NewDockerChecker(DockerCheckerOptions{
		Client: fakeDocker{
			ping: client.PingResult{APIVersion: "1.44"},
			info: system.Info{ServerVersion: "29.0.0"},
		},
		// Timeout not set; should default to 3s.
	})
	if err != nil {
		t.Fatal(err)
	}
	if c.timeout != 3*time.Second {
		t.Fatalf("expected default timeout 3s, got %v", c.timeout)
	}
}

// TestDockerChecker_DefaultClock verifies default clock is applied.
func TestDockerChecker_DefaultClock(t *testing.T) {
	c, err := NewDockerChecker(DockerCheckerOptions{
		Client: fakeDocker{
			ping: client.PingResult{APIVersion: "1.44"},
			info: system.Info{ServerVersion: "29.0.0"},
		},
		Timeout: 50 * time.Millisecond,
		// Clock not set; should default to time.Now().UTC().
	})
	if err != nil {
		t.Fatal(err)
	}
	// Just verify clock is set and produces a reasonable time.
	now := c.now()
	if now.IsZero() {
		t.Fatal("expected non-zero clock time")
	}
}

// TestDockerChecker_DetailsFields verifies all expected Details fields are populated.
// Moby Engine v29 SDK returns system.Info with ContainersRunning and Driver.
func TestDockerChecker_DetailsFields(t *testing.T) {
	c, err := NewDockerChecker(DockerCheckerOptions{
		Client: fakeDocker{
			ping: client.PingResult{APIVersion: "1.45"},
			info: system.Info{
				ServerVersion:     "29.0.1",
				Driver:            "overlay2",
				ContainersRunning: 5,
			},
		},
		Timeout: 50 * time.Millisecond,
		Clock:   func() time.Time { return time.Unix(30, 0).UTC() },
	})
	if err != nil {
		t.Fatal(err)
	}
	st := c.Check(context.Background())
	if st.State != stateOK {
		t.Fatalf("want ok, got %s", st.State)
	}
	// Verify api_version from PingResult.
	if v, ok := st.Details["api_version"]; !ok || v.(string) != "1.45" {
		t.Fatalf("unexpected api_version: %#v", st.Details)
	}
	// Verify containers_running from system.Info.
	if v, ok := st.Details["containers_running"]; !ok || v.(int) != 5 {
		t.Fatalf("unexpected containers_running: %#v", st.Details)
	}
	// Verify driver from system.Info.
	if v, ok := st.Details["driver"]; !ok || v.(string) != "overlay2" {
		t.Fatalf("unexpected driver: %#v", st.Details)
	}
}
