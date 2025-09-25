package runner

import (
	"context"
	"errors"
	"fmt"

	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

var (
	ErrTicketRequired             = errors.New("ticket is required")
	ErrEventsClientRequired       = errors.New("events client is required")
	ErrTicketValidationFailed     = errors.New("ticket payload failed validation")
	ErrCheckpointValidationFailed = errors.New("checkpoint payload failed validation")
)

type EventsClient interface {
	ClaimTicket(ctx context.Context, ticketID string) (contracts.WorkflowTicket, error)
	PublishCheckpoint(ctx context.Context, checkpoint contracts.WorkflowCheckpoint) error
}

type Options struct {
	Ticket string
	Events EventsClient
}

func Run(ctx context.Context, opts Options) error {
	if opts.Ticket == "" {
		return ErrTicketRequired
	}
	if opts.Events == nil {
		return ErrEventsClientRequired
	}

	ticket, err := opts.Events.ClaimTicket(ctx, opts.Ticket)
	if err != nil {
		return err
	}
	if err := ticket.Validate(); err != nil {
		return fmt.Errorf("%w: %v", ErrTicketValidationFailed, err)
	}

	checkpoint := contracts.WorkflowCheckpoint{
		SchemaVersion: contracts.SchemaVersion,
		TicketID:      opts.Ticket,
		Stage:         "ticket-claimed",
		Status:        contracts.CheckpointStatusClaimed,
	}
	if err := checkpoint.Validate(); err != nil {
		return fmt.Errorf("%w: %v", ErrCheckpointValidationFailed, err)
	}

	if err := opts.Events.PublishCheckpoint(ctx, checkpoint); err != nil {
		return err
	}

	return nil
}
