package mods

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"
	modplan "github.com/iw2rmb/ploy/internal/mods/plan"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

// StageJobSubmitter defines scheduler interactions required by the orchestrator.
type StageJobSubmitter interface {
	SubmitStageJob(ctx context.Context, spec StageJobSpec) (StageJob, error)
}

// Options configures the Mods orchestrator service.
type Options struct {
    Prefix     string
    Scheduler  StageJobSubmitter
    Clock      func() time.Time
    JobWatcher JobCompletionWatcher
}

// Hydration removed: stage manifests are submitted as-is.

const (
	manifestMetadataKey   = "step_manifest"
	metadataRepoURLKey    = "hydration_repo_url"
	metadataRevisionKey   = "hydration_revision"
	metadataInputNameKey  = "hydration_input_name"
	defaultHydrationInput = "workspace"
)

// Service orchestrates Mods ticket submission and lifecycle transitions.
type Service struct {
    store     *store
    scheduler StageJobSubmitter
    clock     func() time.Time
    watcher   JobCompletionWatcher

    ctx    context.Context
    cancel context.CancelFunc
    wg     sync.WaitGroup
}

// NewService constructs a Mods orchestrator backed by etcd.
func NewService(client *clientv3.Client, opts Options) (*Service, error) {
	if client == nil {
		return nil, fmt.Errorf("mods: etcd client is required")
	}
	if opts.Scheduler == nil {
		return nil, fmt.Errorf("mods: scheduler is required")
	}
	cfg := applyServiceDefaults(opts)
	ctx, cancel := context.WithCancel(context.Background())
	service := &Service{
		store:     newStore(client, cfg.Prefix, cfg.Clock),
		scheduler: cfg.Scheduler,
		clock:     cfg.Clock,
		watcher:   cfg.JobWatcher,
		ctx:       ctx,
		cancel:    cancel,
    }
	service.startWatchers()
	return service, nil
}

// Submit registers a new Mods ticket and enqueues root stages.
func (s *Service) Submit(ctx context.Context, spec TicketSpec) (*TicketStatus, error) {
	if err := s.validateSpec(spec); err != nil {
		return nil, err
	}
	graph, err := buildStageGraph(spec.Stages)
	if err != nil {
		return nil, err
	}
	status, err := s.store.createTicket(ctx, spec, graph)
	if err != nil {
		return nil, err
	}
	for _, stageID := range graph.roots() {
		stageDef := graph.stages[stageID]
		stageStatus := status.Stages[stageID]
		queued, err := s.enqueueStage(ctx, spec.TicketID, stageDef, stageStatus)
		if err != nil {
			return nil, err
		}
		status.Stages[stageID] = *queued
	}
	return status, nil
}

// ClaimStage attempts to claim a queued stage for execution.
func (s *Service) ClaimStage(ctx context.Context, req ClaimStageRequest) (*StageStatus, error) {
	if req.TicketID == "" || req.StageID == "" || req.JobID == "" {
		return nil, fmt.Errorf("mods: claim requires ticket, stage, and job id")
	}
	status, err := s.store.claimStage(ctx, req.TicketID, req)
	if err != nil {
		return nil, err
	}
	_ = s.store.updateTicketState(ctx, req.TicketID, TicketStateRunning)
	return status, nil
}

// ProcessJobCompletion reconciles job completion events with ticket state.
func (s *Service) ProcessJobCompletion(ctx context.Context, completion JobCompletion) error {
	if completion.TicketID == "" || completion.StageID == "" {
		return fmt.Errorf("mods: completion requires ticket and stage")
	}
	stage, err := s.store.stageStatus(ctx, completion.TicketID, completion.StageID)
	if err != nil {
		return err
	}
	if stage.CurrentJobID != "" && completion.JobID != "" && stage.CurrentJobID != completion.JobID {
		// Ignore stale completion for superseded job attempt.
		return nil
	}

	graph, err := s.store.readGraph(ctx, completion.TicketID)
	if err != nil {
		return err
	}

	switch completion.State {
	case JobCompletionSucceeded:
		if _, err := s.store.completeStageSuccess(ctx, completion.TicketID, completion); err != nil {
			return err
		}
		return s.afterStageSuccess(ctx, completion.TicketID, completion.StageID, graph)
	case JobCompletionFailed, JobCompletionCancelled:
		return s.handleStageFailure(ctx, completion, stage, graph)
	default:
		return fmt.Errorf("mods: unsupported completion state %q", completion.State)
	}
}

