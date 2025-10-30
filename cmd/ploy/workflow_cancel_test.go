package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/iw2rmb/ploy/internal/workflow/contracts"
	"github.com/iw2rmb/ploy/internal/workflow/runner"
)

func TestHandleWorkflowCancelValidatesFlags(t *testing.T) {
	buf := &bytes.Buffer{}
err := handleWorkflowCancel([]string{}, buf)
	if err == nil {
		t.Fatal("expected error for missing run id")
	}
	if !strings.Contains(buf.String(), "Usage: ploy workflow cancel") {
		t.Fatalf("expected cancel usage, got %q", buf.String())
	}
}

func TestHandleWorkflowCancelRequiresGridEndpoint(t *testing.T) {
	prevFactory := gridFactory
	defer func() { gridFactory = prevFactory }()
	gridFactory = func() (runner.GridClient, error) { return runner.NewInMemoryGrid(), nil }
err := handleWorkflowCancel([]string{"--run-id", "run-123"}, io.Discard)
	if err == nil {
		t.Fatal("expected cancellation unsupported error")
	}
	expectedGridID := envLabel(gridIDEnv, gridIDFallbackEnv)
	expectedAPIKey := envLabel(gridAPIKeyEnv, gridAPIKeyFallbackEnv)
	if !strings.Contains(err.Error(), expectedGridID) || !strings.Contains(err.Error(), expectedAPIKey) {
		t.Fatalf("expected unsupported message, got %v", err)
	}
}

func TestHandleWorkflowCancelSuccess(t *testing.T) {
	prevFactory := gridFactory
	defer func() { gridFactory = prevFactory }()
	recorder := &recordingCancelGrid{}
	gridFactory = func() (runner.GridClient, error) { return recorder, nil }
	buf := &bytes.Buffer{}
err := handleWorkflowCancel([]string{"--run-id", "run-42", "--workflow", "build", "--reason", "testing"}, buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if recorder.request.RunID != "run-42" {
		t.Fatalf("expected run id run-42, got %s", recorder.request.RunID)
	}
	if recorder.request.WorkflowID != "build" {
		t.Fatalf("expected workflow id build, got %s", recorder.request.WorkflowID)
	}
	if recorder.request.Reason != "testing" {
		t.Fatalf("expected reason testing, got %s", recorder.request.Reason)
	}
	output := buf.String()
	if !strings.Contains(output, "Cancellation requested for run run-42") {
		t.Fatalf("expected confirmation output, got %q", output)
	}
}

type recordingCancelGrid struct {
	request runner.CancelRequest
}

func (g *recordingCancelGrid) ExecuteStage(ctx context.Context, ticket contracts.WorkflowTicket, stage runner.Stage, workspace string) (runner.StageOutcome, error) {
	return runner.StageOutcome{}, errors.New("not implemented")
}

func (g *recordingCancelGrid) CancelWorkflow(ctx context.Context, req runner.CancelRequest) (runner.CancelResult, error) {
	g.request = req
	return runner.CancelResult{RunID: req.RunID, Status: runner.StageStatusRunning, Requested: true}, nil
}

func (g *recordingCancelGrid) Invocations() []runner.StageInvocation {
	return nil
}
