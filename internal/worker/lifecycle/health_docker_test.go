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