// TicketStatus fetches the current status for a ticket.
func (s *Service) TicketStatus(ctx context.Context, ticketID string) (*TicketStatus, error) {
	return s.store.ticketStatus(ctx, ticketID)
}

// StageStatus fetches the current status for a specific stage.
func (s *Service) StageStatus(ctx context.Context, ticketID, stageID string) (*StageStatus, error) {
	return s.store.stageStatus(ctx, ticketID, stageID)
}

// Cancel transitions the ticket into cancelling state and stops pending stages.
func (s *Service) Cancel(ctx context.Context, ticketID string) error {
	stages, err := s.store.listStages(ctx, ticketID)
	if err != nil {
		return err
	}
	for id, entry := range stages {
		if entry.doc.State == StageStateSucceeded || entry.doc.State == StageStateFailed {
			continue
		}
		entry.doc.State = StageStateCancelled
		entry.doc.CurrentJobID = ""
		if _, err := s.store.writeStage(ctx, ticketID, entry.doc, entry.revision); err != nil {
			return err
		}
		stages[id] = entry
	}
	if err := s.store.updateTicketState(ctx, ticketID, TicketStateCancelled); err != nil {
		return err
	}
	return nil
}

// Resume restarts a cancelled ticket by requeueing eligible stages.
func (s *Service) Resume(ctx context.Context, ticketID string) (*TicketStatus, error) {
	graph, err := s.store.readGraph(ctx, ticketID)
	if err != nil {
		return nil, err
	}
	stages, err := s.store.listStages(ctx, ticketID)
	if err != nil {
		return nil, err
	}
	status, err := s.store.ticketStatus(ctx, ticketID)
	if err != nil {
		return nil, err
	}
	for id, entry := range stages {
		switch entry.doc.State {
		case StageStateCancelled, StageStateFailed:
			if entry.doc.Attempts >= entry.doc.MaxAttempts {
				continue
			}
			entry.doc.State = StageStatePending
			entry.doc.CurrentJobID = ""
			entry.doc.LastError = ""
			updated, err := s.store.writeStage(ctx, ticketID, entry.doc, entry.revision)
			if err != nil {
				return nil, err
			}
			status.Stages[id] = *updated
		}
	}
	if err := s.enqueueReadyStages(ctx, ticketID, graph, status.Stages); err != nil {
		return nil, err
	}
	if err := s.store.updateTicketState(ctx, ticketID, TicketStatePending); err != nil {
		return nil, err
	}
	return s.store.ticketStatus(ctx, ticketID)
}

// Close stops background orchestrator loops.
func (s *Service) Close() error {
	s.cancel()
	s.wg.Wait()
	return nil
}

// validateSpec ensures required submission fields are populated.
func (s *Service) validateSpec(spec TicketSpec) error {
	if spec.TicketID == "" {
		return fmt.Errorf("mods: ticket id is required")
	}
	if len(spec.Stages) == 0 {
		return fmt.Errorf("mods: stage graph is required")
	}
	return nil
}

// enqueueStage submits a stage to the scheduler and marks it queued.
func (s *Service) enqueueStage(ctx context.Context, ticketID string, def StageDefinition, current StageStatus) (*StageStatus, error) {
	// First, synthesize a manifest for the Mods plan stage when missing.
	if updated, err := s.synthesizePlanManifest(ctx, ticketID, def); err == nil && updated != nil {
		def = *updated
	} else if err != nil {
		return nil, err
	}
    // Hydration reuse removed; manifests are used without snapshot injection.
	spec := StageJobSpec{
		JobID:        current.CurrentJobID,
		TicketID:     ticketID,
		StageID:      def.ID,
		Priority:     def.Priority,
		MaxAttempts:  current.MaxAttempts,
		RetryAttempt: current.Attempts,
		Metadata:     cloneMap(def.Metadata),
	}
	job, err := s.scheduler.SubmitStageJob(ctx, spec)
	if err != nil {
		return nil, fmt.Errorf("submit stage job: %w", err)
	}
	return s.store.markStageQueued(ctx, ticketID, def.ID, job.JobID)
}

