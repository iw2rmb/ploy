package mods

import (
    "context"
    "archive/tar"
    "bytes"
    "encoding/json"
    "encoding/base64"
    "fmt"
    "io"
    "net/http"
    "net/url"
    "path/filepath"
    "strings"
    "sync"
    "time"

    clientv3 "go.etcd.io/etcd/client/v3"
    gitlabcfg "github.com/iw2rmb/ploy/internal/config/gitlab"
    modplan "github.com/iw2rmb/ploy/internal/mods/plan"
    "github.com/iw2rmb/ploy/internal/workflow/contracts"
    "os"
    artifacts "github.com/iw2rmb/ploy/internal/workflow/artifacts"
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
        Image: dockerImageRef("mods-plan"),
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
// dockerImageRef builds the fully-qualified image reference for a Mods image name
// using Docker Hub by default. It mirrors the precedence used by runner templates:
//  1) DOCKERHUB_USERNAME -> docker.io/$DOCKERHUB_USERNAME
//  2) MODS_IMAGE_PREFIX  -> absolute prefix (e.g., docker.io/org or ghcr.io/org)
//  3) fallback           -> docker.io/iw2rmb
func dockerImageRef(name string) string {
    ns := strings.TrimSpace(os.Getenv("DOCKERHUB_USERNAME"))
    if ns != "" {
        return "docker.io/" + ns + "/" + name + ":latest"
    }
    if p := strings.TrimSpace(os.Getenv("MODS_IMAGE_PREFIX")); p != "" {
        return p + "/" + name + ":latest"
    }
    return "docker.io/iw2rmb/" + name + ":latest"
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
        if err := s.store.updateTicketState(ctx, ticketID, TicketStateSucceeded); err != nil {
            return err
        }
        // Best-effort MR publication; log/return errors to caller.
        if err := s.publishMergeRequest(ctx, ticketID, status); err != nil {
            return err
        }
        return nil
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

// publishMergeRequest creates a GitLab branch + MR using the diff bundle from orw-apply, when configured.
func (s *Service) publishMergeRequest(ctx context.Context, ticketID string, status *TicketStatus) error {
    if status == nil {
        st, err := s.store.ticketStatus(ctx, ticketID)
        if err != nil { return err }
        status = st
    }
    repoURL := strings.TrimSpace(status.Repository)
    if repoURL == "" {
        return nil
    }
    baseRef := strings.TrimSpace(status.Metadata["repo_base_ref"])
    targetRef := strings.TrimSpace(status.Metadata["repo_target_ref"])
    if baseRef == "" || targetRef == "" {
        return nil
    }
    // Locate diff artifact from ORW apply stage.
    orw := strings.TrimSpace(modplan.StageNameORWApply)
    st, ok := status.Stages[orw]
    if !ok || strings.ToLower(string(st.State)) != string(StageStateSucceeded) {
        // No ORW stage or not successful; skip MR.
        return nil
    }
    diffCID := strings.TrimSpace(st.Artifacts["diff_cid"])
    if diffCID == "" {
        return nil
    }
    // Load GitLab config
    kv := gitlabcfg.NewEtcdKV(s.store.client)
    cfgStore := gitlabcfg.NewStore(kv)
    cfg, _, err := cfgStore.Load(ctx)
    if err != nil {
        return fmt.Errorf("mods: load gitlab config: %w", err)
    }
    if strings.TrimSpace(cfg.APIBaseURL) == "" || strings.TrimSpace(cfg.DefaultToken.Value) == "" {
        return nil
    }
    apiBase := strings.TrimSuffix(strings.TrimSpace(cfg.APIBaseURL), "/")
    parsedRepo, err := url.Parse(repoURL)
    if err != nil { return nil }
    if !strings.EqualFold(parsedRepo.Hostname(), strings.TrimPrefix(strings.TrimPrefix(strings.TrimPrefix(strings.TrimSpace(cfg.APIBaseURL), "https://"), "http://"), "/")) {
        // Repo host does not match configured API base; skip.
        // This simplistic check ensures we only act on repos under the configured GitLab.
        // It is best-effort to avoid publishing to different providers.
        return nil
    }
    projectPath := strings.TrimSuffix(strings.TrimPrefix(parsedRepo.Path, "/"), ".git")
    if projectPath == "" { return nil }
    projectID := url.PathEscape(projectPath)

    // Fetch diff tar from IPFS Cluster
    artClient, err := newClusterClientFromEnv()
    if err != nil { return fmt.Errorf("mods: artifacts client: %w", err) }
    art, err := artClient.Fetch(ctx, diffCID)
    if err != nil { return fmt.Errorf("mods: fetch diff artifact %s: %w", diffCID, err) }
    files, err := untarToMemory(bytes.NewReader(art.Data))
    if err != nil { return fmt.Errorf("mods: decode diff tar: %w", err) }
    if len(files) == 0 {
        return nil
    }
    gl := gitlabClient{base: apiBase, token: strings.TrimSpace(cfg.DefaultToken.Value), http: &http.Client{Timeout: 30 * time.Second}}
    // Ensure branch exists (via commits API with start_branch) and commit changes.
    actions := make([]gitlabCommitAction, 0, len(files))
    for path, content := range files {
        // Use base64 for safety (binary-safe)
        actions = append(actions, gitlabCommitAction{Action: "create", FilePath: path, Content: base64Encode(content), Encoding: "base64"})
    }
    // First try commit with start_branch to create branch; on 400 (branch exists), try without start_branch with updates.
    msg := fmt.Sprintf("Ploy Mods: apply OpenRewrite + upgrades (ticket %s)", ticketID)
    if err := gl.commit(ctx, projectID, targetRef, baseRef, msg, actions); err != nil {
        // Try falling back to update actions when files exist
        for i := range actions { actions[i].Action = "update" }
        if err2 := gl.commit(ctx, projectID, targetRef, "", msg, actions); err2 != nil {
            return fmt.Errorf("mods: gitlab commit: %v (retry: %v)", err, err2)
        }
    }
    // Create MR (idempotent-ish: if already exists, ignore)
    title := fmt.Sprintf("Ploy Mods: Upgrade Java 11→17 (%s)", ticketID)
    desc := "Automated Mods run applying OpenRewrite and validations."
    _ = gl.createMR(ctx, projectID, targetRef, baseRef, title, desc)
    return nil
}

// newClusterClientFromEnv wires an IPFS Cluster client from environment.
func newClusterClientFromEnv() (*artifacts.ClusterClient, error) {
    base := strings.TrimSpace(os.Getenv("PLOY_IPFS_CLUSTER_API"))
    if base == "" {
        return nil, fmt.Errorf("PLOY_IPFS_CLUSTER_API not set")
    }
    return artifacts.NewClusterClient(artifacts.ClusterClientOptions{
        BaseURL:           base,
        AuthToken:         strings.TrimSpace(os.Getenv("PLOY_IPFS_CLUSTER_TOKEN")),
        BasicAuthUsername: strings.TrimSpace(os.Getenv("PLOY_IPFS_CLUSTER_USERNAME")),
        BasicAuthPassword: strings.TrimSpace(os.Getenv("PLOY_IPFS_CLUSTER_PASSWORD")),
    })
}

// untarToMemory returns a map of relative file paths to content.
func untarToMemory(r io.Reader) (map[string][]byte, error) {
    files := make(map[string][]byte)
    tr := tar.NewReader(r)
    for {
        hdr, err := tr.Next()
        if err == io.EOF { break }
        if err != nil { return nil, err }
        name := filepath.Clean(hdr.Name)
        if name == "." || strings.HasSuffix(name, "/") { continue }
        if hdr.FileInfo().Mode().IsDir() || (hdr.FileInfo().Mode()&os.ModeSymlink) != 0 { continue }
        data, err := io.ReadAll(tr)
        if err != nil { return nil, err }
        files[name] = data
    }
    return files, nil
}

func base64Encode(b []byte) string { return base64.StdEncoding.EncodeToString(b) }

// gitlabClient is a minimal GitLab REST client.
type gitlabClient struct {
    base  string
    token string
    http  *http.Client
}

type gitlabCommitAction struct {
    Action   string `json:"action"`
    FilePath string `json:"file_path"`
    Content  string `json:"content"`
    Encoding string `json:"encoding,omitempty"`
}

func (g gitlabClient) commit(ctx context.Context, project, branch, startBranch, message string, actions []gitlabCommitAction) error {
    if g.base == "" || g.token == "" { return fmt.Errorf("gitlab: client not configured") }
    endpoint := fmt.Sprintf("%s/api/v4/projects/%s/repository/commits", strings.TrimSuffix(g.base, "/"), project)
    payload := map[string]any{"branch": branch, "commit_message": message, "actions": actions}
    if strings.TrimSpace(startBranch) != "" { payload["start_branch"] = startBranch }
    body, _ := json.Marshal(payload)
    req, _ := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
    req.Header.Set("Content-Type", "application/json")
    req.Header.Set("PRIVATE-TOKEN", g.token)
    resp, err := g.http.Do(req)
    if err != nil { return err }
    defer resp.Body.Close()
    if resp.StatusCode >= 300 {
        data, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
        return fmt.Errorf("gitlab: commit %s", strings.TrimSpace(string(data)))
    }
    return nil
}

func (g gitlabClient) createMR(ctx context.Context, project, source, target, title, description string) error {
    endpoint := fmt.Sprintf("%s/api/v4/projects/%s/merge_requests", strings.TrimSuffix(g.base, "/"), project)
    payload := map[string]any{"source_branch": source, "target_branch": target, "title": title, "description": description}
    body, _ := json.Marshal(payload)
    req, _ := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
    req.Header.Set("Content-Type", "application/json")
    req.Header.Set("PRIVATE-TOKEN", g.token)
    resp, err := g.http.Do(req)
    if err != nil { return err }
    defer resp.Body.Close()
    if resp.StatusCode >= 300 {
        // Best-effort: ignore errors (e.g., MR exists or permissions) to avoid failing the ticket.
        return nil
    }
    return nil
}
