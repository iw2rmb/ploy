package fabric

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/nats-io/nats.go"
)

// Config captures stream and subject configuration for the platform event fabric.
type Config struct {
	MaxAge             time.Duration
	BuildStream        string
	BuildSubjectPrefix string
	BuildReplicas      int
	AllocStream        string
	AllocSubjectPrefix string
	AllocReplicas      int
	ModsStream         string
	ModsSubjectPrefix  string
	ModsReplicas       int
}

// Fabric coordinates publishing to JetStream streams that back platform telemetry events.
type Fabric struct {
	conn *nats.Conn
	js   nats.JetStreamContext
	cfg  Config
}

// New constructs a Fabric using an existing NATS connection and ensures streams exist.
func New(ctx context.Context, conn *nats.Conn, cfg Config) (*Fabric, error) {
	if conn == nil {
		return nil, errors.New("nats connection required")
	}
	js, err := conn.JetStream()
	if err != nil {
		return nil, fmt.Errorf("create jetstream context: %w", err)
	}
	fabric := &Fabric{conn: conn, js: js, cfg: cfg}
	if err := fabric.ensureStreams(ctx); err != nil {
		return nil, err
	}
	return fabric, nil
}

// Close closes the underlying NATS connection.
func (f *Fabric) Close() {
	if f != nil && f.conn != nil {
		f.conn.Close()
	}
}

// JetStream exposes the JetStream context for consumers (read-only).
func (f *Fabric) JetStream() nats.JetStreamContext { return f.js }

func (f *Fabric) BuildStream() string { return f.cfg.BuildStream }

func (f *Fabric) AllocationStream() string { return f.cfg.AllocStream }

func (f *Fabric) ModsStream() string { return f.cfg.ModsStream }

// BuildSubject produces the subject used for a build status entry.
func (f *Fabric) BuildSubject(buildID string) string {
	return joinSubject(f.cfg.BuildSubjectPrefix, buildID)
}

// AllocationSubject produces the subject for an allocation readiness event.
func (f *Fabric) AllocationSubject(jobID string) string {
	return joinSubject(f.cfg.AllocSubjectPrefix, jobID)
}

// ModsSubject produces the subject for a Mods telemetry event.
func (f *Fabric) ModsSubject(modID string) string {
	return joinSubject(f.cfg.ModsSubjectPrefix, modID)
}

// BuildStatusEvent represents an async build status transition.
type BuildStatusEvent struct {
	ID        string    `json:"id"`
	App       string    `json:"app,omitempty"`
	Status    string    `json:"status,omitempty"`
	Code      int       `json:"code,omitempty"`
	Message   string    `json:"message,omitempty"`
	StartedAt string    `json:"started_at,omitempty"`
	EndedAt   string    `json:"ended_at,omitempty"`
	Timestamp time.Time `json:"ts"`
}

// AllocationReadyEvent captures readiness info for a Nomad allocation.
type AllocationReadyEvent struct {
	JobID         string            `json:"job_id"`
	AllocID       string            `json:"alloc_id"`
	ClientStatus  string            `json:"client_status,omitempty"`
	DesiredStatus string            `json:"desired_status,omitempty"`
	HealthyCount  int               `json:"healthy_count"`
	ObservedAt    time.Time         `json:"ts"`
	TaskSummaries map[string]string `json:"task_summaries,omitempty"`
}

// ModsEvent mirrors runner telemetry for Mods workflows.
type ModsEvent struct {
	ModID   string    `json:"mod_id"`
	Phase   string    `json:"phase,omitempty"`
	Step    string    `json:"step,omitempty"`
	Level   string    `json:"level,omitempty"`
	Message string    `json:"message,omitempty"`
	Time    time.Time `json:"ts"`
	JobName string    `json:"job_name,omitempty"`
	AllocID string    `json:"alloc_id,omitempty"`
}