// synthesizePlanManifest attaches a default step manifest for the plan stage when none is provided.
func (s *Service) synthesizePlanManifest(ctx context.Context, ticketID string, def StageDefinition) (*StageDefinition, error) {
    // Only synthesize for the plan stage (by id) or when lane explicitly indicates mods-plan.
    if strings.TrimSpace(def.ID) != modplan.StageNamePlan && strings.TrimSpace(def.Lane) != modplan.StageNamePlan {
        return nil, nil
    }
    if def.Metadata != nil {
        if raw := strings.TrimSpace(def.Metadata[manifestMetadataKey]); raw != "" {
            // Manifest already present; nothing to do.
            return nil, nil
        }
    }

    // Fetch ticket meta to source repository and hints.
    meta, _, err := s.store.readMeta(ctx, ticketID)
    if err != nil {
        return nil, fmt.Errorf("mods: read ticket meta: %w", err)
    }

    repoURL := strings.TrimSpace(meta.Repository)
    // Optional hints from ticket metadata
    baseRef := strings.TrimSpace(meta.Metadata["repo_base_ref"])
    targetRef := strings.TrimSpace(meta.Metadata["repo_target_ref"])
    commit := strings.TrimSpace(meta.Metadata["repo_commit"])
    workspaceHint := strings.TrimSpace(meta.Metadata["repo_workspace_hint"])

    // Build a minimal, but valid manifest for mods-plan.
    // If no repository (or an invalid/opaque ref) is available, skip synthesis and let the client supply a manifest.
    if repoURL == "" || (!strings.Contains(repoURL, "://") && !strings.Contains(repoURL, "@")) {
        return nil, nil
    }

    manifest := contracts.StepManifest{
        ID:    modplan.StageNamePlan,
        Name:  "Mods Plan",
        Image: "ghcr.io/ploy/mods/plan:latest",
        Command: []string{"mods-plan"},
        Args:    []string{"--run"},
        Env:     map[string]string{"MODS_PLAN_CACHE": "/workspace/cache"},
        Inputs: []contracts.StepInput{{
            Name:      defaultHydrationInput,
            MountPath: "/" + defaultHydrationInput,
            Mode:      contracts.StepInputModeReadWrite,
            Hydration: &contracts.StepInputHydration{Repo: &contracts.RepoMaterialization{
                URL:           repoURL,
                BaseRef:       baseRef,
                TargetRef:     targetRef,
                Commit:        commit,
                WorkspaceHint: workspaceHint,
            }},
        }},
        Resources: contracts.StepResourceSpec{CPU: "2000m", Memory: "4Gi"},
        // Keep retention modest by default; nodes may override via lane/catalog later.
        Retention: contracts.StepRetentionSpec{RetainContainer: false, TTL: "72h"},
    }

    if err := manifest.Validate(); err != nil {
        return nil, fmt.Errorf("mods: synthesized plan manifest invalid: %w", err)
    }
    payload, err := json.Marshal(manifest)
    if err != nil {
        return nil, fmt.Errorf("mods: encode synthesized manifest: %w", err)
    }
    if def.Metadata == nil {
        def.Metadata = map[string]string{}
    }
    def.Metadata[manifestMetadataKey] = string(payload)
    // Hydration reuse removed; keep minimal metadata only if useful for downstream tools.
    def.Metadata[metadataRepoURLKey] = repoURL
    // Prefer commit; fall back to target ref if commit is not available.
    revision := commit
    if revision == "" {
        revision = targetRef
    }
    if revision != "" {
        def.Metadata[metadataRevisionKey] = revision
    }
    def.Metadata[metadataInputNameKey] = defaultHydrationInput
    return &def, nil
}
// prepareStageHydration removed.

