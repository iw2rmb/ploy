package runner

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/iw2rmb/ploy/internal/workflow/contracts"
	"github.com/iw2rmb/ploy/internal/workflow/manifests"
	"github.com/iw2rmb/ploy/internal/workflow/mods"
)

type stageNode struct {
	Stage      Stage
	Remaining  int
	Dependents []string
}

type stageResult struct {
	node     *stageNode
	executed Stage
	outcome  StageOutcome
	err      error
}

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

	if opts.WorkspacePreparer != nil {
		if err := opts.WorkspacePreparer.Prepare(ctx, WorkspacePrepareRequest{Ticket: ticket, Path: workspace}); err != nil {
			return fmt.Errorf("prepare workspace: %w", err)
		}
	}

	maxRetries := opts.MaxStageRetries
	if maxRetries < 0 {
		maxRetries = 0
	}

	if len(plan.Stages) == 0 {
		if err := publishCheckpoint(ctx, opts.Events, ticket.TicketID, "workflow", StageStatusCompleted, "", nil, nil); err != nil {
			return err
		}
		return nil
	}

	stageOrder := make(map[string]int, len(plan.Stages))
	stageNodes := make(map[string]*stageNode, len(plan.Stages))
	orderedNodes := make([]*stageNode, 0, len(plan.Stages))
	for idx, rawStage := range plan.Stages {
		stage := rawStage
		stage.Name = strings.TrimSpace(stage.Name)
		if stage.Name == "" {
			return fmt.Errorf("%w: stage name is required", ErrCheckpointValidationFailed)
		}
		if _, exists := stageNodes[stage.Name]; exists {
			return fmt.Errorf("%w: duplicate stage %s in plan", ErrCheckpointValidationFailed, stage.Name)
		}
		stageOrder[stage.Name] = idx
		stage.Dependencies = copyStringSlice(stage.Dependencies)
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
		node := &stageNode{Stage: stage, Remaining: len(stage.Dependencies)}
		stageNodes[stage.Name] = node
		orderedNodes = append(orderedNodes, node)
	}
	for _, node := range stageNodes {
		for _, dep := range node.Stage.Dependencies {
			depNode, ok := stageNodes[dep]
			if !ok {
				return fmt.Errorf("%w: stage %s depends on unknown stage %s", ErrCheckpointValidationFailed, node.Stage.Name, dep)
			}
			depNode.Dependents = append(depNode.Dependents, node.Stage.Name)
		}
	}

	initialReady := make([]*stageNode, 0, len(orderedNodes))
	for _, node := range orderedNodes {
		if node.Remaining == 0 {
			initialReady = append(initialReady, node)
		}
	}
	if len(orderedNodes) > 0 && len(initialReady) == 0 {
		return fmt.Errorf("%w: workflow plan has no root stages", ErrCheckpointValidationFailed)
	}

	stageCtx, cancelStages := context.WithCancel(ctx)
	defer cancelStages()

	var (
		wg              sync.WaitGroup
		active          int
		completed       int
		totalStages     = len(orderedNodes)
		runErr          error
		publishMu       sync.Mutex
		failureOnce     sync.Once
		healingAttempts int
	)
	resultCh := make(chan stageResult, len(orderedNodes))
	executor := stageExecutor{
		events:      opts.Events,
		grid:        opts.Grid,
		ticket:      ticket,
		workspace:   workspace,
		maxRetries:  maxRetries,
		publishMu:   &publishMu,
		failureOnce: &failureOnce,
		jobComposer: opts.JobComposer,
	}
	scheduled := make(map[string]bool, len(stageNodes))
	startStages := func(nodes []*stageNode) {
		if runErr != nil || len(nodes) == 0 {
			return
		}
		sort.Slice(nodes, func(i, j int) bool {
			return stageOrder[nodes[i].Stage.Name] < stageOrder[nodes[j].Stage.Name]
		})
		for _, node := range nodes {
			if scheduled[node.Stage.Name] {
				continue
			}
			scheduled[node.Stage.Name] = true
			active++
			wg.Add(1)
			go func(n *stageNode) {
				defer wg.Done()
				executed, outcome, err := executor.runStage(stageCtx, n.Stage)
				resultCh <- stageResult{node: n, executed: executed, outcome: outcome, err: err}
			}(node)
		}
	}

	startStages(initialReady)
	for completed < totalStages {
		res := <-resultCh
		if res.err != nil {
			handled, added, skipped, healErr := handleHealing(ctx, res, &healingAttempts, stageNodes, stageOrder, &orderedNodes, scheduled, ticket, compiledManifest, opts, composer, startStages)
			if handled && healErr == nil {
				totalStages += added
				totalStages -= skipped
			} else {
				if healErr != nil {
					runErr = healErr
				} else if runErr == nil {
					runErr = res.err
				}
				cancelStages()
			}
		} else {
			completed++
			res.node.Stage = res.executed
			newlyReady := make([]*stageNode, 0, len(res.node.Dependents))
			for _, depName := range res.node.Dependents {
				depNode := stageNodes[depName]
				depNode.Remaining--
				if depNode.Remaining == 0 {
					newlyReady = append(newlyReady, depNode)
				}
			}
			startStages(newlyReady)
		}
		active--
		if active == 0 {
			if runErr != nil {
				break
			}
			if completed == totalStages {
				break
			}
			runErr = fmt.Errorf("%w: workflow plan has unresolved stage dependencies", ErrCheckpointValidationFailed)
			break
		}
	}

	wg.Wait()
	if runErr != nil {
		return runErr
	}
	if completed != totalStages {
		return fmt.Errorf("%w: workflow plan incomplete", ErrCheckpointValidationFailed)
	}

	if err := executor.publishWorkflowCompletion(ctx); err != nil {
		return err
	}

	return nil
}

