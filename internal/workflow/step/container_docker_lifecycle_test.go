package step

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"
)

// -----------------------------------------------------------------------------
// DockerContainerRuntime basic lifecycle tests (start, wait, remove)
// -----------------------------------------------------------------------------

// TestDockerContainerRuntimeStart verifies container start with moby client.
func TestDockerContainerRuntimeStart(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name     string
		handle   ContainerHandle
		startErr error
		wantErr  bool
	}{
		{
			name:    "success",
			handle:  ContainerHandle("container123"),
			wantErr: false,
		},
		{
			name:     "error_start_fails",
			handle:   ContainerHandle("container456"),
			startErr: errors.New("container not found"),
			wantErr:  true,
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			fake := &fakeDockerClient{startErr: tc.startErr}
			rt := newDockerContainerRuntimeWithClient(fake, DockerContainerRuntimeOptions{})

			err := rt.Start(context.Background(), tc.handle)

			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if fake.startID != string(tc.handle) {
				t.Errorf("started container %q, want %q", fake.startID, string(tc.handle))
			}
		})
	}
}

// TestDockerContainerRuntimeWait verifies container wait with moby client.
func TestDockerContainerRuntimeWait(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name       string
		handle     ContainerHandle
		statusCode int64
		waitErr    error
		startedAt  string
		finishedAt string
		inspectErr error
		wantCode   int
		wantErr    bool
	}{
		{
			name:       "success_exit_0",
			handle:     ContainerHandle("container123"),
			statusCode: 0,
			startedAt:  "2024-01-15T10:00:00.000000000Z",
			finishedAt: "2024-01-15T10:01:00.000000000Z",
			wantCode:   0,
			wantErr:    false,
		},
		{
			name:       "success_exit_1",
			handle:     ContainerHandle("container456"),
			statusCode: 1,
			startedAt:  "2024-01-15T10:00:00.000000000Z",
			finishedAt: "2024-01-15T10:00:30.000000000Z",
			wantCode:   1,
			wantErr:    false,
		},
		{
			name:       "success_inspect_fails_gracefully",
			handle:     ContainerHandle("container789"),
			statusCode: 0,
			inspectErr: errors.New("inspect failed"),
			wantCode:   0,
			wantErr:    false,
		},
		{
			name:    "error_wait_fails",
			handle:  ContainerHandle("container-err"),
			waitErr: errors.New("container died unexpectedly"),
			wantErr: true,
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			fake := &fakeDockerClient{
				waitStatusCode: tc.statusCode,
				waitErr:        tc.waitErr,
				inspectErr:     tc.inspectErr,
			}
			// Set up inspect result with timestamps.
			if tc.startedAt != "" || tc.finishedAt != "" {
				fake.inspectResult = client.ContainerInspectResult{
					Container: container.InspectResponse{
						State: &container.State{
							StartedAt:  tc.startedAt,
							FinishedAt: tc.finishedAt,
						},
					},
				}
			}
			rt := newDockerContainerRuntimeWithClient(fake, DockerContainerRuntimeOptions{})

			result, err := rt.Wait(context.Background(), tc.handle)

			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if result.ExitCode != tc.wantCode {
				t.Errorf("got exit code %d, want %d", result.ExitCode, tc.wantCode)
			}
			// Verify timestamps were parsed (if inspect succeeded).
			if tc.inspectErr == nil && tc.startedAt != "" {
				if result.StartedAt.IsZero() {
					t.Error("StartedAt should not be zero")
				}
				if result.CompletedAt.IsZero() {
					t.Error("CompletedAt should not be zero")
				}
			}
		})
	}
}

// TestDockerContainerRuntimeRemove verifies container removal with moby client.
func TestDockerContainerRuntimeRemove(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name      string
		handle    ContainerHandle
		removeErr error
		wantErr   bool
	}{
		{
			name:    "success",
			handle:  ContainerHandle("container123"),
			wantErr: false,
		},
		{
			name:      "error_remove_fails",
			handle:    ContainerHandle("container456"),
			removeErr: errors.New("container busy"),
			wantErr:   true,
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			fake := &fakeDockerClient{removeErr: tc.removeErr}
			rt := newDockerContainerRuntimeWithClient(fake, DockerContainerRuntimeOptions{})

			err := rt.Remove(context.Background(), tc.handle)

			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !fake.removeCalled {
				t.Error("remove should have been called")
			}
			if fake.removeID != string(tc.handle) {
				t.Errorf("removed container %q, want %q", fake.removeID, string(tc.handle))
			}
		})
	}
}

// TestDockerContainerRuntimeNilClient verifies misconfigured runtime methods
// fail with a clear error instead of panicking.
func TestDockerContainerRuntimeNilClient(t *testing.T) {
	t.Parallel()
	rt := &DockerContainerRuntime{client: nil}
	ctx := context.Background()

	testCases := []struct {
		name string
		call func() error
	}{
		{
			name: "create",
			call: func() error {
				_, err := rt.Create(ctx, ContainerSpec{Image: "alpine"})
				return err
			},
		},
		{
			name: "start",
			call: func() error {
				return rt.Start(ctx, ContainerHandle("x"))
			},
		},
		{
			name: "wait",
			call: func() error {
				_, err := rt.Wait(ctx, ContainerHandle("x"))
				return err
			},
		},
		{
			name: "logs",
			call: func() error {
				_, err := rt.Logs(ctx, ContainerHandle("x"))
				return err
			},
		},
		{
			name: "stream_logs",
			call: func() error {
				return rt.StreamLogs(ctx, ContainerHandle("x"), &bytes.Buffer{}, &bytes.Buffer{})
			},
		},
		{
			name: "remove",
			call: func() error {
				return rt.Remove(ctx, ContainerHandle("x"))
			},
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := tc.call()
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !strings.Contains(err.Error(), "docker runtime not configured") {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

// TestParseDockerTime verifies RFC3339Nano timestamp parsing.
func TestParseDockerTime(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name    string
		input   string
		wantOK  bool
		wantUTC bool
	}{
		{
			name:    "valid_timestamp",
			input:   "2024-01-15T10:30:00.123456789Z",
			wantOK:  true,
			wantUTC: true,
		},
		{
			name:   "empty_string",
			input:  "",
			wantOK: false,
		},
		{
			name:   "whitespace_only",
			input:  "   ",
			wantOK: false,
		},
		{
			name:   "invalid_format",
			input:  "not-a-timestamp",
			wantOK: false,
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			result := parseDockerTime(tc.input)

			if tc.wantOK {
				if result.IsZero() {
					t.Error("expected non-zero time")
				}
				if tc.wantUTC && result.Location() != time.UTC {
					t.Error("expected UTC location")
				}
			} else if !result.IsZero() {
				t.Errorf("expected zero time, got %v", result)
			}
		})
	}
}
