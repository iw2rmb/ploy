package git

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// ExecRunner executes commands via os/exec.
type ExecRunner struct{}

// Run executes the command in the provided directory, capturing stderr for context.
func (ExecRunner) Run(ctx context.Context, dir string, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		stderrStr := strings.TrimSpace(stderr.String())
		if stderrStr != "" {
			return fmt.Errorf("%s %s: %w: %s", name, strings.Join(args, " "), err, stderrStr)
		}
		return err
	}
	return nil
}

// ServiceConfig configures a new Service instance.
type ServiceConfig struct {
	Runner    CommandRunner
	EventSink EventSink
}

// Service provides Git functionality and emits structured events.
type Service struct {
	runner CommandRunner
	sink   EventSink
}

// NewService constructs a Service with the provided configuration.
func NewService(cfg ServiceConfig) *Service {
	runner := cfg.Runner
	if runner == nil {
		runner = ExecRunner{}
	}
	return &Service{runner: runner, sink: cfg.EventSink}
}

// NewGitOperations preserves the legacy constructor signature with default configuration.
func NewGitOperations(workDir string) *Service {
	_ = workDir // deprecated; service no longer requires a working directory hint
	return NewService(ServiceConfig{})
}

// SetEventSink updates the sink used for emitting events; primarily for wiring observers post-construction.
func (g *Service) SetEventSink(sink EventSink) {
	g.sink = sink
}

// emit publishes an event to the operation and the optional sink.
func (g *Service) emit(op *Operation, event Event) {
	op.emit(event)
	if g.sink != nil {
		g.sink.Publish(event)
	}
}

// finalize publishes the terminal event and marks the operation complete.
func (g *Service) finalize(op *Operation, event Event) {
	op.finalize(event)
	if g.sink != nil {
		g.sink.Publish(event)
	}
}