func handleHealing(ctx context.Context, res stageResult, attempts *int, stageNodes map[string]*stageNode, stageOrder map[string]int, orderedNodes *[]*stageNode, scheduled map[string]bool, ticket contracts.WorkflowTicket, compiled manifests.Compilation, opts Options, composer CacheComposer, start func([]*stageNode)) (bool, int, int, error) {
	if !shouldScheduleHealing(res, *attempts, opts.Mods) {
		return false, 0, 0, nil
	}
	attempt := *attempts + 1
	skippedNames := collectDependentStages(res.node, stageNodes)
	for _, name := range skippedNames {
		scheduled[name] = true
	}
	ready, added, err := appendHealingPlan(ctx, ticket, compiled, opts, composer, stageNodes, stageOrder, orderedNodes, attempt, res.outcome)
	if err != nil {
		return true, 0, 0, err
	}
	*attempts = attempt
	if len(ready) > 0 {
		start(ready)
	}
	return true, added, len(skippedNames), nil
}

func shouldScheduleHealing(res stageResult, attempts int, modsOpts ModsOptions) bool {
	if res.outcome.Stage.Kind != StageKindBuildGate {
		return false
	}
	if !res.outcome.Retryable {
		return false
	}
	if attempts >= 1 {
		return false
	}
	if strings.TrimSpace(modsOpts.PlanLane) == "" {
		return false
	}
	return true
}

