package contracts

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/nats-io/nats.go"
)

// JetStreamOptions controls how the JetStream events client connects to NATS.
type JetStreamOptions struct {
	URL         string
	Tenant      string
	Durable     string
	PullTimeout time.Duration
}

// JetStreamClient implements EventsClient backed by NATS JetStream.
type JetStreamClient struct {
	conn        *nats.Conn
	js          nats.JetStreamContext
	tenant      string
	durable     string
	pullTimeout time.Duration

	mu        sync.Mutex
	ticketSub *nats.Subscription
}

// NewJetStreamClient dials JetStream and returns an EventsClient implementation.
func NewJetStreamClient(opts JetStreamOptions) (*JetStreamClient, error) {
	url := strings.TrimSpace(opts.URL)
	if url == "" {
		return nil, fmt.Errorf("jetstream url is required")
	}
	tenant := strings.TrimSpace(opts.Tenant)
	if tenant == "" {
		return nil, fmt.Errorf("tenant is required")
	}

	durable := strings.TrimSpace(opts.Durable)
	if durable == "" {
		durable = fmt.Sprintf("ploy-workflow-%s", tenant)
	}

	pullTimeout := opts.PullTimeout
	if pullTimeout <= 0 {
		pullTimeout = 3 * time.Second
	}

	conn, err := nats.Connect(url, nats.Name("ploy-workflow-cli"))
	if err != nil {
		return nil, fmt.Errorf("connect jetstream: %w", err)
	}

	js, err := conn.JetStream()
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("jetstream context: %w", err)
	}

	return &JetStreamClient{
		conn:        conn,
		js:          js,
		tenant:      tenant,
		durable:     durable,
		pullTimeout: pullTimeout,
	}, nil
}

// Close drains the JetStream connection. It is safe to call multiple times.
func (c *JetStreamClient) Close() {
	c.mu.Lock()
	sub := c.ticketSub
	c.ticketSub = nil
	c.mu.Unlock()

	if sub != nil {
		_ = sub.Drain()
	}
	if c.conn != nil {
		if err := c.conn.Drain(); err != nil {
			c.conn.Close()
		}
	}
}

// ClaimTicket pulls the next WorkflowTicket message from JetStream.
func (c *JetStreamClient) ClaimTicket(ctx context.Context, ticketID string) (WorkflowTicket, error) {
	trimmed := strings.TrimSpace(ticketID)
	if trimmed != "" {
		return WorkflowTicket{}, fmt.Errorf("explicit ticket requests are not supported with JetStream integration")
	}

	sub, err := c.ensureTicketSubscription()
	if err != nil {
		return WorkflowTicket{}, err
	}

	msgs, err := sub.Fetch(1, nats.MaxWait(c.pullTimeout))
	if err != nil {
		if errors.Is(err, nats.ErrTimeout) {
			return WorkflowTicket{}, fmt.Errorf("no tickets available: %w", err)
		}
		return WorkflowTicket{}, fmt.Errorf("fetch ticket: %w", err)
	}

	msg := msgs[0]
	defer func() { _ = msg.Ack() }()

	var ticket WorkflowTicket
	if err := json.Unmarshal(msg.Data, &ticket); err != nil {
		return WorkflowTicket{}, fmt.Errorf("decode ticket: %w", err)
	}

	if ticket.Tenant == "" {
		ticket.Tenant = c.tenant
	}

	if err := ticket.Validate(); err != nil {
		return WorkflowTicket{}, fmt.Errorf("validate ticket: %w", err)
	}

	return ticket, nil
}

// PublishCheckpoint writes a workflow checkpoint to JetStream.
func (c *JetStreamClient) PublishCheckpoint(ctx context.Context, checkpoint WorkflowCheckpoint) error {
	_ = ctx

	if err := checkpoint.Validate(); err != nil {
		return fmt.Errorf("invalid checkpoint: %w", err)
	}

	payload, err := json.Marshal(checkpoint)
	if err != nil {
		return fmt.Errorf("encode checkpoint: %w", err)
	}

	if _, err := c.js.Publish(checkpoint.Subject(), payload); err != nil {
		return fmt.Errorf("publish checkpoint: %w", err)
	}
	return nil
}

func (c *JetStreamClient) ensureTicketSubscription() (*nats.Subscription, error) {
	c.mu.Lock()
	if c.ticketSub != nil {
		sub := c.ticketSub
		c.mu.Unlock()
		return sub, nil
	}
	c.mu.Unlock()

	inbox := SubjectsForTenant(c.tenant, "").TicketInbox

	stream, err := c.js.StreamNameBySubject(inbox)
	if err != nil {
		return nil, fmt.Errorf("resolve ticket stream: %w", err)
	}

	// Attempt to bind to an existing consumer first.
	sub, err := c.js.PullSubscribe(inbox, "", nats.Bind(stream, c.durable))
	if err != nil {
		if !errors.Is(err, nats.ErrConsumerNotFound) {
			return nil, fmt.Errorf("bind ticket consumer: %w", err)
		}
		sub, err = c.js.PullSubscribe(inbox, c.durable, nats.BindStream(stream), nats.ManualAck())
		if err != nil {
			return nil, fmt.Errorf("create ticket consumer: %w", err)
		}
	}

	c.mu.Lock()
	if c.ticketSub == nil {
		c.ticketSub = sub
	} else {
		// Another goroutine set the subscription while we were connecting.
		existing := c.ticketSub
		c.mu.Unlock()
		_ = sub.Drain()
		return existing, nil
	}
	c.mu.Unlock()

	return sub, nil
}
