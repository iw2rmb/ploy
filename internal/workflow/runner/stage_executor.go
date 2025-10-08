package runner

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

type stageExecutor struct {
	events      EventsClient
	grid        GridClient
	ticket      contracts.WorkflowTicket
	workspace   string
	maxRetries  int
	publishMu   *sync.Mutex
	failureOnce *sync.Once
	jobComposer JobComposer
}

func (e *stageExecutor) runStage(ctx context.Context, stage Stage) (Stage, StageOutcome, error) {
	attempt := 0
	for {
		if e.jobComposer != nil {
			jobSpec, err := e.jobComposer.Compose(ctx, JobComposeRequest{Stage: stage, Ticket: e.ticket})
			if err != nil {
				return Stage{}, StageOutcome{}, err
			}
			stage.Job = jobSpec
		}
		if err := e.publishStage(ctx, stage, StageStatusRunning, nil); err != nil {
			return Stage{}, StageOutcome{}, err
		}
		outcome, execErr := e.grid.ExecuteStage(ctx, e.ticket, stage, e.workspace)
		if execErr != nil {
			if ctx.Err() != nil {
				return Stage{}, StageOutcome{}, ctx.Err()
			}
			return Stage{}, StageOutcome{}, execErr
		}

		executedStage := resolvedStage(stage, outcome.Stage)
		executedStage.CacheKey = stage.CacheKey

		status := outcome.Status
		if status == "" {
			status = StageStatusCompleted
		}

		if status == StageStatusFailed {
			if outcome.Retryable && attempt < e.maxRetries {
				stage = executedStage
				if err := e.publishStage(ctx, stage, StageStatusRetrying, nil); err != nil {
					return Stage{}, StageOutcome{}, err
				}
				attempt++
				continue
			}
			if err := e.publishStage(ctx, executedStage, StageStatusFailed, nil); err != nil {
				return Stage{}, StageOutcome{}, err
			}
			e.failureOnce.Do(func() {
				_ = e.publishWorkflowFailure(ctx)
			})
			message := strings.TrimSpace(outcome.Message)
			if message == "" {
				message = "stage failed"
			}
			return executedStage, outcome, fmt.Errorf("%w: stage %s: %s", ErrStageFailed, stage.Name, message)
		}

		if err := e.publishStage(ctx, executedStage, StageStatusCompleted, outcome.Artifacts); err != nil {
			return Stage{}, StageOutcome{}, err
		}
		return executedStage, outcome, nil
	}
}

func (e *stageExecutor) publishStage(ctx context.Context, stage Stage, status StageStatus, artifacts []Artifact) error {
	stageCopy := stage
	e.publishMu.Lock()
	defer e.publishMu.Unlock()
	return publishCheckpoint(ctx, e.events, e.ticket.TicketID, stage.Name, status, stage.CacheKey, &stageCopy, artifacts)
}

func (e *stageExecutor) publishWorkflowFailure(ctx context.Context) error {
	e.publishMu.Lock()
	defer e.publishMu.Unlock()
	return publishCheckpoint(ctx, e.events, e.ticket.TicketID, "workflow", StageStatusFailed, "", nil, nil)
}

func (e *stageExecutor) publishWorkflowCompletion(ctx context.Context) error {
	e.publishMu.Lock()
	defer e.publishMu.Unlock()
	return publishCheckpoint(ctx, e.events, e.ticket.TicketID, "workflow", StageStatusCompleted, "", nil, nil)
}
