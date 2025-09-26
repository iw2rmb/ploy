package contracts

import (
	"context"
	"fmt"
	"strings"
)

type InMemoryBus struct {
	Tenant         string
	ClaimedTickets []string
	Checkpoints    []WorkflowCheckpoint
	tickets        []string
}

func NewInMemoryBus(tenant string) *InMemoryBus {
	return &InMemoryBus{Tenant: tenant}
}

func (b *InMemoryBus) EnqueueTicket(ticketID string) {
	b.tickets = append(b.tickets, ticketID)
}

func (b *InMemoryBus) ClaimTicket(ctx context.Context, ticketID string) (WorkflowTicket, error) {
	_ = ctx
	trimmed := strings.TrimSpace(ticketID)
	if trimmed == "" {
		if len(b.tickets) > 0 {
			trimmed = b.tickets[0]
			b.tickets = b.tickets[1:]
		} else {
			trimmed = fmt.Sprintf("ticket-auto-%d", len(b.ClaimedTickets)+1)
		}
	}
	b.ClaimedTickets = append(b.ClaimedTickets, trimmed)
	return WorkflowTicket{
		SchemaVersion: SchemaVersion,
		TicketID:      trimmed,
		Tenant:        b.Tenant,
	}, nil
}

func (b *InMemoryBus) PublishCheckpoint(ctx context.Context, checkpoint WorkflowCheckpoint) error {
	_ = ctx
	b.Checkpoints = append(b.Checkpoints, checkpoint)
	return nil
}
