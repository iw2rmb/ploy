package executor_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/iw2rmb/ploy/internal/api/controlplane"
	"github.com/iw2rmb/ploy/internal/api/executor"
	"github.com/iw2rmb/ploy/internal/node/logstream"
	"github.com/iw2rmb/ploy/internal/workflow/runner"
	workflowruntime "github.com/iw2rmb/ploy/internal/workflow/runtime"
)

func TestExecutorResolvesRuntime(t *testing.T) {
	registry := workflowruntime.NewRegistry()
	if err := registry.Register(stubAdapter{name: "local"}); err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	exec := executor.New(executor.Options{Registry: registry, DefaultAdapter: "local"})
	assignment := controlplane.Assignment{ID: "a1", Runtime: "local"}
	if _, err := exec.Execute(context.Background(), assignment); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
}

func TestExecutorRequiresAdapter(t *testing.T) {
	registry := workflowruntime.NewRegistry()
	exec := executor.New(executor.Options{Registry: registry, DefaultAdapter: "local"})
	if _, err := exec.Execute(context.Background(), controlplane.Assignment{ID: "a1"}); err == nil {
		t.Fatal("expected error for missing adapter")
	}
}

func TestExecutorPublishesLogsOnSuccess(t *testing.T) {
	t.Helper()
	registry := workflowruntime.NewRegistry()
	conn := &stubAdapter{name: "local"}
	if err := registry.Register(conn); err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	streams := logstream.NewHub(logstream.Options{HistorySize: 8})
	exec := executor.New(executor.Options{
		Registry:       registry,
		DefaultAdapter: "local",
		LogStreams:     streams,
		Clock: func() time.Time {
			return time.Date(2025, 10, 27, 21, 30, 0, 0, time.UTC)
		},
	})
	assign := controlplane.Assignment{ID: "job-1", Runtime: "local"}
	if _, err := exec.Execute(context.Background(), assign); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	sub, err := streams.Subscribe(context.Background(), "job-1", 0)
	if err != nil {
		t.Fatalf("Subscribe() error = %v", err)
	}
	defer sub.Cancel()

	events := drainEvents(t, sub.Events, 3)
	if len(events) != 3 {
		t.Fatalf("expected 3 events, got %d", len(events))
	}
	if events[0].Type != "log" || string(events[0].Data) == "" {
		t.Fatalf("first event should be log, got %+v", events[0])
	}
	if events[2].Type != "done" {
		t.Fatalf("expected status event, got %s", events[2].Type)
	}
}

func TestExecutorPublishesFailure(t *testing.T) {
	t.Helper()
	registry := workflowruntime.NewRegistry()
	if err := registry.Register(stubAdapter{name: "broken", err: errors.New("boom")}); err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	streams := logstream.NewHub(logstream.Options{HistorySize: 8})
	exec := executor.New(executor.Options{
		Registry:       registry,
		DefaultAdapter: "broken",
		LogStreams:     streams,
		Clock: func() time.Time {
			return time.Date(2025, 10, 27, 21, 31, 0, 0, time.UTC)
		},
	})
	assign := controlplane.Assignment{ID: "job-fail", Runtime: "broken"}
	if _, err := exec.Execute(context.Background(), assign); err == nil {
		t.Fatal("expected error from failing adapter")
	}
	sub, err := streams.Subscribe(context.Background(), "job-fail", 0)
	if err != nil {
		t.Fatalf("Subscribe() error = %v", err)
	}
	defer sub.Cancel()
	events := drainEvents(t, sub.Events, 3)
	if len(events) == 0 {
		t.Fatal("expected log events for failure")
	}
	last := events[len(events)-1]
	if last.Type != "done" {
		t.Fatalf("expected final status event, got %s", last.Type)
	}
}

type stubAdapter struct {
	name string
	err  error
}

func (s stubAdapter) Metadata() workflowruntime.AdapterMetadata {
	return workflowruntime.AdapterMetadata{Name: s.name}
}

func (s stubAdapter) Connect(context.Context) (runner.RuntimeClient, error) {
    return nil, s.err
}

func drainEvents(t *testing.T, ch <-chan logstream.Event, want int) []logstream.Event {
	t.Helper()
	events := make([]logstream.Event, 0, want)
	timeout := time.After(50 * time.Millisecond)
	for {
		select {
		case evt, ok := <-ch:
			if !ok {
				return events
			}
			events = append(events, evt)
			if len(events) >= want {
				return events
			}
		case <-timeout:
			return events
		}
	}
}
