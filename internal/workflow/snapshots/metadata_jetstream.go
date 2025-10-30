package snapshots

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/iw2rmb/ploy/internal/workflow/contracts"
	"github.com/nats-io/nats.go"
)

// JetStreamMetadataOptions configures the JetStream metadata publisher.
type JetStreamMetadataOptions struct {
	// Timeout bounds how long a publish call waits for an ACK. Defaults to 3s.
	Timeout time.Duration
	// Name customises the JetStream connection name. Optional.
	Name string
}

type jetStreamMetadataPublisher struct {
	conn    *nats.Conn
	js      nats.JetStreamContext
	timeout time.Duration
}

// NewJetStreamMetadataPublisher returns a MetadataPublisher that writes snapshot
// metadata envelopes to JetStream.
func NewJetStreamMetadataPublisher(endpoint string, opts JetStreamMetadataOptions) (MetadataPublisher, error) {
	url := strings.TrimSpace(endpoint)
	if url == "" {
		return nil, fmt.Errorf("jetstream url required")
	}

	name := strings.TrimSpace(opts.Name)
	if name == "" {
		name = "ploy-snapshot-metadata"
	}
	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = 3 * time.Second
	}

	conn, err := nats.Connect(url, nats.Name(name))
	if err != nil {
		return nil, fmt.Errorf("connect jetstream: %w", err)
	}

	js, err := conn.JetStream()
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("jetstream context: %w", err)
	}

	return &jetStreamMetadataPublisher{
		conn:    conn,
		js:      js,
		timeout: timeout,
	}, nil
}

func (p *jetStreamMetadataPublisher) Publish(ctx context.Context, meta SnapshotMetadata) error {
	trimmedTicket := strings.TrimSpace(meta.TicketID)
	if trimmedTicket == "" {
		return fmt.Errorf("metadata ticket id required")
	}
	trimmedCID := strings.TrimSpace(meta.ArtifactCID)
	if trimmedCID == "" {
		return fmt.Errorf("metadata artifact cid required")
	}
    meta.TicketID = trimmedTicket
    meta.ArtifactCID = trimmedCID

	envelope := metadataEnvelope{
		SchemaVersion:    contracts.SchemaVersion,
		SnapshotMetadata: meta,
	}

	payload, err := json.Marshal(envelope)
	if err != nil {
		return fmt.Errorf("encode metadata envelope: %w", err)
	}

    subjects := contracts.SubjectsForTenant("", trimmedTicket)
    subject := subjects.ArtifactStream
    if strings.TrimSpace(subject) == "" { return fmt.Errorf("artifact subject missing") }

	publishCtx := ctx
	if publishCtx == nil {
		publishCtx = context.Background()
	}

	var cancel context.CancelFunc
	if p.timeout > 0 {
		publishCtx, cancel = context.WithTimeout(publishCtx, p.timeout)
		defer cancel()
	}

	if _, err := p.js.Publish(subject, payload, nats.Context(publishCtx)); err != nil {
		return fmt.Errorf("publish snapshot metadata: %w", err)
	}
	return nil
}

type metadataEnvelope struct {
	SchemaVersion string `json:"schema_version"`
	SnapshotMetadata
}
