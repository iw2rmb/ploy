package runner_test

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/iw2rmb/ploy/internal/workflow/aster"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
	"github.com/iw2rmb/ploy/internal/workflow/manifests"
	"github.com/iw2rmb/ploy/internal/workflow/mods"
	"github.com/iw2rmb/ploy/internal/workflow/runner"
)

const modsPlanStage = mods.StageNamePlan

func defaultManifestCompilation() manifests.Compilation {
	return manifests.Compilation{
		Manifest: manifests.Metadata{Name: "smoke", Version: "2025-09-26"},
		Lanes: manifests.LaneSet{
			Required: []manifests.Lane{{Name: "node-wasm"}, {Name: "go-native"}},
			Allowed:  []manifests.Lane{{Name: "gpu-ml"}},
		},
	}
}

func newStubCompiler() *recordingCompiler {
	return &recordingCompiler{compiled: defaultManifestCompilation()}
}

func setRunnerModsOptions(t *testing.T, opts *runner.Options, planTimeout time.Duration, maxParallel int) {
	t.Helper()
	value := reflect.ValueOf(opts).Elem()
	modsField := value.FieldByName("Mods")
	if !modsField.IsValid() {
		t.Fatalf("runner.Options missing Mods field: %#v", opts)
	}
	if modsField.Kind() != reflect.Struct {
		t.Fatalf("runner.Options Mods field is not a struct: %s", modsField.Kind())
	}
	planTimeoutField := modsField.FieldByName("PlanTimeout")
	if !planTimeoutField.IsValid() {
		t.Fatalf("runner.ModsOptions missing PlanTimeout: %#v", modsField.Interface())
	}
	if !planTimeoutField.CanSet() {
		t.Fatalf("runner.ModsOptions PlanTimeout not settable")
	}
	if planTimeoutField.Type() != reflect.TypeOf(time.Duration(0)) {
		t.Fatalf("runner.ModsOptions PlanTimeout has unexpected type: %s", planTimeoutField.Type())
	}
	planTimeoutField.Set(reflect.ValueOf(planTimeout))

	maxParallelField := modsField.FieldByName("MaxParallel")
	if !maxParallelField.IsValid() {
		t.Fatalf("runner.ModsOptions missing MaxParallel: %#v", modsField.Interface())
	}
	if !maxParallelField.CanSet() {
		t.Fatalf("runner.ModsOptions MaxParallel not settable")
	}
	if maxParallelField.Kind() != reflect.Int {
		t.Fatalf("runner.ModsOptions MaxParallel has unexpected kind: %s", maxParallelField.Kind())
	}
	maxParallelField.SetInt(int64(maxParallel))
}

type errorGrid struct {
	err error
}

func (g errorGrid) ExecuteStage(ctx context.Context, ticket contracts.WorkflowTicket, stage runner.Stage, workspace string) (runner.StageOutcome, error) {
	_ = ctx
	_ = ticket
	_ = stage
	_ = workspace
	return runner.StageOutcome{}, g.err
}

type recordingCacheComposer struct {
	calls []runner.CacheComposeRequest
}

func (r *recordingCacheComposer) Compose(ctx context.Context, req runner.CacheComposeRequest) (string, error) {
	_ = ctx
	r.calls = append(r.calls, req)
	return fmt.Sprintf("cache-%s", strings.ToLower(req.Stage.Name)), nil
}

type errorEvents struct {
	ticket      contracts.WorkflowTicket
	claimErr    error
	publishErr  error
	artifactErr error
}

func (e *errorEvents) ClaimTicket(ctx context.Context, ticketID string) (contracts.WorkflowTicket, error) {
	_ = ctx
	if e.claimErr != nil {
		return contracts.WorkflowTicket{}, e.claimErr
	}
	if e.ticket.TicketID == "" {
		e.ticket.TicketID = ticketID
	}
	if e.ticket.SchemaVersion == "" {
		e.ticket.SchemaVersion = contracts.SchemaVersion
	}
	if e.ticket.Tenant == "" {
		e.ticket.Tenant = "acme"
	}
	if e.ticket.Manifest.Name == "" || e.ticket.Manifest.Version == "" {
		e.ticket.Manifest = contracts.ManifestReference{Name: "smoke", Version: "2025-09-26"}
	}
	if strings.TrimSpace(e.ticket.TicketID) == "" {
		e.ticket.TicketID = "ticket-auto"
	}
	return e.ticket, nil
}