func appendHealingPlan(ctx context.Context, ticket contracts.WorkflowTicket, compiled manifests.Compilation, opts Options, composer CacheComposer, stageNodes map[string]*stageNode, stageOrder map[string]int, orderedNodes *[]*stageNode, attempt int, outcome StageOutcome) ([]*stageNode, int, error) {
	plannerOpts := mods.Options{
		PlanLane:        opts.Mods.PlanLane,
		OpenRewriteLane: opts.Mods.OpenRewriteLane,
		LLMPlanLane:     opts.Mods.LLMPlanLane,
		LLMExecLane:     opts.Mods.LLMExecLane,
		HumanLane:       opts.Mods.HumanLane,
		PlanTimeout:     opts.Mods.PlanTimeout,
		MaxParallel:     opts.Mods.MaxParallel,
		Advisor:         opts.Mods.Advisor,
	}
	modPlanner := mods.NewPlanner(plannerOpts)
	signals := buildHealingSignals(outcome)
	modStages, err := modPlanner.Plan(ctx, mods.PlanInput{Ticket: ticket, Signals: signals})
	if err != nil {
		return nil, 0, err
	}
	suffix := fmt.Sprintf("#heal%d", attempt)
	ready := make([]*stageNode, 0, len(modStages))
	added := 0
	for _, modStage := range modStages {
		stage := Stage{
			Name:         modStage.Name + suffix,
			Kind:         StageKind(modStage.Kind),
			Lane:         strings.TrimSpace(modStage.Lane),
			Dependencies: renameDependencies(modStage.Dependencies, suffix),
			Metadata:     convertStageMetadata(modStage.Metadata),
		}
		if stage.Lane == "" {
			stage.Lane = modStage.Lane
		}
		stage.Constraints.Manifest = compiled
		asterMeta, err := resolveStageAster(ctx, stage, compiled, opts.Aster)
		if err != nil {
			return nil, 0, err
		}
		stage.Aster = asterMeta
		cacheKey, err := composer.Compose(ctx, CacheComposeRequest{Stage: stage, Ticket: ticket})
		if err != nil {
			return nil, 0, err
		}
		stage.CacheKey = strings.TrimSpace(cacheKey)
		node := &stageNode{Stage: stage, Remaining: len(stage.Dependencies)}
		stageNodes[stage.Name] = node
		stageOrder[stage.Name] = len(*orderedNodes)
		*orderedNodes = append(*orderedNodes, node)
		if node.Remaining == 0 {
			ready = append(ready, node)
		}
		for _, dep := range stage.Dependencies {
			if depNode, ok := stageNodes[dep]; ok {
				depNode.Dependents = append(depNode.Dependents, stage.Name)
			}
		}
		added++
	}

	appendLinearStage := func(name string, kind StageKind, lane string, deps []string) error {
		stage := Stage{
			Name:         name,
			Kind:         kind,
			Lane:         lane,
			Dependencies: deps,
		}
		stage.Constraints.Manifest = compiled
		asterMeta, err := resolveStageAster(ctx, stage, compiled, opts.Aster)
		if err != nil {
			return err
		}
		stage.Aster = asterMeta
		cacheKey, err := composer.Compose(ctx, CacheComposeRequest{Stage: stage, Ticket: ticket})
		if err != nil {
			return err
		}
		stage.CacheKey = strings.TrimSpace(cacheKey)
		node := &stageNode{Stage: stage, Remaining: len(deps)}
		stageNodes[stage.Name] = node
		stageOrder[stage.Name] = len(*orderedNodes)
		*orderedNodes = append(*orderedNodes, node)
		if node.Remaining == 0 {
			ready = append(ready, node)
		}
		for _, dep := range deps {
			if depNode, ok := stageNodes[dep]; ok {
				depNode.Dependents = append(depNode.Dependents, stage.Name)
			}
		}
		added++
		return nil
	}

	if err := appendLinearStage(buildGateStageName+suffix, StageKindBuildGate, "go-native", []string{mods.StageNameHuman + suffix}); err != nil {
		return nil, 0, err
	}
	if err := appendLinearStage(staticChecksStageName+suffix, StageKindStaticChecks, "go-native", []string{buildGateStageName + suffix}); err != nil {
		return nil, 0, err
	}
	if err := appendLinearStage("test"+suffix, StageKindTest, "go-native", []string{staticChecksStageName + suffix}); err != nil {
		return nil, 0, err
	}

	return ready, added, nil
}

func renameDependencies(deps []string, suffix string) []string {
	if len(deps) == 0 {
		return nil
	}
	renamed := make([]string, len(deps))
	for i, dep := range deps {
		renamed[i] = strings.TrimSpace(dep) + suffix
	}
	return renamed
}

func buildHealingSignals(outcome StageOutcome) mods.AdviceSignals {
	signals := mods.AdviceSignals{}
	if msg := strings.TrimSpace(outcome.Message); msg != "" {
		signals.Errors = append(signals.Errors, msg)
	}
	if outcome.Stage.Metadata.BuildGate != nil {
		for _, finding := range outcome.Stage.Metadata.BuildGate.LogFindings {
			if trimmed := strings.TrimSpace(finding.Message); trimmed != "" {
				signals.Errors = append(signals.Errors, trimmed)
			}
		}
		for _, report := range outcome.Stage.Metadata.BuildGate.StaticChecks {
			for _, failure := range report.Failures {
				if trimmed := strings.TrimSpace(failure.Message); trimmed != "" {
					signals.Errors = append(signals.Errors, trimmed)
				}
			}
		}
	}
	return signals
}

func collectDependentStages(node *stageNode, stageNodes map[string]*stageNode) []string {
	if node == nil {
		return nil
	}
	seen := make(map[string]struct{})
	var visit func(name string)
	visit = func(name string) {
		if _, ok := seen[name]; ok {
			return
		}
		seen[name] = struct{}{}
		n := stageNodes[name]
		if n == nil {
			return
		}
		for _, dep := range n.Dependents {
			visit(dep)
		}
	}
	visit(node.Stage.Name)
	result := make([]string, 0, len(seen))
	for name := range seen {
		result = append(result, name)
	}
	return result
}
