package lifecycle

import (
	"context"
	"errors"
	"testing"
	"time"

	// Docker Engine v29 SDK modules (moby). These replace the deprecated
	// github.com/docker/docker imports with supported Engine v29 equivalents.
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
	if st.State != StateError {
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
	if st.State != StateDegraded {
		t.Fatalf("want degraded, got %s", st.State)
	}
}

func TestDockerChecker_OK(t *testing.T) {
	// Test uses moby Engine v29 SDK types (client.PingResult, system.Info)
	// to verify successful health check returns OK state with correct details.
	c, err := NewDockerChecker(DockerCheckerOptions{
		Client: fakeDocker{
			ping: client.PingResult{APIVersion: "1.44", OSType: "linux"},
			info: system.Info{ServerVersion: "25.0.0", Driver: "overlay2", ContainersRunning: 3},
		},
		Timeout: 50 * time.Millisecond,
		Clock:   func() time.Time { return time.Unix(12, 0).UTC() },
	})
	if err != nil {
		t.Fatal(err)
	}
	st := c.Check(context.Background())
	if st.State != StateOK {
		t.Fatalf("want ok, got %s", st.State)
	}
	if st.Version != "25.0.0" {
		t.Fatalf("unexpected version: %s", st.Version)
	}
	if v, ok := st.Details["api_version"]; !ok || v.(string) != "1.44" {
		t.Fatalf("unexpected api_version: %#v", st.Details)
	}
	// Verify os_type from PingResult is included in Details.
	if v, ok := st.Details["os_type"]; !ok || v.(string) != "linux" {
		t.Fatalf("unexpected os_type: %#v", st.Details)
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
	if st.State != StateUnknown {
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
	if st.State != StateUnknown {
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
	if st.State != StateError {
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
			ping: client.PingResult{APIVersion: "1.45", OSType: "linux"},
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
	if st.State != StateOK {
		t.Fatalf("want ok, got %s", st.State)
	}
	// Verify api_version from PingResult.
	if v, ok := st.Details["api_version"]; !ok || v.(string) != "1.45" {
		t.Fatalf("unexpected api_version: %#v", st.Details)
	}
	// Verify os_type from PingResult.
	if v, ok := st.Details["os_type"]; !ok || v.(string) != "linux" {
		t.Fatalf("unexpected os_type: %#v", st.Details)
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

// TestDockerChecker_EngineVersionCompatibility verifies stable Details keys across
// Engine v28 and v29 responses. The same field names and semantics should work
// for both daemon versions.
func TestDockerChecker_EngineVersionCompatibility(t *testing.T) {
	// Test cases simulate representative Engine v28 and v29 responses.
	// The Details keys (api_version, os_type, containers_running, driver)
	// must remain stable across versions.
	tests := []struct {
		name           string
		ping           client.PingResult
		info           system.Info
		wantVersion    string
		wantAPIVersion string
		wantOSType     string
		wantDriver     string
		wantRunning    int
	}{
		{
			name:           "Engine v28.x response",
			ping:           client.PingResult{APIVersion: "1.44", OSType: "linux"},
			info:           system.Info{ServerVersion: "28.0.0", Driver: "overlay2", ContainersRunning: 2},
			wantVersion:    "28.0.0",
			wantAPIVersion: "1.44",
			wantOSType:     "linux",
			wantDriver:     "overlay2",
			wantRunning:    2,
		},
		{
			name:           "Engine v29.x response",
			ping:           client.PingResult{APIVersion: "1.45", OSType: "linux"},
			info:           system.Info{ServerVersion: "29.0.0", Driver: "overlay2", ContainersRunning: 5},
			wantVersion:    "29.0.0",
			wantAPIVersion: "1.45",
			wantOSType:     "linux",
			wantDriver:     "overlay2",
			wantRunning:    5,
		},
		{
			name:           "Engine v29.x Windows",
			ping:           client.PingResult{APIVersion: "1.45", OSType: "windows"},
			info:           system.Info{ServerVersion: "29.0.1", Driver: "windowsfilter", ContainersRunning: 1},
			wantVersion:    "29.0.1",
			wantAPIVersion: "1.45",
			wantOSType:     "windows",
			wantDriver:     "windowsfilter",
			wantRunning:    1,
		},
		{
			name:           "Engine v28.x with vfs driver",
			ping:           client.PingResult{APIVersion: "1.44", OSType: "linux"},
			info:           system.Info{ServerVersion: "28.1.0", Driver: "vfs", ContainersRunning: 0},
			wantVersion:    "28.1.0",
			wantAPIVersion: "1.44",
			wantOSType:     "linux",
			wantDriver:     "vfs",
			wantRunning:    0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c, err := NewDockerChecker(DockerCheckerOptions{
				Client:  fakeDocker{ping: tc.ping, info: tc.info},
				Timeout: 50 * time.Millisecond,
				Clock:   func() time.Time { return time.Unix(100, 0).UTC() },
			})
			if err != nil {
				t.Fatal(err)
			}
			st := c.Check(context.Background())
			if st.State != StateOK {
				t.Fatalf("want ok, got %s", st.State)
			}
			// Verify Version from system.Info.ServerVersion.
			if st.Version != tc.wantVersion {
				t.Errorf("Version: got %q, want %q", st.Version, tc.wantVersion)
			}
			// Verify api_version from PingResult.APIVersion.
			if v, ok := st.Details["api_version"]; !ok || v.(string) != tc.wantAPIVersion {
				t.Errorf("api_version: got %v, want %q", st.Details["api_version"], tc.wantAPIVersion)
			}
			// Verify os_type from PingResult.OSType.
			if v, ok := st.Details["os_type"]; !ok || v.(string) != tc.wantOSType {
				t.Errorf("os_type: got %v, want %q", st.Details["os_type"], tc.wantOSType)
			}
			// Verify driver from system.Info.Driver.
			if v, ok := st.Details["driver"]; !ok || v.(string) != tc.wantDriver {
				t.Errorf("driver: got %v, want %q", st.Details["driver"], tc.wantDriver)
			}
			// Verify containers_running from system.Info.ContainersRunning.
			if v, ok := st.Details["containers_running"]; !ok || v.(int) != tc.wantRunning {
				t.Errorf("containers_running: got %v, want %d", st.Details["containers_running"], tc.wantRunning)
			}
		})
	}
}

// TestDockerChecker_StableDetailsKeys verifies that the Details map contains
// exactly the expected stable keys. This ensures no keys are accidentally
// removed or renamed during future refactoring.
func TestDockerChecker_StableDetailsKeys(t *testing.T) {
	c, err := NewDockerChecker(DockerCheckerOptions{
		Client: fakeDocker{
			ping: client.PingResult{APIVersion: "1.45", OSType: "linux"},
			info: system.Info{ServerVersion: "29.0.0", Driver: "overlay2", ContainersRunning: 3},
		},
		Timeout: 50 * time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}
	st := c.Check(context.Background())

	// Define the stable keys that must be present in Details.
	// These keys are documented in DockerChecker and Check comments.
	stableKeys := []string{"api_version", "os_type", "containers_running", "driver"}

	for _, key := range stableKeys {
		if _, ok := st.Details[key]; !ok {
			t.Errorf("missing stable Details key: %q", key)
		}
	}

	// Verify no unexpected keys are present.
	if len(st.Details) != len(stableKeys) {
		t.Errorf("Details has %d keys, want %d; keys: %v", len(st.Details), len(stableKeys), st.Details)
	}
}

// TestDockerChecker_MixedDaemonVersionHealth validates that DockerChecker produces
// sane ComponentStatus across mixed Engine v28 and v29 daemon versions. This test
// exercises the full matrix of version-specific responses to ensure:
//   - State (OK, Degraded, Error) is meaningful for both daemon versions.
//   - Version field correctly reflects the ServerVersion from Info.
//   - Details keys remain stable and correctly typed across versions.
func TestDockerChecker_MixedDaemonVersionHealth(t *testing.T) {
	// Define test cases covering representative Engine v28 and v29 responses.
	// Each case validates State, Version, and Details correctness.
	tests := []struct {
		name string
		// fake input
		ping    client.PingResult
		pingErr error
		info    system.Info
		infoErr error
		// expected output
		wantState      ComponentState
		wantVersion    string
		wantAPIVersion string
		wantOSType     string
		wantDriver     string
		wantRunning    int
		wantHasMessage bool
	}{
		// Engine v28.x healthy scenarios.
		{
			name:           "v28.0.0 healthy linux overlay2",
			ping:           client.PingResult{APIVersion: "1.44", OSType: "linux"},
			info:           system.Info{ServerVersion: "28.0.0", Driver: "overlay2", ContainersRunning: 0},
			wantState:      StateOK,
			wantVersion:    "28.0.0",
			wantAPIVersion: "1.44",
			wantOSType:     "linux",
			wantDriver:     "overlay2",
			wantRunning:    0,
		},
		{
			name:           "v28.1.3 healthy linux with running containers",
			ping:           client.PingResult{APIVersion: "1.44", OSType: "linux"},
			info:           system.Info{ServerVersion: "28.1.3", Driver: "overlay2", ContainersRunning: 42},
			wantState:      StateOK,
			wantVersion:    "28.1.3",
			wantAPIVersion: "1.44",
			wantOSType:     "linux",
			wantDriver:     "overlay2",
			wantRunning:    42,
		},
		{
			name:           "v28.0.5 healthy linux vfs driver",
			ping:           client.PingResult{APIVersion: "1.44", OSType: "linux"},
			info:           system.Info{ServerVersion: "28.0.5", Driver: "vfs", ContainersRunning: 1},
			wantState:      StateOK,
			wantVersion:    "28.0.5",
			wantAPIVersion: "1.44",
			wantOSType:     "linux",
			wantDriver:     "vfs",
			wantRunning:    1,
		},
		// Engine v29.x healthy scenarios.
		{
			name:           "v29.0.0 healthy linux overlay2",
			ping:           client.PingResult{APIVersion: "1.45", OSType: "linux"},
			info:           system.Info{ServerVersion: "29.0.0", Driver: "overlay2", ContainersRunning: 0},
			wantState:      StateOK,
			wantVersion:    "29.0.0",
			wantAPIVersion: "1.45",
			wantOSType:     "linux",
			wantDriver:     "overlay2",
			wantRunning:    0,
		},
		{
			name:           "v29.0.3 healthy linux with many containers",
			ping:           client.PingResult{APIVersion: "1.45", OSType: "linux"},
			info:           system.Info{ServerVersion: "29.0.3", Driver: "overlay2", ContainersRunning: 100},
			wantState:      StateOK,
			wantVersion:    "29.0.3",
			wantAPIVersion: "1.45",
			wantOSType:     "linux",
			wantDriver:     "overlay2",
			wantRunning:    100,
		},
		{
			name:           "v29.1.0 healthy windows windowsfilter",
			ping:           client.PingResult{APIVersion: "1.45", OSType: "windows"},
			info:           system.Info{ServerVersion: "29.1.0", Driver: "windowsfilter", ContainersRunning: 5},
			wantState:      StateOK,
			wantVersion:    "29.1.0",
			wantAPIVersion: "1.45",
			wantOSType:     "windows",
			wantDriver:     "windowsfilter",
			wantRunning:    5,
		},
		// Degraded scenarios: Ping OK but Info fails.
		// Both v28 and v29 should produce Degraded state when Info fails.
		{
			name:           "v28 degraded info error",
			ping:           client.PingResult{APIVersion: "1.44", OSType: "linux"},
			infoErr:        errors.New("daemon busy"),
			wantState:      StateDegraded,
			wantAPIVersion: "1.44",
			wantOSType:     "linux",
			wantHasMessage: true,
		},
		{
			name:           "v29 degraded info error",
			ping:           client.PingResult{APIVersion: "1.45", OSType: "linux"},
			infoErr:        errors.New("info unavailable"),
			wantState:      StateDegraded,
			wantAPIVersion: "1.45",
			wantOSType:     "linux",
			wantHasMessage: true,
		},
		// Error scenarios: Ping fails.
		// Both v28 and v29 should produce Error state when Ping fails.
		{
			name:           "v28 error ping fails",
			pingErr:        errors.New("cannot connect to docker daemon"),
			wantState:      StateError,
			wantHasMessage: true,
		},
		{
			name:           "v29 error ping fails",
			pingErr:        errors.New("connection refused"),
			wantState:      StateError,
			wantHasMessage: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c, err := NewDockerChecker(DockerCheckerOptions{
				Client: fakeDocker{
					ping:    tc.ping,
					pingErr: tc.pingErr,
					info:    tc.info,
					infoErr: tc.infoErr,
				},
				Timeout: 50 * time.Millisecond,
				Clock:   func() time.Time { return time.Unix(200, 0).UTC() },
			})
			if err != nil {
				t.Fatal(err)
			}

			st := c.Check(context.Background())

			// Verify State is as expected.
			if st.State != tc.wantState {
				t.Errorf("State: got %q, want %q", st.State, tc.wantState)
			}

			// Verify Message is present when expected.
			if tc.wantHasMessage && st.Message == "" {
				t.Error("expected Message to be set, got empty")
			}

			// For non-error states, verify Version and Details.
			if tc.wantState == StateOK {
				if st.Version != tc.wantVersion {
					t.Errorf("Version: got %q, want %q", st.Version, tc.wantVersion)
				}
				if v, ok := st.Details["api_version"]; !ok || v.(string) != tc.wantAPIVersion {
					t.Errorf("api_version: got %v, want %q", st.Details["api_version"], tc.wantAPIVersion)
				}
				if v, ok := st.Details["os_type"]; !ok || v.(string) != tc.wantOSType {
					t.Errorf("os_type: got %v, want %q", st.Details["os_type"], tc.wantOSType)
				}
				if v, ok := st.Details["driver"]; !ok || v.(string) != tc.wantDriver {
					t.Errorf("driver: got %v, want %q", st.Details["driver"], tc.wantDriver)
				}
				if v, ok := st.Details["containers_running"]; !ok || v.(int) != tc.wantRunning {
					t.Errorf("containers_running: got %v, want %d", st.Details["containers_running"], tc.wantRunning)
				}
			}

			// For degraded states, verify partial Details (api_version, os_type from Ping).
			if tc.wantState == StateDegraded {
				if v, ok := st.Details["api_version"]; !ok || v.(string) != tc.wantAPIVersion {
					t.Errorf("degraded api_version: got %v, want %q", st.Details["api_version"], tc.wantAPIVersion)
				}
				if v, ok := st.Details["os_type"]; !ok || v.(string) != tc.wantOSType {
					t.Errorf("degraded os_type: got %v, want %q", st.Details["os_type"], tc.wantOSType)
				}
			}
		})
	}
}

// TestDockerChecker_VersionStringEdgeCases verifies that version strings with
// whitespace or unusual formatting are handled correctly. ServerVersion from
// Docker Engine may have trailing whitespace in some edge cases.
func TestDockerChecker_VersionStringEdgeCases(t *testing.T) {
	tests := []struct {
		name          string
		serverVersion string
		driver        string
		wantVersion   string
		wantDriver    string
	}{
		{
			name:          "clean version",
			serverVersion: "28.0.0",
			driver:        "overlay2",
			wantVersion:   "28.0.0",
			wantDriver:    "overlay2",
		},
		{
			name:          "version with trailing space",
			serverVersion: "29.0.0 ",
			driver:        "overlay2",
			wantVersion:   "29.0.0",
			wantDriver:    "overlay2",
		},
		{
			name:          "version with leading space",
			serverVersion: " 28.1.0",
			driver:        "overlay2",
			wantVersion:   "28.1.0",
			wantDriver:    "overlay2",
		},
		{
			name:          "driver with trailing space",
			serverVersion: "29.0.0",
			driver:        "overlay2 ",
			wantVersion:   "29.0.0",
			wantDriver:    "overlay2",
		},
		{
			name:          "version with build metadata",
			serverVersion: "28.0.0-ce",
			driver:        "overlay2",
			wantVersion:   "28.0.0-ce",
			wantDriver:    "overlay2",
		},
		{
			name:          "version with git hash suffix",
			serverVersion: "29.0.0-dev+abc123",
			driver:        "overlay2",
			wantVersion:   "29.0.0-dev+abc123",
			wantDriver:    "overlay2",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c, err := NewDockerChecker(DockerCheckerOptions{
				Client: fakeDocker{
					ping: client.PingResult{APIVersion: "1.45", OSType: "linux"},
					info: system.Info{
						ServerVersion:     tc.serverVersion,
						Driver:            tc.driver,
						ContainersRunning: 1,
					},
				},
				Timeout: 50 * time.Millisecond,
			})
			if err != nil {
				t.Fatal(err)
			}

			st := c.Check(context.Background())

			if st.State != StateOK {
				t.Fatalf("State: got %q, want %q", st.State, StateOK)
			}
			// Verify trimmed version.
			if st.Version != tc.wantVersion {
				t.Errorf("Version: got %q, want %q", st.Version, tc.wantVersion)
			}
			// Verify trimmed driver.
			if v, ok := st.Details["driver"]; !ok || v.(string) != tc.wantDriver {
				t.Errorf("driver: got %q, want %q", st.Details["driver"], tc.wantDriver)
			}
		})
	}
}

// TestDockerChecker_CheckedAtTimestamp verifies that CheckedAt is set correctly
// for both v28 and v29 responses using the injected clock.
func TestDockerChecker_CheckedAtTimestamp(t *testing.T) {
	fixedTime := time.Date(2024, 12, 5, 10, 30, 0, 0, time.UTC)
	tests := []struct {
		name string
		ping client.PingResult
		info system.Info
	}{
		{
			name: "v28 timestamp",
			ping: client.PingResult{APIVersion: "1.44", OSType: "linux"},
			info: system.Info{ServerVersion: "28.0.0", Driver: "overlay2"},
		},
		{
			name: "v29 timestamp",
			ping: client.PingResult{APIVersion: "1.45", OSType: "linux"},
			info: system.Info{ServerVersion: "29.0.0", Driver: "overlay2"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c, err := NewDockerChecker(DockerCheckerOptions{
				Client:  fakeDocker{ping: tc.ping, info: tc.info},
				Timeout: 50 * time.Millisecond,
				Clock:   func() time.Time { return fixedTime },
			})
			if err != nil {
				t.Fatal(err)
			}

			st := c.Check(context.Background())

			if !st.CheckedAt.Equal(fixedTime) {
				t.Errorf("CheckedAt: got %v, want %v", st.CheckedAt, fixedTime)
			}
		})
	}
}

// TestDockerChecker_APIVersionNegotiationRange verifies that DockerChecker handles
// the API version range used by Engine v28 (1.44) and v29 (1.45) correctly.
// This ensures the api_version Detail reflects the negotiated version.
func TestDockerChecker_APIVersionNegotiationRange(t *testing.T) {
	tests := []struct {
		name       string
		apiVersion string
		wantAPI    string
	}{
		{name: "v28 API 1.44", apiVersion: "1.44", wantAPI: "1.44"},
		{name: "v29 API 1.45", apiVersion: "1.45", wantAPI: "1.45"},
		{name: "older API 1.43", apiVersion: "1.43", wantAPI: "1.43"},
		{name: "future API 1.46", apiVersion: "1.46", wantAPI: "1.46"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c, err := NewDockerChecker(DockerCheckerOptions{
				Client: fakeDocker{
					ping: client.PingResult{APIVersion: tc.apiVersion, OSType: "linux"},
					info: system.Info{ServerVersion: "29.0.0", Driver: "overlay2"},
				},
				Timeout: 50 * time.Millisecond,
			})
			if err != nil {
				t.Fatal(err)
			}

			st := c.Check(context.Background())

			if st.State != StateOK {
				t.Fatalf("State: got %q, want %q", st.State, StateOK)
			}
			if v := st.Details["api_version"]; v.(string) != tc.wantAPI {
				t.Errorf("api_version: got %q, want %q", v, tc.wantAPI)
			}
		})
	}
}
