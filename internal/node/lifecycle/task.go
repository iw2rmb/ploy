package lifecycle

import (
	"context"
	"strings"
	"time"
)

// PublishTask periodically publishes lifecycle snapshots to etcd.
type PublishTask struct {
	name      string
	interval  time.Duration
	publisher *Publisher
}

// PublishTaskOptions configure the lifecycle publish task.
type PublishTaskOptions struct {
	Name      string
	Interval  time.Duration
	Publisher *Publisher
}

// NewPublishTask constructs a scheduler task that invokes the publisher.
func NewPublishTask(opts PublishTaskOptions) *PublishTask {
	name := strings.TrimSpace(opts.Name)
	if name == "" {
		name = "lifecycle-heartbeat"
	}
	interval := opts.Interval
	if interval <= 0 {
		interval = 30 * time.Second
	}
	return &PublishTask{
		name:      name,
		interval:  interval,
		publisher: opts.Publisher,
	}
}

// Name returns the task identifier.
func (p *PublishTask) Name() string {
	return p.name
}

// Interval returns the execution cadence for the task.
func (p *PublishTask) Interval() time.Duration {
	return p.interval
}

// Run executes a lifecycle publish cycle.
func (p *PublishTask) Run(ctx context.Context) error {
	if p == nil || p.publisher == nil {
		return nil
	}
	return p.publisher.Publish(ctx)
}
