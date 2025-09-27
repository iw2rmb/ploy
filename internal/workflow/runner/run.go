package runner

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Run executes the workflow pipeline for the claimed ticket end-to-end.
func Run(ctx context.Context, opts Options) (err error) {
	if opts.Events == nil {
		return ErrEventsClientRequired
	}
	if opts.Grid == nil {
		return ErrGridClientRequired
	}
	if opts.ManifestCompiler == nil {
		return ErrManifestCompilerRequired
	}

	planner := opts.Planner
	if planner == nil {
		planner = NewDefaultPlannerWithMods(opts.Mods)
	}

	trimmedTicket := strings.TrimSpace(opts.Ticket)

	ticket, err := opts.Events.ClaimTicket(ctx, trimmedTicket)
	if err != nil {
		return err
	}
	if err := ticket.Validate(); err != nil {
		return fmt.Errorf("%w: %v", ErrTicketValidationFailed, err)
	}

	compiledManifest, err := opts.ManifestCompiler.Compile(ctx, ticket.Manifest)
	if err != nil {
		return err
	}

	plan, err := planner.Build(ctx, ticket)
	if err != nil {
		return err
	}

	composer := opts.CacheComposer
	if composer == nil {
		composer = defaultCacheComposer{}
	}

	workspaceRoot := strings.TrimSpace(opts.WorkspaceRoot)
	if workspaceRoot == "" {
		workspaceRoot = os.TempDir()
	}

	if err := os.MkdirAll(workspaceRoot, 0o755); err != nil {
		return fmt.Errorf("create workspace root: %w", err)
	}

	workspace, err := os.MkdirTemp(workspaceRoot, "ploy-workflow-")
	if err != nil {
		return fmt.Errorf("create workspace: %w", err)
	}

	workspace = filepath.Clean(workspace)
	defer func() {
		if removeErr := os.RemoveAll(workspace); removeErr != nil {
			err = errors.Join(err, fmt.Errorf("workspace cleanup: %w", removeErr))
		}
	}()

	if err := publishCheckpoint(ctx, opts.Events, ticket.TicketID, "ticket-claimed", StageStatusCompleted, "", nil, nil); err != nil {
		return err
	}

	maxRetries := opts.MaxStageRetries
	if maxRetries < 0 {
		maxRetries = 0
	}

	for _, stage := range plan.Stages {
		if strings.TrimSpace(stage.Lane) == "" {
			return fmt.Errorf("%w: %s", ErrLaneRequired, stage.Name)
		}
		stage.Constraints.Manifest = compiledManifest
		asterMeta, err := resolveStageAster(ctx, stage, compiledManifest, opts.Aster)
		if err != nil {
			return err
		}
		stage.Aster = asterMeta
		cacheKey, err := composer.Compose(ctx, CacheComposeRequest{Stage: stage, Ticket: ticket})
		if err != nil {
			return fmt.Errorf("compose cache key for stage %s: %w", stage.Name, err)
		}
		stage.CacheKey = strings.TrimSpace(cacheKey)
		for attempt := 0; ; attempt++ {
			runningStage := stage
			if err := publishCheckpoint(ctx, opts.Events, ticket.TicketID, stage.Name, StageStatusRunning, stage.CacheKey, &runningStage, nil); err != nil {
				return err
			}

			outcome, execErr := opts.Grid.ExecuteStage(ctx, ticket, stage, workspace)
			if execErr != nil {
				return execErr
			}

			executedStage := resolvedStage(stage, outcome.Stage)

			status := outcome.Status
			if status == "" {
				status = StageStatusCompleted
			}

			if status == StageStatusFailed {
				if outcome.Retryable && attempt < maxRetries {
					stageCopy := executedStage
					if err := publishCheckpoint(ctx, opts.Events, ticket.TicketID, stage.Name, StageStatusRetrying, stage.CacheKey, &stageCopy, nil); err != nil {
						return err
					}
					continue
				}

				stageCopy := executedStage
				if err := publishCheckpoint(ctx, opts.Events, ticket.TicketID, stage.Name, StageStatusFailed, stage.CacheKey, &stageCopy, nil); err != nil {
					return err
				}
				_ = publishCheckpoint(ctx, opts.Events, ticket.TicketID, "workflow", StageStatusFailed, "", nil, nil)

				message := outcome.Message
				if strings.TrimSpace(message) == "" {
					message = "stage failed"
				}
				return fmt.Errorf("%w: stage %s: %s", ErrStageFailed, stage.Name, message)
			}

			stageCopy := executedStage
			if err := publishCheckpoint(ctx, opts.Events, ticket.TicketID, stage.Name, StageStatusCompleted, stage.CacheKey, &stageCopy, outcome.Artifacts); err != nil {
				return err
			}
			break
		}
	}

	if err := publishCheckpoint(ctx, opts.Events, ticket.TicketID, "workflow", StageStatusCompleted, "", nil, nil); err != nil {
		return err
	}

	return nil
}
