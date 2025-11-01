package runner

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

type stageExecutor struct {
	events      EventsClient
	runtime     RuntimeClient
	ticket      contracts.WorkflowTicket
	workspace   string
	maxRetries  int
	publishMu   *sync.Mutex
	failureOnce *sync.Once
	jobComposer JobComposer
}

func (e *stageExecutor) runStage(ctx context.Context, stage Stage) (Stage, StageOutcome, error) {
	attempt := 0
	// Apply a conservative timeout to Mods lanes to avoid indefinite hangs.
	// Build Gate stages are not capped here; only lanes prefixed with "mods-".
	baseCtx := ctx
	if isModsLane(stage) {
		var cancel context.CancelFunc
		baseCtx, cancel = context.WithTimeout(ctx, 5*time.Minute)
		defer cancel()
	}
	for {
		if e.jobComposer != nil {
			jobSpec, err := e.jobComposer.Compose(ctx, JobComposeRequest{Stage: stage, Ticket: e.ticket})
			if err != nil {
				return Stage{}, StageOutcome{}, err
			}
			stage.Job = jobSpec
		}
		if err := e.publishStage(baseCtx, stage, StageStatusRunning, nil); err != nil {
			return Stage{}, StageOutcome{}, err
		}
		outcome, execErr := e.runtime.ExecuteStage(baseCtx, e.ticket, stage, e.workspace)
		if execErr != nil {
			if baseCtx.Err() != nil {
				return Stage{}, StageOutcome{}, baseCtx.Err()
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
				if err := e.publishStage(baseCtx, stage, StageStatusRetrying, nil); err != nil {
					return Stage{}, StageOutcome{}, err
				}
				attempt++
				continue
			}
			if err := e.publishStage(baseCtx, executedStage, StageStatusFailed, nil); err != nil {
				return Stage{}, StageOutcome{}, err
			}
			e.failureOnce.Do(func() {
				_ = e.publishWorkflowFailure(baseCtx)
			})
			message := strings.TrimSpace(outcome.Message)
			if message == "" {
				message = "stage failed"
			}
			return executedStage, outcome, fmt.Errorf("%w: stage %s: %s", ErrStageFailed, stage.Name, message)
		}

		if err := e.publishStage(baseCtx, executedStage, StageStatusCompleted, outcome.Artifacts); err != nil {
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

func isModsLane(stage Stage) bool {
	lane := strings.TrimSpace(stage.Lane)
	return strings.HasPrefix(lane, "mods-")
}
