package selfupdate

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	nats "github.com/nats-io/nats.go"
)

type WorkQueueConfig struct {
	Stream        string
	SubjectPrefix string
	DurablePrefix string
	Lane          string
	AckWait       time.Duration
	MaxAckPending int
	MaxDeliver    int
	Replicas      int
}

type WorkQueueTask struct {
	DeploymentID string
	SubmittedBy  string
	Request      UpdateRequest
	Metadata     map[string]string
}

type WorkQueueMessage struct {
	DeploymentID string
	Request      UpdateRequest
	Metadata     map[string]string
	SubmittedBy  string
	SubmittedAt  time.Time
	Lane         string
	Headers      nats.Header
	msg          *nats.Msg
}

func (m *WorkQueueMessage) Ack() error {
	if m.msg == nil {
		return errors.New("nil message ack")
	}
	return m.msg.Ack()
}

func (m *WorkQueueMessage) NakWithDelay(delay time.Duration) error {
	if m.msg == nil {
		return errors.New("nil message nack")
	}
	return m.msg.NakWithDelay(delay)
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

type queuePayload struct {
	DeploymentID string            `json:"deployment_id"`
	SubmittedBy  string            `json:"submitted_by,omitempty"`
	SubmittedAt  time.Time         `json:"submitted_at"`
	Request      UpdateRequest     `json:"request"`
	Metadata     map[string]string `json:"metadata,omitempty"`
	Lane         string            `json:"lane"`
}

type JetStreamWorkQueue struct {
	js       nats.JetStreamContext
	cfg      WorkQueueConfig
	stream   string
	subject  string
	durable  string
	consumer *nats.Subscription
	ackWait  time.Duration
}

func NewJetStreamWorkQueue(ctx context.Context, js nats.JetStreamContext, cfg WorkQueueConfig) (*JetStreamWorkQueue, error) {
	if cfg.Stream == "" || cfg.SubjectPrefix == "" || cfg.DurablePrefix == "" || cfg.Lane == "" {
		return nil, fmt.Errorf("work queue config missing required fields")
	}

	streamName := sanitizeStreamName(cfg.Stream)
	subject := fmt.Sprintf("%s.%s", cfg.SubjectPrefix, cfg.Lane)
	durable := fmt.Sprintf("%s-%s", cfg.DurablePrefix, cfg.Lane)

	ackWait := cfg.AckWait
	if ackWait == 0 {
		ackWait = 2 * time.Minute
	}
	maxAckPending := cfg.MaxAckPending
	if maxAckPending <= 0 {
		maxAckPending = 1
	}
	maxDeliver := cfg.MaxDeliver
	if maxDeliver <= 0 {
		maxDeliver = 5
	}
	replicas := cfg.Replicas
	if replicas <= 0 {
		replicas = 1
	}

	streamCfg := &nats.StreamConfig{
		Name:       streamName,
		Subjects:   []string{subject},
		Retention:  nats.WorkQueuePolicy,
		Storage:    nats.FileStorage,
		Duplicates: 24 * time.Hour,
		Replicas:   replicas,
	}

	if _, err := js.AddStream(streamCfg, nats.Context(ctx)); err != nil {
		if !errors.Is(err, nats.ErrStreamNameAlreadyInUse) {
			return nil, fmt.Errorf("add stream %s: %w", streamName, err)
		}
		if _, err := js.UpdateStream(streamCfg, nats.Context(ctx)); err != nil {
			return nil, fmt.Errorf("update stream %s: %w", streamName, err)
		}
	}

	consumerCfg := &nats.ConsumerConfig{
		Durable:       durable,
		AckPolicy:     nats.AckExplicitPolicy,
		AckWait:       ackWait,
		FilterSubject: subject,
		DeliverPolicy: nats.DeliverAllPolicy,
		MaxAckPending: maxAckPending,
		MaxDeliver:    maxDeliver,
		ReplayPolicy:  nats.ReplayInstantPolicy,
	}

	if _, err := js.AddConsumer(streamName, consumerCfg, nats.Context(ctx)); err != nil {
		if !errors.Is(err, nats.ErrConsumerNameAlreadyInUse) {
			return nil, fmt.Errorf("add consumer %s: %w", durable, err)
		}
		if _, err := js.UpdateConsumer(streamName, consumerCfg, nats.Context(ctx)); err != nil {
			return nil, fmt.Errorf("update consumer %s: %w", durable, err)
		}
	}

	sub, err := js.PullSubscribe(subject, durable, nats.BindStream(streamName), nats.ManualAck())
	if err != nil {
		return nil, fmt.Errorf("subscribe work queue: %w", err)
	}

	return &JetStreamWorkQueue{
		js:       js,
		cfg:      cfg,
		stream:   streamName,
		subject:  subject,
		durable:  durable,
		consumer: sub,
		ackWait:  ackWait,
	}, nil
}

func (q *JetStreamWorkQueue) DefaultFetchTimeout() time.Duration {
	if q.ackWait <= 0 {
		return 2 * time.Second
	}
	if q.ackWait < 2*time.Second {
		return q.ackWait
	}
	return q.ackWait / 2
}

func (q *JetStreamWorkQueue) Enqueue(ctx context.Context, task WorkQueueTask) error {
	if task.DeploymentID == "" {
		return fmt.Errorf("deployment id required")
	}

	payload := queuePayload{
		DeploymentID: task.DeploymentID,
		SubmittedBy:  task.SubmittedBy,
		SubmittedAt:  time.Now().UTC().Round(time.Millisecond),
		Request:      task.Request,
		Metadata:     task.Metadata,
		Lane:         q.cfg.Lane,
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal task payload: %w", err)
	}

	msg := &nats.Msg{
		Subject: q.subject,
		Header:  nats.Header{},
		Data:    data,
	}

	msg.Header.Set("Nats-Msg-Id", task.DeploymentID)
	if task.SubmittedBy != "" {
		msg.Header.Set("X-Ploy-Submitted-By", task.SubmittedBy)
	}
	msg.Header.Set("X-Ploy-Deployment", task.DeploymentID)
	msg.Header.Set("X-Ploy-Lane", q.cfg.Lane)

	if _, err := q.js.PublishMsg(msg, nats.Context(ctx)); err != nil {
		return fmt.Errorf("publish task: %w", err)
	}

	return nil
}

func (q *JetStreamWorkQueue) Fetch(ctx context.Context, wait time.Duration) (*WorkQueueMessage, error) {
	if wait <= 0 {
		wait = time.Second
	}

	msgs, err := q.consumer.Fetch(1, nats.MaxWait(wait))
	if err != nil {
		if errors.Is(err, nats.ErrTimeout) {
			return nil, nil
		}
		return nil, err
	}
	if len(msgs) == 0 {
		return nil, nil
	}

	msg := msgs[0]
	var payload queuePayload
	if err := json.Unmarshal(msg.Data, &payload); err != nil {
		_ = msg.Term()
		return nil, fmt.Errorf("decode task payload: %w", err)
	}

	return &WorkQueueMessage{
		DeploymentID: payload.DeploymentID,
		Request:      payload.Request,
		Metadata:     payload.Metadata,
		SubmittedBy:  payload.SubmittedBy,
		SubmittedAt:  payload.SubmittedAt,
		Lane:         payload.Lane,
		Headers:      msg.Header,
		msg:          msg,
	}, nil
}

type StatusStreamConfig struct {
	Stream        string
	SubjectPrefix string
	DurablePrefix string
	Replicas      int
	MaxAge        time.Duration
}

type StatusEvent struct {
	DeploymentID string            `json:"deployment_id"`
	Phase        string            `json:"phase"`
	Progress     int               `json:"progress"`
	Message      string            `json:"message,omitempty"`
	Executor     string            `json:"executor,omitempty"`
	Metadata     map[string]string `json:"metadata,omitempty"`
	Timestamp    time.Time         `json:"timestamp"`
}

// ErrStatusEventNotFound is returned when no status events exist for a deployment.
var ErrStatusEventNotFound = errors.New("status event not found")

type StatusPublisher struct {
	js      nats.JetStreamContext
	cfg     StatusStreamConfig
	stream  string
	subject string
}

func NewStatusPublisher(ctx context.Context, js nats.JetStreamContext, cfg StatusStreamConfig) (*StatusPublisher, error) {
	if cfg.Stream == "" || cfg.SubjectPrefix == "" {
		return nil, fmt.Errorf("status stream config missing required fields")
	}

	replicas := cfg.Replicas
	if replicas <= 0 {
		replicas = 1
	}
	maxAge := cfg.MaxAge
	if maxAge <= 0 {
		maxAge = 72 * time.Hour
	}

	streamName := sanitizeStreamName(cfg.Stream)

	duplicates := 24 * time.Hour
	if maxAge > 0 && maxAge < duplicates {
		duplicates = maxAge
	}

	streamCfg := &nats.StreamConfig{
		Name:       streamName,
		Subjects:   []string{fmt.Sprintf("%s.>", cfg.SubjectPrefix)},
		Retention:  nats.LimitsPolicy,
		Storage:    nats.FileStorage,
		Replicas:   replicas,
		MaxAge:     maxAge,
		Duplicates: duplicates,
	}

	if _, err := js.AddStream(streamCfg, nats.Context(ctx)); err != nil {
		if !errors.Is(err, nats.ErrStreamNameAlreadyInUse) {
			return nil, fmt.Errorf("add status stream %s: %w", streamName, err)
		}
		if _, err := js.UpdateStream(streamCfg, nats.Context(ctx)); err != nil {
			return nil, fmt.Errorf("update status stream %s: %w", streamName, err)
		}
	}

	return &StatusPublisher{
		js:      js,
		cfg:     cfg,
		stream:  streamName,
		subject: cfg.SubjectPrefix,
	}, nil
}

func (p *StatusPublisher) Publish(ctx context.Context, event StatusEvent) error {
	if event.DeploymentID == "" {
		return fmt.Errorf("deployment id required")
	}
	if event.Phase == "" {
		event.Phase = "unknown"
	}
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now().UTC().Round(time.Millisecond)
	}

	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal status event: %w", err)
	}

	subject := fmt.Sprintf("%s.%s", p.subject, event.DeploymentID)
	msg := &nats.Msg{
		Subject: subject,
		Header:  nats.Header{},
		Data:    data,
	}

	msg.Header.Set("X-Ploy-Phase", event.Phase)
	msg.Header.Set("X-Ploy-Progress", fmt.Sprintf("%d", event.Progress))
	msg.Header.Set("X-Ploy-Deployment", event.DeploymentID)
	if event.Executor != "" {
		msg.Header.Set("X-Ploy-Executor", event.Executor)
	}

	if _, err := p.js.PublishMsg(msg, nats.Context(ctx)); err != nil {
		return fmt.Errorf("publish status event: %w", err)
	}

	return nil
}

// LastEvent fetches the most recent status event for a deployment.
func (p *StatusPublisher) LastEvent(ctx context.Context, deploymentID string) (*StatusEvent, error) {
	if p == nil {
		return nil, fmt.Errorf("status publisher unavailable")
	}
	if deploymentID == "" {
		return nil, ErrStatusEventNotFound
	}

	if ctx == nil {
		ctx = context.Background()
	}

	subject := fmt.Sprintf("%s.%s", p.subject, deploymentID)
	msg, err := p.js.GetLastMsg(p.stream, subject, nats.Context(ctx))
	if err != nil {
		if errors.Is(err, nats.ErrMsgNotFound) {
			return nil, ErrStatusEventNotFound
		}
		return nil, fmt.Errorf("fetch last status event: %w", err)
	}

	var event StatusEvent
	if err := json.Unmarshal(msg.Data, &event); err != nil {
		return nil, fmt.Errorf("decode status event: %w", err)
	}

	if event.DeploymentID == "" {
		event.DeploymentID = deploymentID
	}
	if event.Metadata != nil {
		event.Metadata = cloneMetadata(event.Metadata)
	}
	if event.Timestamp.IsZero() {
		event.Timestamp = msg.Time.UTC()
	}

	return &event, nil
}
