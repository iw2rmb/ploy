package contracts

import "context"

type InMemoryBus struct {
	Tenant         string
	ClaimedTickets []string
	Checkpoints    []WorkflowCheckpoint
}

func NewInMemoryBus(tenant string) *InMemoryBus {
	return &InMemoryBus{Tenant: tenant}
}

func (b *InMemoryBus) ClaimTicket(ctx context.Context, ticketID string) (WorkflowTicket, error) {
	_ = ctx
	b.ClaimedTickets = append(b.ClaimedTickets, ticketID)
	return WorkflowTicket{
		SchemaVersion: SchemaVersion,
		TicketID:      ticketID,
		Tenant:        b.Tenant,
	}, nil
}

func (b *InMemoryBus) PublishCheckpoint(ctx context.Context, checkpoint WorkflowCheckpoint) error {
	_ = ctx
	b.Checkpoints = append(b.Checkpoints, checkpoint)
	return nil
}
