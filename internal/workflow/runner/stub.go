package runner

import (
	"context"
	"sync"

	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

type StageInvocation struct {
	TicketID  string
	Stage     Stage
	Workspace string
}

type InMemoryGrid struct {
	mu            sync.Mutex
	StageOutcomes map[string][]StageOutcome
	invocations   []StageInvocation
}

func NewInMemoryGrid() *InMemoryGrid {
	return &InMemoryGrid{StageOutcomes: make(map[string][]StageOutcome)}
}

func (g *InMemoryGrid) ExecuteStage(ctx context.Context, ticket contracts.WorkflowTicket, stage Stage, workspace string) (StageOutcome, error) {
	_ = ctx
	g.mu.Lock()
	defer g.mu.Unlock()

	g.invocations = append(g.invocations, StageInvocation{TicketID: ticket.TicketID, Stage: stage, Workspace: workspace})

	outcomes := g.StageOutcomes[stage.Name]
	if len(outcomes) == 0 {
		return StageOutcome{Stage: stage, Status: StageStatusCompleted}, nil
	}

	outcome := outcomes[0]
	g.StageOutcomes[stage.Name] = outcomes[1:]
	if outcome.Stage.Name == "" {
		outcome.Stage = stage
	}
	if outcome.Status == "" {
		outcome.Status = StageStatusCompleted
	}
	return outcome, nil
}

func (g *InMemoryGrid) Invocations() []StageInvocation {
	g.mu.Lock()
	defer g.mu.Unlock()
	dst := make([]StageInvocation, len(g.invocations))
	copy(dst, g.invocations)
	return dst
}
