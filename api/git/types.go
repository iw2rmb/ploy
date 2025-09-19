package git

import (
	"context"
	"sync"
)

// CommandRunner executes system commands; allows substitution in tests.
type CommandRunner interface {
	Run(ctx context.Context, dir string, name string, args ...string) error
}

// EventType represents the type of event emitted by git operations.
type EventType string

const (
	EventStarted   EventType = "started"
	EventProgress  EventType = "progress"
	EventCompleted EventType = "completed"
	EventFailed    EventType = "failed"
)

// Event captures the lifecycle of a git operation.
type Event struct {
	Type      EventType
	Operation string
	Message   string
	Err       error
}

// EventSink receives emitted git operation events.
type EventSink interface {
	Publish(Event)
}

// Operation represents a long-running git action with observable events.
type Operation struct {
	name   string
	events chan Event
	done   chan struct{}
	once   sync.Once
	mu     sync.Mutex
	err    error
}

// newOperation constructs an Operation with pre-wired channels for event delivery.
func newOperation(name string) *Operation {
	return &Operation{name: name, events: make(chan Event, 6), done: make(chan struct{})}
}

// Events returns a channel for streaming operation events until completion.
func (o *Operation) Events() <-chan Event { return o.events }

// Wait blocks until the operation completes and returns its terminal error, if any.
func (o *Operation) Wait() error {
	<-o.done
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.err
}

// Err is an alias for Wait to interoperate with legacy call sites.
func (o *Operation) Err() error { return o.Wait() }

// emit queues an intermediate event for observers without closing the operation.
func (o *Operation) emit(event Event) {
	o.events <- event
}

// finalize records the terminal event, captures any error, and closes the operation channels.
func (o *Operation) finalize(event Event) {
	o.mu.Lock()
	o.err = event.Err
	o.mu.Unlock()
	o.events <- event
	o.once.Do(func() {
		close(o.events)
		close(o.done)
	})
}
