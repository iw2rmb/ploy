package runner_test

import (
	"context"
	"errors"
	"testing"

	"github.com/iw2rmb/ploy/internal/workflow/runner"
)

func TestRunRequiresTicket(t *testing.T) {
	err := runner.Run(context.Background(), runner.Options{})
	if !errors.Is(err, runner.ErrTicketRequired) {
		t.Fatalf("expected ErrTicketRequired, got %v", err)
	}
}

func TestRunAcceptsExplicitTicket(t *testing.T) {
	opts := runner.Options{Ticket: "TICKET-123"}
	err := runner.Run(context.Background(), opts)
	if !errors.Is(err, runner.ErrNotImplemented) {
		t.Fatalf("expected ErrNotImplemented for stub run, got %v", err)
	}
}
