package updates

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/nats-io/nats.go"

	"github.com/iw2rmb/ploy/api/selfupdate"
)

// TailOptions configures consumption of self-update status events.
type TailOptions struct {
	Stream        string
	SubjectPrefix string
	DurablePrefix string
	DeploymentID  string
	AckWait       time.Duration
	FetchTimeout  time.Duration
	Follow        bool
}

// ConsumeStatusEvents subscribes to self-update status events for a deployment and
// invokes the provided handler for each event. It returns when a terminal event is
// observed (unless Follow is true) or the context is cancelled.
func ConsumeStatusEvents(ctx context.Context, js nats.JetStreamContext, opts TailOptions, handler func(selfupdate.StatusEvent)) error {
	if js == nil {
		return fmt.Errorf("jetstream context required")
	}
	if opts.Stream == "" {
		return fmt.Errorf("status stream name required")
	}
	if opts.SubjectPrefix == "" {
		return fmt.Errorf("status subject prefix required")
	}
	if opts.DeploymentID == "" {
		return fmt.Errorf("deployment id required")
	}

	streamName := sanitizeStreamName(opts.Stream)
	if streamName == "" {
		return fmt.Errorf("status stream name required")
	}
	subject := fmt.Sprintf("%s.%s", opts.SubjectPrefix, opts.DeploymentID)
	durable := buildDurableName(opts.DurablePrefix, opts.DeploymentID)
	ackWait := opts.AckWait
	if ackWait <= 0 {
		ackWait = 30 * time.Second
	}
	fetchTimeout := opts.FetchTimeout
	if fetchTimeout <= 0 {
		fetchTimeout = 2 * time.Second
	}

	consumerCfg := &nats.ConsumerConfig{
		Durable:       durable,
		AckPolicy:     nats.AckExplicitPolicy,
		AckWait:       ackWait,
		FilterSubject: subject,
		DeliverPolicy: nats.DeliverAllPolicy,
		ReplayPolicy:  nats.ReplayInstantPolicy,
		MaxAckPending: 32,
	}

	if _, err := js.AddConsumer(streamName, consumerCfg); err != nil {
		if !errors.Is(err, nats.ErrConsumerNameAlreadyInUse) {
			return fmt.Errorf("ensure status consumer: %w", err)
		}
	}

	sub, err := js.PullSubscribe(subject, durable, nats.BindStream(streamName))
	if err != nil {
		return fmt.Errorf("subscribe status stream: %w", err)
	}
	defer func() { _ = sub.Unsubscribe() }()

	for {
		if err := ctx.Err(); err != nil {
			return nil
		}

		msgs, err := sub.Fetch(1, nats.MaxWait(fetchTimeout))
		if err != nil {
			if errors.Is(err, nats.ErrTimeout) {
				continue
			}
			if errors.Is(err, context.Canceled) {
				return nil
			}
			return fmt.Errorf("fetch status event: %w", err)
		}

		for _, msg := range msgs {
			var event selfupdate.StatusEvent
			if err := json.Unmarshal(msg.Data, &event); err != nil {
				_ = msg.Ack()
				continue
			}
			if event.DeploymentID == "" {
				event.DeploymentID = opts.DeploymentID
			}
			if event.Timestamp.IsZero() {
				event.Timestamp = time.Now().UTC().Round(time.Millisecond)
			}

			lane := "unknown"
			if event.Metadata != nil {
				if candidate := strings.TrimSpace(event.Metadata["lane"]); candidate != "" {
					lane = candidate
				}
			}
			recordStatusLag("cli", lane, time.Since(event.Timestamp))

			handler(event)
			_ = msg.Ack()

			if !opts.Follow && isTerminalPhase(event.Phase) {
				return nil
			}
		}
	}
}

var durableSanitizer = regexp.MustCompile(`[^a-zA-Z0-9_-]`)

func buildDurableName(prefix, deploymentID string) string {
	if prefix == "" {
		prefix = "updates-status-cli"
	}
	safeID := durableSanitizer.ReplaceAllString(strings.ToLower(deploymentID), "-")
	if len(safeID) > 48 {
		safeID = safeID[:48]
	}
	return fmt.Sprintf("%s-%s", prefix, safeID)
}

func isTerminalPhase(phase string) bool {
	switch strings.ToLower(phase) {
	case "completed", "failed", "aborted":
		return true
	default:
		return false
	}
}

func sanitizeStreamName(name string) string {
	if name == "" {
		return name
	}
	sanitized := make([]rune, 0, len(name))
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' {
			sanitized = append(sanitized, r)
			continue
		}
		sanitized = append(sanitized, '_')
	}
	return string(sanitized)
}
