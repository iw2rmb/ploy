package runner_test

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/iw2rmb/ploy/internal/workflow/aster"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
	"github.com/iw2rmb/ploy/internal/workflow/manifests"
	"github.com/iw2rmb/ploy/internal/workflow/runner"
)

type errorRuntime struct {
    err error
}

func (g errorRuntime) ExecuteStage(ctx context.Context, ticket contracts.WorkflowTicket, stage runner.Stage, workspace string) (runner.StageOutcome, error) {
	_ = ctx
	_ = ticket
	_ = stage
	_ = workspace
    return runner.StageOutcome{}, g.err
}

func (g errorRuntime) CancelWorkflow(ctx context.Context, req runner.CancelRequest) (runner.CancelResult, error) {
	_ = ctx
	_ = req
    return runner.CancelResult{}, runner.ErrCancellationUnsupported
}

type recordingCacheComposer struct {
	calls []runner.CacheComposeRequest
}

func (r *recordingCacheComposer) Compose(ctx context.Context, req runner.CacheComposeRequest) (string, error) {
	_ = ctx
	r.calls = append(r.calls, req)
	return fmt.Sprintf("cache-%s", strings.ToLower(req.Stage.Name)), nil
}

type statuslessRuntime struct{}

func (statuslessRuntime) ExecuteStage(ctx context.Context, ticket contracts.WorkflowTicket, stage runner.Stage, workspace string) (runner.StageOutcome, error) {
	_ = ctx
	_ = ticket
	_ = workspace
    return runner.StageOutcome{Stage: stage}, nil
}

func (statuslessRuntime) CancelWorkflow(ctx context.Context, req runner.CancelRequest) (runner.CancelResult, error) {
	_ = ctx
	_ = req
    return runner.CancelResult{}, runner.ErrCancellationUnsupported
}

type noStageRuntime struct{}

func (noStageRuntime) ExecuteStage(ctx context.Context, ticket contracts.WorkflowTicket, stage runner.Stage, workspace string) (runner.StageOutcome, error) {
	_ = ctx
	_ = ticket
	_ = workspace
    return runner.StageOutcome{Status: runner.StageStatusCompleted}, nil
}

func (noStageRuntime) CancelWorkflow(ctx context.Context, req runner.CancelRequest) (runner.CancelResult, error) {
	_ = ctx
	_ = req
    return runner.CancelResult{}, runner.ErrCancellationUnsupported
}

type runtimeCall struct {
    stage     runner.Stage
    workspace string
}

type fakeRuntime struct {
    mu            sync.Mutex
    outcomes      map[string][]runner.StageOutcome
    calls         []runtimeCall
    lastWorkspace string
}

func (g *fakeRuntime) ExecuteStage(ctx context.Context, ticket contracts.WorkflowTicket, stage runner.Stage, workspace string) (runner.StageOutcome, error) {
	_ = ctx
	_ = ticket
	g.mu.Lock()
    g.calls = append(g.calls, runtimeCall{stage: stage, workspace: workspace})
	g.lastWorkspace = workspace
	queue := g.outcomes[stage.Name]
	var outcome runner.StageOutcome
	if len(queue) > 0 {
		outcome = queue[0]
		g.outcomes[stage.Name] = queue[1:]
	}
	g.mu.Unlock()
	if outcome.Stage.Name == "" {
		outcome.Stage = stage
	}
	if outcome.Status == "" {
		outcome.Status = runner.StageStatusCompleted
	}
    return outcome, nil
}

func (g *fakeRuntime) setOutcomes(stage string, outcomes []runner.StageOutcome) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.outcomes == nil {
		g.outcomes = make(map[string][]runner.StageOutcome)
	}
	dup := make([]runner.StageOutcome, len(outcomes))
	copy(dup, outcomes)
	g.outcomes[stage] = dup
}

func (g *fakeRuntime) callsSnapshot() []runtimeCall {
	g.mu.Lock()
	defer g.mu.Unlock()
    dup := make([]runtimeCall, len(g.calls))
	copy(dup, g.calls)
    return dup
}

func (g *fakeRuntime) CancelWorkflow(ctx context.Context, req runner.CancelRequest) (runner.CancelResult, error) {
	_ = ctx
	_ = req
    return runner.CancelResult{}, runner.ErrCancellationUnsupported
}

func gatherStageAttempts(calls []runtimeCall, stage string) int {
	count := 0
	for _, call := range calls {
		if call.stage.Name == stage {
			count++
		}
	}
	return count
}

func findStageCall(calls []runtimeCall, stageName string) runtimeCall {
	for _, call := range calls {
		if call.stage.Name == stageName {
			return call
		}
	}
    return runtimeCall{}
}

type failingPlanner struct {
	err error
}

func (f failingPlanner) Build(ctx context.Context, ticket contracts.WorkflowTicket) (runner.ExecutionPlan, error) {
	_ = ctx
	_ = ticket
	return runner.ExecutionPlan{}, f.err
}

type invalidStagePlanner struct{}

func (invalidStagePlanner) Build(ctx context.Context, ticket contracts.WorkflowTicket) (runner.ExecutionPlan, error) {
	_ = ctx
	return runner.ExecutionPlan{
		TicketID: ticket.TicketID,
		Stages: []runner.Stage{
			{Name: "", Kind: runner.StageKindModsPlan, Lane: "node-wasm"},
		},
	}, nil
}

type missingLanePlanner struct{}