// PublishBuildStatus publishes a build status transition to the configured stream.
func (f *Fabric) PublishBuildStatus(ctx context.Context, ev BuildStatusEvent) error {
	if ev.ID == "" {
		return errors.New("build id required")
	}
	if ev.Timestamp.IsZero() {
		ev.Timestamp = time.Now().UTC()
	}
	return f.publish(ctx, f.cfg.BuildStream, f.BuildSubject(ev.ID), f.cfg.BuildReplicas, ev)
}

// PublishAllocationReady publishes a readiness snapshot for a Nomad allocation.
func (f *Fabric) PublishAllocationReady(ctx context.Context, ev AllocationReadyEvent) error {
	if ev.JobID == "" || ev.AllocID == "" {
		return errors.New("job id and alloc id required")
	}
	if ev.ObservedAt.IsZero() {
		ev.ObservedAt = time.Now().UTC()
	}
	return f.publish(ctx, f.cfg.AllocStream, f.AllocationSubject(ev.JobID), f.cfg.AllocReplicas, ev)
}

// PublishModsEvent publishes Mods runner telemetry to JetStream.
func (f *Fabric) PublishModsEvent(ctx context.Context, ev ModsEvent) error {
	if ev.ModID == "" {
		return errors.New("mod id required")
	}
	if ev.Time.IsZero() {
		ev.Time = time.Now().UTC()
	}
	return f.publish(ctx, f.cfg.ModsStream, f.ModsSubject(ev.ModID), f.cfg.ModsReplicas, ev)
}

func (f *Fabric) ensureStreams(ctx context.Context) error {
	if err := f.ensureStream(ctx, f.cfg.BuildStream, f.cfg.BuildSubjectPrefix, f.cfg.BuildReplicas); err != nil {
		return fmt.Errorf("ensure build stream: %w", err)
	}
	if err := f.ensureStream(ctx, f.cfg.AllocStream, f.cfg.AllocSubjectPrefix, f.cfg.AllocReplicas); err != nil {
		return fmt.Errorf("ensure alloc stream: %w", err)
	}
	if err := f.ensureStream(ctx, f.cfg.ModsStream, f.cfg.ModsSubjectPrefix, f.cfg.ModsReplicas); err != nil {
		return fmt.Errorf("ensure mods stream: %w", err)
	}
	return nil
}

func (f *Fabric) ensureStream(ctx context.Context, name, subjectPrefix string, replicas int) error {
	if name == "" || subjectPrefix == "" {
		return errors.New("stream name and subject prefix required")
	}
	cfg := &nats.StreamConfig{
		Name:              name,
		Subjects:          []string{subjectPrefix + ".>"},
		Retention:         nats.LimitsPolicy,
		MaxMsgsPerSubject: 2048,
		MaxAge:            sanitizeAge(f.cfg.MaxAge),
		Replicas:          sanitizeReplicas(replicas),
		Storage:           nats.FileStorage,
	}
	if _, err := f.js.AddStream(cfg, nats.Context(ctx)); err != nil {
		if !errors.Is(err, nats.ErrStreamNameAlreadyInUse) {
			return err
		}
		// Update ensures new subject prefixes/limits are applied.
		if _, err := f.js.UpdateStream(cfg, nats.Context(ctx)); err != nil {
			return err
		}
	}
	return nil
}

func (f *Fabric) publish(ctx context.Context, stream, subject string, replicas int, payload interface{}) error {
	if subject == "" {
		return errors.New("subject required")
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal event: %w", err)
	}
	msg := &nats.Msg{Subject: subject, Data: b}
	_, err = f.js.PublishMsg(msg, nats.Context(ctx))
	return err
}

func sanitizeAge(age time.Duration) time.Duration {
	if age <= 0 {
		return 72 * time.Hour
	}
	return age
}

func sanitizeReplicas(replicas int) int {
	if replicas <= 0 {
		return 1
	}
	return replicas
}

var invalidToken = regexp.MustCompile(`[^A-Za-z0-9-_]`)

func joinSubject(prefix, token string) string {
	if prefix == "" {
		return ""
	}
	token = strings.TrimSpace(token)
	if token == "" {
		return prefix
	}
	sanitized := invalidToken.ReplaceAllString(strings.ToLower(token), "-")
	return prefix + "." + sanitized
}
