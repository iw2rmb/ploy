package git

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
)

// fakeRunner records git invocations for assertions in tests.
type fakeRunner struct {
	calls  []string
	failAt int
}

// Run appends the executed command and optionally fails at the configured call.
func (f *fakeRunner) Run(ctx context.Context, dir string, name string, args ...string) error {
	f.calls = append(f.calls, name+" "+joinArgs(args))
	if f.failAt == len(f.calls) {
		return errors.New("boom")
	}
	return nil
}

// joinArgs joins command arguments for compact comparison.
func joinArgs(args []string) string {
	if len(args) == 0 {
		return ""
	}
	s := args[0]
	for i := 1; i < len(args); i++ {
		s += " " + args[i]
	}
	return s
}

// TestPushBranchPublishesEventsOnSuccess verifies that push events include start and completion states.
func TestPushBranchPublishesEventsOnSuccess(t *testing.T) {
	// Ensure token injection does not alter the expected remote URL in this test
	_ = os.Unsetenv("PLOY_GITLAB_PAT")
	runner := &fakeRunner{}
	svc := NewService(ServiceConfig{Runner: runner})

	ctx := context.Background()
	op := svc.PushBranchAsync(ctx, PushRequest{RepoPath: "/repo", RemoteURL: "https://example.com/repo.git", Branch: "feature"})

	var events []Event
	for ev := range op.Events() {
		events = append(events, ev)
	}

	if len(events) < 2 {
		t.Fatalf("expected at least start and completion events, got %d", len(events))
	}

	if events[0].Type != EventStarted {
		t.Fatalf("expected first event to be started, got %s", events[0].Type)
	}

	last := events[len(events)-1]
	if last.Type != EventCompleted || last.Err != nil {
		t.Fatalf("expected completed event without error, got %+v", last)
	}

	if err := op.Err(); err != nil {
		t.Fatalf("unexpected error waiting for operation: %v", err)
	}

	expected := []string{
		"git remote remove origin",
		"git remote add origin https://example.com/repo.git",
		"git push -u origin feature",
	}
	if len(runner.calls) != len(expected) {
		t.Fatalf("expected %d calls, got %d", len(expected), len(runner.calls))
	}
	for i, call := range expected {
		if runner.calls[i] != call {
			t.Fatalf("call %d mismatch: want %q got %q", i, call, runner.calls[i])
		}
	}
}

// TestPushBranchPublishesFailureEvent verifies failure events propagate errors to observers.
func TestPushBranchPublishesFailureEvent(t *testing.T) {
	_ = os.Unsetenv("GITLAB_TOKEN")
	runner := &fakeRunner{failAt: 3}
	svc := NewService(ServiceConfig{Runner: runner})

	ctx := context.Background()
	op := svc.PushBranchAsync(ctx, PushRequest{RepoPath: "/repo", RemoteURL: "https://example.com/repo.git", Branch: "feature"})

	var events []Event
	for ev := range op.Events() {
		events = append(events, ev)
	}

	if len(events) == 0 {
		t.Fatalf("expected events, got 0")
	}

	gotErr := events[len(events)-1]
	if gotErr.Type != EventFailed || gotErr.Err == nil {
		t.Fatalf("expected final event to be failure with error, got %+v", gotErr)
	}

	if err := op.Err(); err == nil {
		t.Fatalf("expected operation error")
	}
}

// TestAuthenticatedRemoteURL ensures credentials are injected only when a token is present.
func TestAuthenticatedRemoteURL(t *testing.T) {
	svc := NewService(ServiceConfig{})
	original := "https://gitlab.com/namespace/project.git"

	_ = os.Unsetenv("GITLAB_TOKEN")
	if got := svc.authenticatedRemoteURL(original); got != original {
		t.Fatalf("expected unchanged URL when token missing, got %q", got)
	}

	_ = os.Setenv("PLOY_GITLAB_PAT", "test-token-123")
	defer func() { _ = os.Unsetenv("PLOY_GITLAB_PAT") }()
	got := svc.authenticatedRemoteURL(original)
	if !strings.HasPrefix(got, "https://oauth2:test-token-123@") {
		t.Fatalf("expected oauth2 credentials injected, got %q", got)
	}
}

// TestPushBranchReturnsErrorPropagated verifies the synchronous wrapper surfaces failures.
func TestPushBranchReturnsErrorPropagated(t *testing.T) {
	_ = os.Unsetenv("GITLAB_TOKEN")
	runner := &fakeRunner{failAt: 2}
	svc := NewService(ServiceConfig{Runner: runner})

	err := svc.PushBranch(context.Background(), "/repo", "https://example.com/repo.git", "feature")
	if err == nil {
		t.Fatalf("expected push error when runner fails")
	}
	if !strings.Contains(err.Error(), "set remote origin") && !strings.Contains(err.Error(), "git push failed") {
		t.Fatalf("expected error to mention git failure, got %v", err)
	}
}
