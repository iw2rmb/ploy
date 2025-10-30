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

// Run executes the workflow pipeline for the claimed ticket end-to-end.
func Run(ctx context.Context, opts Options) (err error) {
	if opts.Events == nil {
		return ErrEventsClientRequired
	}
    if opts.Runtime == nil {
        return ErrRuntimeClientRequired
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
        runtime:     opts.Runtime,
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
