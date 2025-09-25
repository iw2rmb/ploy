package runner

import (
	"context"
	"errors"
)

var (
	ErrTicketRequired = errors.New("ticket is required")
	ErrNotImplemented = errors.New("workflow runner not implemented")
)

type Options struct {
	Ticket string
}

func Run(ctx context.Context, opts Options) error {
	if opts.Ticket == "" {
		return ErrTicketRequired
	}
	_ = ctx
	return ErrNotImplemented
}
