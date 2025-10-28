package executor

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/iw2rmb/ploy/internal/api/controlplane"
	"github.com/iw2rmb/ploy/internal/node/logstream"
	workflowruntime "github.com/iw2rmb/ploy/internal/workflow/runtime"
)

// Options configure the assignment executor.
type Options struct {
	Registry       *workflowruntime.Registry
	DefaultAdapter string
	LogStreams     *logstream.Hub
	Clock          func() time.Time
}

// Executor resolves runtime adapters to process assignments.
type Executor struct {
	registry       *workflowruntime.Registry
	defaultAdapter string
	streams        *logstream.Hub
	now            func() time.Time
}

// New constructs an Executor instance.
func New(opts Options) *Executor {
	adapter := strings.ToLower(strings.TrimSpace(opts.DefaultAdapter))
	now := opts.Clock
	if now == nil {
		now = time.Now
	}
	return &Executor{
		registry:       opts.Registry,
		defaultAdapter: adapter,
		streams:        opts.LogStreams,
		now:            now,
	}
}

// Execute resolves the target runtime and establishes a connection.
func (e *Executor) Execute(ctx context.Context, assignment controlplane.Assignment) error {
	if e == nil || e.registry == nil {
		return errors.New("executor: registry not initialised")
	}
	name := strings.ToLower(strings.TrimSpace(assignment.Runtime))
	if name == "" {
		name = e.defaultAdapter
	}

	streamID := strings.TrimSpace(assignment.ID)
	logEnabled := streamID != "" && e.streams != nil
	if logEnabled {
		e.streams.Ensure(streamID)
	}

	now := func() string {
		return e.now().UTC().Format(time.RFC3339Nano)
	}
	publishLog := func(line string) {
		if !logEnabled || strings.TrimSpace(line) == "" {
			return
		}
		if err := e.streams.PublishLog(ctx, streamID, logstream.LogRecord{
			Timestamp: now(),
			Stream:    "stdout",
			Line:      line,
		}); err != nil && !errors.Is(err, logstream.ErrStreamClosed) {
			log.Printf("executor: publish log failed for job %s: %v", streamID, err)
		}
	}
	publishStatus := func(status string) {
		if !logEnabled {
			return
		}
		if err := e.streams.PublishStatus(ctx, streamID, logstream.Status{Status: status}); err != nil && !errors.Is(err, logstream.ErrStreamClosed) {
			log.Printf("executor: publish status failed for job %s: %v", streamID, err)
		}
	}

	if name == "" {
		publishLog("executor: runtime unspecified")
		publishStatus("failed")
		return errors.New("executor: runtime not specified")
	}

	adapter, _, err := e.registry.Resolve(name)
	if err != nil {
		publishLog(fmt.Sprintf("executor: resolve runtime %q failed: %v", name, err))
		publishStatus("failed")
		return err
	}

	publishLog(fmt.Sprintf("executor: resolved runtime %s", name))
	if _, err := adapter.Connect(ctx); err != nil {
		publishLog(fmt.Sprintf("executor: connect runtime %s failed: %v", name, err))
		publishStatus("failed")
		return err
	}

	publishLog(fmt.Sprintf("executor: runtime %s connection established", name))
	publishStatus("completed")
	return nil
}
