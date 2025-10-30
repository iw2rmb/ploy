package runner

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/iw2rmb/ploy/internal/workflow/contracts"
	"github.com/iw2rmb/ploy/internal/workflow/manifests"
)

type StageInvocation struct {
	TicketID  string
	Stage     Stage
	Workspace string
	RunID     string
	Archive   *StageArchive
	Evidence  *StageEvidence
	Artifacts []Artifact
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

	if strings.TrimSpace(stage.Lane) == "" {
		return StageOutcome{}, fmt.Errorf("lane missing for stage %s", stage.Name)
	}

	allowed := allowedLaneSet(stage.Constraints.Manifest.Lanes)
	if len(allowed) > 0 {
		lane := strings.TrimSpace(stage.Lane)
		if _, ok := allowed[lane]; !ok {
			manifestName := stage.Constraints.Manifest.Manifest.Name
			return StageOutcome{}, fmt.Errorf("lane %q not declared in manifest %s", lane, manifestName)
		}
	}

	g.invocations = append(g.invocations, StageInvocation{TicketID: ticket.TicketID, Stage: stage, Workspace: workspace})
	idx := len(g.invocations) - 1

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
	if idx >= 0 && idx < len(g.invocations) {
		g.invocations[idx].RunID = outcome.RunID
		g.invocations[idx].Archive = outcome.Archive
		g.invocations[idx].Evidence = outcome.Evidence
		if len(outcome.Artifacts) > 0 {
			g.invocations[idx].Artifacts = cloneArtifacts(outcome.Artifacts)
		}
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

func cloneArtifacts(src []Artifact) []Artifact {
	if len(src) == 0 {
		return nil
	}
	dst := make([]Artifact, len(src))
	copy(dst, src)
	return dst
}

func (g *InMemoryGrid) CancelWorkflow(ctx context.Context, req CancelRequest) (CancelResult, error) {
	_ = ctx
	_ = req
    return CancelResult{}, ErrCancellationUnsupported
}

func allowedLaneSet(set manifests.LaneSet) map[string]struct{} {
	allowed := make(map[string]struct{})
	for _, lane := range set.Required {
		if trimmed := strings.TrimSpace(lane.Name); trimmed != "" {
			allowed[trimmed] = struct{}{}
		}
	}
	for _, lane := range set.Allowed {
		if trimmed := strings.TrimSpace(lane.Name); trimmed != "" {
			allowed[trimmed] = struct{}{}
		}
	}
	return allowed
}