func (missingLanePlanner) Build(ctx context.Context, ticket contracts.WorkflowTicket) (runner.ExecutionPlan, error) {
	_ = ctx
	return runner.ExecutionPlan{
		TicketID: ticket.TicketID,
		Stages: []runner.Stage{
			{Name: modsPlanStage, Kind: runner.StageKindModsPlan, Lane: ""},
		},
	}, nil
}

func withCleanupDeadline(t *testing.T) {
	t.Helper()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		<-ctx.Done()
	})
}

type failingCompiler struct {
	err error
}

func (f failingCompiler) Compile(ctx context.Context, ref contracts.ManifestReference) (manifests.Compilation, error) {
	_ = ctx
	_ = ref
	return manifests.Compilation{}, f.err
}

type recordingCompiler struct {
	compiled manifests.Compilation
	ref      contracts.ManifestReference
}

func (r *recordingCompiler) Compile(ctx context.Context, ref contracts.ManifestReference) (manifests.Compilation, error) {
	_ = ctx
	r.ref = ref
	return r.compiled, nil
}

type stubAsterLocator struct {
	bundles map[string]aster.Metadata
}

func (s *stubAsterLocator) Locate(ctx context.Context, req aster.Request) (aster.Metadata, error) {
	_ = ctx
	key := fmt.Sprintf("%s/%s", strings.ToLower(strings.TrimSpace(req.Stage)), strings.ToLower(strings.TrimSpace(req.Toggle)))
	if meta, ok := s.bundles[key]; ok {
		return meta, nil
	}
	return aster.Metadata{}, aster.ErrBundleNotFound
}

type metadataPlanner struct {
	plan runner.ExecutionPlan
}

func (m metadataPlanner) Build(ctx context.Context, ticket contracts.WorkflowTicket) (runner.ExecutionPlan, error) {
	_ = ctx
	_ = ticket
	return m.plan, nil
}

type stageGate struct {
	ch   chan struct{}
	once sync.Once
}

type parallelRecordingGrid struct {
	mu            sync.Mutex
	outcomes      map[string][]runner.StageOutcome
	gates         map[string]*stageGate
	startCh       chan string
    calls         []runtimeCall
	pendingStarts map[string]int
}

func newParallelRecordingGrid() *parallelRecordingGrid {
	return &parallelRecordingGrid{
		outcomes:      make(map[string][]runner.StageOutcome),
		gates:         make(map[string]*stageGate),
		startCh:       make(chan string, 64),
		pendingStarts: make(map[string]int),
	}
}

func (g *parallelRecordingGrid) setOutcomes(stage string, outcomes []runner.StageOutcome) {
	g.mu.Lock()
	defer g.mu.Unlock()
	clone := make([]runner.StageOutcome, len(outcomes))
	copy(clone, outcomes)
	g.outcomes[stage] = clone
}

func (g *parallelRecordingGrid) addGate(stage string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if _, ok := g.gates[stage]; ok {
		return
	}
	g.gates[stage] = &stageGate{ch: make(chan struct{})}
}

func (g *parallelRecordingGrid) allow(stage string) {
	g.mu.Lock()
	gate := g.gates[stage]
	g.mu.Unlock()
	if gate == nil {
		return
	}
	gate.once.Do(func() {
		close(gate.ch)
	})
}

func (g *parallelRecordingGrid) waitForStart(stage string, timeout time.Duration) bool {
	deadline := time.After(timeout)
	for {
		g.mu.Lock()
		if count := g.pendingStarts[stage]; count > 0 {
			g.pendingStarts[stage] = count - 1
			g.mu.Unlock()
			return true
		}
		g.mu.Unlock()
		select {
		case name := <-g.startCh:
			if name == stage {
				return true
			}
			g.mu.Lock()
			g.pendingStarts[name]++
			g.mu.Unlock()
		case <-deadline:
			return false
		}
	}
}

func (g *parallelRecordingGrid) callsSnapshot() []runtimeCall {
	g.mu.Lock()
	defer g.mu.Unlock()
    clone := make([]runtimeCall, len(g.calls))
	copy(clone, g.calls)
	return clone
}

func (g *parallelRecordingGrid) ExecuteStage(ctx context.Context, ticket contracts.WorkflowTicket, stage runner.Stage, workspace string) (runner.StageOutcome, error) {
	_ = ticket
	g.mu.Lock()
    g.calls = append(g.calls, runtimeCall{stage: stage, workspace: workspace})
	gate := g.gates[stage.Name]
	g.mu.Unlock()

	select {
	case g.startCh <- stage.Name:
	case <-ctx.Done():
		return runner.StageOutcome{}, ctx.Err()
	}

	if gate != nil {
		select {
		case <-gate.ch:
		case <-ctx.Done():
			return runner.StageOutcome{}, ctx.Err()
		}
	}

	var outcome runner.StageOutcome
	g.mu.Lock()
	if queue := g.outcomes[stage.Name]; len(queue) > 0 {
		outcome = queue[0]
		g.outcomes[stage.Name] = queue[1:]
	}
	g.mu.Unlock()

	if outcome.Stage.Name == "" {
		outcome.Stage = stage
	}
	if outcome.Status == "" {
		outcome.Status = runner.StageStatusCompleted
	}
	return outcome, nil
}

func (g *parallelRecordingGrid) CancelWorkflow(ctx context.Context, req runner.CancelRequest) (runner.CancelResult, error) {
	_ = ctx
	_ = req
    return runner.CancelResult{}, runner.ErrCancellationUnsupported
}