func (e *errorEvents) PublishCheckpoint(ctx context.Context, checkpoint contracts.WorkflowCheckpoint) error {
	_ = ctx
	if e.publishErr != nil {
		return e.publishErr
	}
	return nil
}

func (e *errorEvents) PublishArtifact(ctx context.Context, artifact contracts.WorkflowArtifact) error {
	_ = ctx
	if e.artifactErr != nil {
		return e.artifactErr
	}
	return nil
}

type statuslessGrid struct{}

func (statuslessGrid) ExecuteStage(ctx context.Context, ticket contracts.WorkflowTicket, stage runner.Stage, workspace string) (runner.StageOutcome, error) {
	_ = ctx
	_ = ticket
	_ = workspace
	return runner.StageOutcome{Stage: stage}, nil
}

type noStageGrid struct{}

func (noStageGrid) ExecuteStage(ctx context.Context, ticket contracts.WorkflowTicket, stage runner.Stage, workspace string) (runner.StageOutcome, error) {
	_ = ctx
	_ = ticket
	_ = workspace
	return runner.StageOutcome{Status: runner.StageStatusCompleted}, nil
}

type countingEvents struct {
	ticket       contracts.WorkflowTicket
	failAt       int
	publishCount int
	err          error
}

func (c *countingEvents) ClaimTicket(ctx context.Context, ticketID string) (contracts.WorkflowTicket, error) {
	_ = ctx
	if c.ticket.SchemaVersion == "" {
		c.ticket.SchemaVersion = contracts.SchemaVersion
	}
	if c.ticket.TicketID == "" {
		c.ticket.TicketID = ticketID
	}
	if c.ticket.Tenant == "" {
		c.ticket.Tenant = "acme"
	}
	if c.ticket.Manifest.Name == "" || c.ticket.Manifest.Version == "" {
		c.ticket.Manifest = contracts.ManifestReference{Name: "smoke", Version: "2025-09-26"}
	}
	return c.ticket, nil
}

func (c *countingEvents) PublishCheckpoint(ctx context.Context, checkpoint contracts.WorkflowCheckpoint) error {
	_ = ctx
	c.publishCount++
	if c.failAt > 0 && c.publishCount == c.failAt {
		if c.err == nil {
			c.err = errors.New("publish checkpoint failure")
		}
		return c.err
	}
	return nil
}

func (c *countingEvents) PublishArtifact(ctx context.Context, artifact contracts.WorkflowArtifact) error {
	_ = ctx
	return nil
}

type stageStatusEntry struct {
	stage  string
	status runner.StageStatus
}

func extractStageStatuses(checkpoints []contracts.WorkflowCheckpoint) []stageStatusEntry {
	result := make([]stageStatusEntry, 0, len(checkpoints))
	for _, cp := range checkpoints {
		result = append(result, stageStatusEntry{stage: cp.Stage, status: runner.StageStatus(cp.Status)})
	}
	return result
}

func compareSequences(actual, expected []stageStatusEntry) error {
	if len(actual) != len(expected) {
		return fmt.Errorf("length mismatch: got %d want %d", len(actual), len(expected))
	}
	for i := range actual {
		a := actual[i]
		e := expected[i]
		if a.stage != e.stage || a.status != e.status {
			return fmt.Errorf("entry %d mismatch: got %s/%s want %s/%s", i, a.stage, a.status, e.stage, e.status)
		}
	}
	return nil
}

func collectStageStatuses(sequence []stageStatusEntry, stage string) []runner.StageStatus {
	statuses := make([]runner.StageStatus, 0, len(sequence))
	for _, entry := range sequence {
		if entry.stage == stage {
			statuses = append(statuses, entry.status)
		}
	}
	return statuses
}

