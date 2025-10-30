package workflow

import (
	"context"
	"errors"
	"testing"

	"github.com/iw2rmb/ploy/internal/workflow/contracts"
	"github.com/iw2rmb/ploy/internal/workflow/runner"
)

type stubGridClient struct {
	callCount int
	lastCtx   context.Context
	lastReq   runner.CancelRequest
	result    runner.CancelResult
	err       error
}

func (s *stubGridClient) ExecuteStage(ctx context.Context, ticket contracts.WorkflowTicket, stage runner.Stage, workspace string) (runner.StageOutcome, error) {
	return runner.StageOutcome{}, errors.New("not implemented")
}

func (s *stubGridClient) CancelWorkflow(ctx context.Context, req runner.CancelRequest) (runner.CancelResult, error) {
	s.callCount++
	s.lastCtx = ctx
	s.lastReq = req
	return s.result, s.err
}

func TestCancelCommandRequiresClient(t *testing.T) {
	cmd := CancelCommand{}
	if _, err := cmd.Run(context.Background(), CancelOptions{}); !errors.Is(err, errMissingClient) {
		t.Fatalf("expected errMissingClient, got %v", err)
	}
}

func TestCancelCommandTrimsAndInvokesClient(t *testing.T) {
	stub := &stubGridClient{
		result: runner.CancelResult{RunID: "run-123"},
	}
	cmd := CancelCommand{Client: stub}
    options := CancelOptions{
        RunID:      " run-123 ",
        WorkflowID: " wf-789 ",
        Reason:     " cleanup ",
    }
	result, err := cmd.Run(nil, options)
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if stub.callCount != 1 {
		t.Fatalf("expected client to be called once, got %d", stub.callCount)
	}
	if stub.lastCtx == nil {
		t.Fatalf("expected non-nil context passed to client")
	}
	if stub.lastReq.RunID != "run-123" {
		t.Fatalf("expected run id trimmed to 'run-123', got %q", stub.lastReq.RunID)
	}
	if stub.lastReq.WorkflowID != "wf-789" {
		t.Fatalf("expected workflow id trimmed to 'wf-789', got %q", stub.lastReq.WorkflowID)
	}
	if stub.lastReq.Reason != "cleanup" {
		t.Fatalf("expected reason trimmed to 'cleanup', got %q", stub.lastReq.Reason)
	}
	if result.RunID != "run-123" {
		t.Fatalf("expected result to propagate, got %+v", result)
	}
}