// afterStageSuccess enqueues dependents and updates ticket state post-success.
func (s *Service) afterStageSuccess(ctx context.Context, ticketID, stageID string, graph *stageGraph) error {
	status, err := s.store.ticketStatus(ctx, ticketID)
	if err != nil {
		return err
	}
	updated := status.Stages[stageID]
	updated.State = StageStateSucceeded
	status.Stages[stageID] = updated
	if err := s.enqueueDependents(ctx, ticketID, graph, status.Stages, stageID); err != nil {
		return err
	}
	if allStagesSucceeded(status.Stages) {
		return s.store.updateTicketState(ctx, ticketID, TicketStateSucceeded)
	}
	return s.store.updateTicketState(ctx, ticketID, TicketStateRunning)
}

// enqueueDependents queues dependent stages whose prerequisites are satisfied.
func (s *Service) enqueueDependents(ctx context.Context, ticketID string, graph *stageGraph, stages map[string]StageStatus, stageID string) error {
	for _, dependent := range graph.dependents(stageID) {
		state, ok := stages[dependent]
		if !ok {
			continue
		}
		if state.State != StageStatePending {
			continue
		}
		if !dependenciesSatisfied(graph, stages, dependent) {
			continue
		}
		def := graph.stages[dependent]
		queued, err := s.enqueueStage(ctx, ticketID, def, state)
		if err != nil {
			return err
		}
		stages[dependent] = *queued
	}
	return nil
}

// enqueueReadyStages walks all pending stages and queues those whose dependencies are complete.
func (s *Service) enqueueReadyStages(ctx context.Context, ticketID string, graph *stageGraph, stages map[string]StageStatus) error {
	for id, state := range stages {
		if state.State != StageStatePending {
			continue
		}
		if !dependenciesSatisfied(graph, stages, id) {
			continue
		}
		def := graph.stages[id]
		queued, err := s.enqueueStage(ctx, ticketID, def, state)
		if err != nil {
			return err
		}
		stages[id] = *queued
	}
	return nil
}

// handleStageFailure evaluates retries and marks terminal failure when exhausted.
func (s *Service) handleStageFailure(ctx context.Context, completion JobCompletion, stage *StageStatus, graph *stageGraph) error {
	if stage.Attempts < stage.MaxAttempts {
		requeued, err := s.store.requeueStageFailure(ctx, completion.TicketID, completion)
		if err != nil {
			return err
		}
		def := graph.stages[completion.StageID]
		if def.MaxAttempts <= 0 {
			def.MaxAttempts = stage.MaxAttempts
		}
		if _, err := s.enqueueStage(ctx, completion.TicketID, def, *requeued); err != nil {
			return err
		}
		return s.store.updateTicketState(ctx, completion.TicketID, TicketStateRunning)
	}
	if _, err := s.store.completeStageFailure(ctx, completion.TicketID, completion); err != nil {
		return err
	}
	return s.store.updateTicketState(ctx, completion.TicketID, TicketStateFailed)
}

func dependenciesSatisfied(graph *stageGraph, stages map[string]StageStatus, stageID string) bool {
	// dependenciesSatisfied verifies all upstream dependencies are in succeeded state.
	for _, dep := range graph.dependencies(stageID) {
		state, ok := stages[dep]
		if !ok {
			return false
		}
		if state.State != StageStateSucceeded {
			return false
		}
	}
	return true
}

func allStagesSucceeded(stages map[string]StageStatus) bool {
	// allStagesSucceeded reports whether every stage is complete and successful.
	if len(stages) == 0 {
		return false
	}
	for _, stage := range stages {
		if stage.State != StageStateSucceeded {
			return false
		}
	}
	return true
}

func applyServiceDefaults(opts Options) Options {
	// applyServiceDefaults normalises service options with sensible defaults.
	if opts.Clock == nil {
		opts.Clock = func() time.Time { return time.Now().UTC() }
	}
	if opts.Prefix == "" {
		opts.Prefix = "mods"
	}
	if opts.Prefix[len(opts.Prefix)-1] != '/' {
		opts.Prefix += "/"
	}
	return opts
}