func requireStageStatuses(t *testing.T, sequence []stageStatusEntry, stage string, expected []runner.StageStatus) {
	t.Helper()
	statuses := collectStageStatuses(sequence, stage)
	if len(statuses) != len(expected) {
		t.Fatalf("stage %s statuses length mismatch: got %d want %d", stage, len(statuses), len(expected))
	}
	for i, status := range statuses {
		if status != expected[i] {
			t.Fatalf("stage %s status %d mismatch: got %s want %s", stage, i, status, expected[i])
		}
	}
}

type recordingEvents struct {
	tenant         string
	nextTicket     string
	invalidTicket  bool
	manifest       contracts.ManifestReference
	claimedTickets []string
	checkpoints    []contracts.WorkflowCheckpoint
	artifacts      []contracts.WorkflowArtifact
	mu             sync.Mutex
}

func (r *recordingEvents) ClaimTicket(ctx context.Context, ticketID string) (contracts.WorkflowTicket, error) {
	_ = ctx
	r.mu.Lock()
	defer r.mu.Unlock()
	if ticketID == "" {
		ticketID = r.nextTicket
	}
	r.claimedTickets = append(r.claimedTickets, ticketID)
	if r.invalidTicket {
		return contracts.WorkflowTicket{}, nil
	}
	ref := r.manifest
	if ref.Name == "" && ref.Version == "" {
		ref = contracts.ManifestReference{Name: "smoke", Version: "2025-09-26"}
	}
	return contracts.WorkflowTicket{
		SchemaVersion: contracts.SchemaVersion,
		TicketID:      ticketID,
		Tenant:        r.tenant,
		Manifest:      ref,
	}, nil
}

func (r *recordingEvents) PublishCheckpoint(ctx context.Context, checkpoint contracts.WorkflowCheckpoint) error {
	_ = ctx
	r.mu.Lock()
	defer r.mu.Unlock()
	r.checkpoints = append(r.checkpoints, checkpoint)
	return nil
}

func (r *recordingEvents) PublishArtifact(ctx context.Context, artifact contracts.WorkflowArtifact) error {
	_ = ctx
	r.mu.Lock()
	defer r.mu.Unlock()
	r.artifacts = append(r.artifacts, artifact)
	return nil
}

type gridCall struct {
	stage     runner.Stage
	workspace string
}

type fakeGrid struct {
	mu            sync.Mutex
	outcomes      map[string][]runner.StageOutcome
	calls         []gridCall
	lastWorkspace string
}

func (g *fakeGrid) ExecuteStage(ctx context.Context, ticket contracts.WorkflowTicket, stage runner.Stage, workspace string) (runner.StageOutcome, error) {
	_ = ctx
	_ = ticket
	g.mu.Lock()
	g.calls = append(g.calls, gridCall{stage: stage, workspace: workspace})
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

func gatherStageAttempts(calls []gridCall, stage string) int {
	count := 0
	for _, call := range calls {
		if call.stage.Name == stage {
			count++
		}
	}
	return count
}

func findStageCall(calls []gridCall, stageName string) gridCall {
	for _, call := range calls {
		if call.stage.Name == stageName {
			return call
		}
	}
	return gridCall{}
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
	mu       sync.Mutex
	outcomes map[string][]runner.StageOutcome
	gates    map[string]*stageGate
	startCh  chan string
	calls    []gridCall
}

func newParallelRecordingGrid() *parallelRecordingGrid {
	return &parallelRecordingGrid{
		outcomes: make(map[string][]runner.StageOutcome),
		gates:    make(map[string]*stageGate),
		startCh:  make(chan string, 64),
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
		select {
		case name := <-g.startCh:
			if name == stage {
				return true
			}
		case <-deadline:
			return false
		}
	}
}

func (g *parallelRecordingGrid) callsSnapshot() []gridCall {
	g.mu.Lock()
	defer g.mu.Unlock()
	clone := make([]gridCall, len(g.calls))
	copy(clone, g.calls)
	return clone
}

func (g *parallelRecordingGrid) ExecuteStage(ctx context.Context, ticket contracts.WorkflowTicket, stage runner.Stage, workspace string) (runner.StageOutcome, error) {
	_ = ticket
	g.mu.Lock()
	g.calls = append(g.calls, gridCall{stage: stage, workspace: workspace})
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
