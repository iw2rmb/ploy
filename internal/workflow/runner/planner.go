package runner

import (
	"context"
	"time"

	"github.com/iw2rmb/ploy/internal/workflow/contracts"
	"github.com/iw2rmb/ploy/internal/workflow/mods"
)

type ExecutionPlan struct {
	TicketID string
	Stages   []Stage
}

type Planner interface {
	Build(ctx context.Context, ticket contracts.WorkflowTicket) (ExecutionPlan, error)
}

const (
	buildGateStageName    = "build-gate"
	staticChecksStageName = "static-checks"
)

type DefaultPlanner struct {
	mods ModsOptions
}

// ModsOptions configures the Mods planner invoked by the workflow runner.
type ModsOptions struct {
	PlanTimeout time.Duration
	MaxParallel int
	Advisor     mods.Advisor
}

// NewDefaultPlanner constructs a planner that uses default Mods settings.
func NewDefaultPlanner() Planner {
	return DefaultPlanner{}
}

// NewDefaultPlannerWithMods constructs a planner with custom Mods options.
func NewDefaultPlannerWithMods(opts ModsOptions) Planner {
	return DefaultPlanner{mods: opts}
}

// Build generates the execution plan using the Mods planner output plus build/test stages.
func (p DefaultPlanner) Build(ctx context.Context, ticket contracts.WorkflowTicket) (ExecutionPlan, error) {
	modPlanner := mods.NewPlanner(mods.Options{
		PlanTimeout: p.mods.PlanTimeout,
		MaxParallel: p.mods.MaxParallel,
		Advisor:     p.mods.Advisor,
	})
	modStages, err := modPlanner.Plan(ctx, mods.PlanInput{Ticket: ticket})
	if err != nil {
		return ExecutionPlan{}, err
	}
	stages := make([]Stage, 0, len(modStages)+3)
	for _, stage := range modStages {
		stages = append(stages, Stage{
			Name:         stage.Name,
			Kind:         StageKind(stage.Kind),
			Lane:         stage.Lane,
			Dependencies: copyStringSlice(stage.Dependencies),
			Metadata:     convertStageMetadata(stage.Metadata),
		})
	}
	stages = append(stages,
		Stage{
			Name:         buildGateStageName,
			Kind:         StageKindBuildGate,
			Lane:         "go-native",
			Dependencies: []string{mods.StageNameHuman},
		},
		Stage{
			Name:         staticChecksStageName,
			Kind:         StageKindStaticChecks,
			Lane:         "go-native",
			Dependencies: []string{buildGateStageName},
		},
		Stage{
			Name:         "test",
			Kind:         StageKindTest,
			Lane:         "go-native",
			Dependencies: []string{staticChecksStageName},
		},
	)
	plan := ExecutionPlan{
		TicketID: ticket.TicketID,
		Stages:   stages,
	}
	return plan, nil
}
