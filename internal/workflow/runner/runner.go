package runner

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

var (
	ErrTicketRequired             = errors.New("ticket is required")
	ErrEventsClientRequired       = errors.New("events client is required")
	ErrGridClientRequired         = errors.New("grid client is required")
	ErrPlannerRequired            = errors.New("planner is required")
	ErrTicketValidationFailed     = errors.New("ticket payload failed validation")
	ErrCheckpointValidationFailed = errors.New("checkpoint payload failed validation")
	ErrStageFailed                = errors.New("workflow stage failed")
	ErrLaneRequired               = errors.New("lane is required")
)

type StageKind string

const (
	StageKindMods  StageKind = "mods"
	StageKindBuild StageKind = "build"
	StageKindTest  StageKind = "test"
)

type Stage struct {
	Name         string
	Kind         StageKind
	Lane         string
	Dependencies []string
}

type StageStatus = contracts.CheckpointStatus

const (
	StageStatusPending   StageStatus = contracts.CheckpointStatusPending
	StageStatusClaimed   StageStatus = contracts.CheckpointStatusClaimed
	StageStatusRunning   StageStatus = contracts.CheckpointStatusRunning
	StageStatusRetrying  StageStatus = contracts.CheckpointStatusRetrying
	StageStatusCompleted StageStatus = contracts.CheckpointStatusCompleted
	StageStatusFailed    StageStatus = contracts.CheckpointStatusFailed
)

type StageOutcome struct {
	Stage     Stage
	Status    StageStatus
	Retryable bool
	Message   string
}

type ExecutionPlan struct {
	TicketID string
	Stages   []Stage
}

type Planner interface {
	Build(ctx context.Context, ticket contracts.WorkflowTicket) (ExecutionPlan, error)
}

type DefaultPlanner struct{}

func NewDefaultPlanner() Planner {
	return DefaultPlanner{}
}

func (DefaultPlanner) Build(ctx context.Context, ticket contracts.WorkflowTicket) (ExecutionPlan, error) {
	_ = ctx
	plan := ExecutionPlan{
		TicketID: ticket.TicketID,
		Stages: []Stage{
			{Name: "mods", Kind: StageKindMods, Lane: "node-wasm"},
			{Name: "build", Kind: StageKindBuild, Lane: "go-native", Dependencies: []string{"mods"}},
			{Name: "test", Kind: StageKindTest, Lane: "go-native", Dependencies: []string{"build"}},
		},
	}
	return plan, nil
}

type GridClient interface {
	ExecuteStage(ctx context.Context, ticket contracts.WorkflowTicket, stage Stage, workspace string) (StageOutcome, error)
}

type EventsClient interface {
	ClaimTicket(ctx context.Context, ticketID string) (contracts.WorkflowTicket, error)
	PublishCheckpoint(ctx context.Context, checkpoint contracts.WorkflowCheckpoint) error
}

type Options struct {
	Ticket          string
	Tenant          string
	Events          EventsClient
	Grid            GridClient
	Planner         Planner
	WorkspaceRoot   string
	MaxStageRetries int
}

func Run(ctx context.Context, opts Options) (err error) {
	if opts.Events == nil {
		return ErrEventsClientRequired
	}
	if opts.Grid == nil {
		return ErrGridClientRequired
	}

	planner := opts.Planner
	if planner == nil {
		planner = NewDefaultPlanner()
	}

	trimmedTicket := strings.TrimSpace(opts.Ticket)

	ticket, err := opts.Events.ClaimTicket(ctx, trimmedTicket)
	if err != nil {
		return err
	}
	if err := ticket.Validate(); err != nil {
		return fmt.Errorf("%w: %v", ErrTicketValidationFailed, err)
	}

	plan, err := planner.Build(ctx, ticket)
	if err != nil {
		return err
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

	if err := publishCheckpoint(ctx, opts.Events, ticket.TicketID, "ticket-claimed", StageStatusCompleted); err != nil {
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
		for attempt := 0; ; attempt++ {
			if err := publishCheckpoint(ctx, opts.Events, ticket.TicketID, stage.Name, StageStatusRunning); err != nil {
				return err
			}

			outcome, execErr := opts.Grid.ExecuteStage(ctx, ticket, stage, workspace)
			if execErr != nil {
				return execErr
			}

			if outcome.Stage.Name == "" {
				outcome.Stage = stage
			}

			status := outcome.Status
			if status == "" {
				status = StageStatusCompleted
			}

			if status == StageStatusFailed {
				if outcome.Retryable && attempt < maxRetries {
					if err := publishCheckpoint(ctx, opts.Events, ticket.TicketID, stage.Name, StageStatusRetrying); err != nil {
						return err
					}
					continue
				}

				if err := publishCheckpoint(ctx, opts.Events, ticket.TicketID, stage.Name, StageStatusFailed); err != nil {
					return err
				}
				_ = publishCheckpoint(ctx, opts.Events, ticket.TicketID, "workflow", StageStatusFailed)

				message := outcome.Message
				if strings.TrimSpace(message) == "" {
					message = "stage failed"
				}
				return fmt.Errorf("%w: stage %s: %s", ErrStageFailed, stage.Name, message)
			}

			if err := publishCheckpoint(ctx, opts.Events, ticket.TicketID, stage.Name, StageStatusCompleted); err != nil {
				return err
			}
			break
		}
	}

	if err := publishCheckpoint(ctx, opts.Events, ticket.TicketID, "workflow", StageStatusCompleted); err != nil {
		return err
	}

	return nil
}

func publishCheckpoint(ctx context.Context, events EventsClient, ticketID, stage string, status StageStatus) error {
	checkpoint := contracts.WorkflowCheckpoint{
		SchemaVersion: contracts.SchemaVersion,
		TicketID:      ticketID,
		Stage:         stage,
		Status:        contracts.CheckpointStatus(status),
	}
	if err := checkpoint.Validate(); err != nil {
		return fmt.Errorf("%w: %v", ErrCheckpointValidationFailed, err)
	}
	return events.PublishCheckpoint(ctx, checkpoint)
}
