package nodeagent

import (
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/iw2rmb/ploy/internal/node/worker/hydration"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
	"github.com/iw2rmb/ploy/internal/workflow/runtime/step"
)

// Agent coordinates the node agent's HTTP server and heartbeat manager.
type Agent struct {
	cfg        Config
	server     *Server
	heartbeat  *HeartbeatManager
	controller *runController
}

// New constructs a new node agent.
func New(cfg Config) (*Agent, error) {
	controller := &runController{
		cfg:  cfg,
		runs: make(map[string]*runContext),
	}

	server, err := NewServer(cfg, controller)
	if err != nil {
		return nil, fmt.Errorf("create server: %w", err)
	}

	heartbeat, err := NewHeartbeatManager(cfg)
	if err != nil {
		return nil, fmt.Errorf("create heartbeat manager: %w", err)
	}

	return &Agent{
		cfg:        cfg,
		server:     server,
		heartbeat:  heartbeat,
		controller: controller,
	}, nil
}

// Run starts the node agent and blocks until the context is canceled.
func (a *Agent) Run(ctx context.Context) error {
	if err := a.server.Start(ctx); err != nil {
		return fmt.Errorf("start server: %w", err)
	}
	slog.Info("node http server listening", "addr", a.server.Address())

	var wg sync.WaitGroup
	errCh := make(chan error, 1)

	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := a.heartbeat.Start(ctx); err != nil && !errors.Is(err, context.Canceled) {
			select {
			case errCh <- fmt.Errorf("heartbeat: %w", err):
			default:
			}
		}
	}()

	<-ctx.Done()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := a.server.Stop(shutdownCtx); err != nil {
		return fmt.Errorf("stop server: %w", err)
	}

	wg.Wait()

	select {
	case err := <-errCh:
		return err
	default:
		return nil
	}
}

// runController implements the RunController interface for managing runs.
type runController struct {
	mu   sync.Mutex
	cfg  Config
	runs map[string]*runContext
}

type runContext struct {
	runID  string
	cancel context.CancelFunc
}

// StartRun accepts a run start request and initiates execution.
func (r *runController) StartRun(ctx context.Context, req StartRunRequest) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.runs[req.RunID]; exists {
		return fmt.Errorf("run %s already exists", req.RunID)
	}

	// Create a cancellable context for this run, derived from caller.
	runCtx, cancel := context.WithCancel(ctx)
	r.runs[req.RunID] = &runContext{
		runID:  req.RunID,
		cancel: cancel,
	}

	// In the skeleton, we just accept the run without executing it.
	// Actual execution will be implemented in subsequent tasks.
	go r.executeRun(runCtx, req)

	return nil
}

// StopRun cancels a running job.
func (r *runController) StopRun(ctx context.Context, req StopRunRequest) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	run, exists := r.runs[req.RunID]
	if !exists {
		return fmt.Errorf("run %s not found", req.RunID)
	}

	run.cancel()
	delete(r.runs, req.RunID)

	return nil
}

func (r *runController) executeRun(ctx context.Context, req StartRunRequest) {
	defer func() {
		r.mu.Lock()
		delete(r.runs, req.RunID)
		r.mu.Unlock()
	}()

	slog.Info("starting run execution", "run_id", req.RunID, "repo_url", req.RepoURL)

	// Convert the StartRunRequest to a StepManifest.
	manifest, err := buildManifestFromRequest(req)
	if err != nil {
		slog.Error("failed to build manifest", "run_id", req.RunID, "error", err)
		return
	}

	// Create ephemeral workspace directory (honors PLOYD_CACHE_HOME when set).
	workspaceRoot, err := createWorkspaceDir()
	if err != nil {
		slog.Error("failed to create workspace", "run_id", req.RunID, "error", err)
		return
	}
	defer func() {
		_ = os.RemoveAll(workspaceRoot)
	}()

	// Initialize runtime components.
	artifactPublisher, err := step.NewFilesystemArtifactPublisher(step.FilesystemArtifactPublisherOptions{})
	if err != nil {
		slog.Error("failed to create artifact publisher", "run_id", req.RunID, "error", err)
		return
	}

	gitFetcher, err := hydration.NewGitFetcher(hydration.GitFetcherOptions{
		Publisher:       artifactPublisher,
		PublishSnapshot: false,
	})
	if err != nil {
		slog.Error("failed to create git fetcher", "run_id", req.RunID, "error", err)
		return
	}

	workspaceHydrator, err := step.NewFilesystemWorkspaceHydrator(step.FilesystemWorkspaceHydratorOptions{
		RepoFetcher: gitFetcher,
	})
	if err != nil {
		slog.Error("failed to create workspace hydrator", "run_id", req.RunID, "error", err)
		return
	}

	containerRuntime, err := step.NewDockerContainerRuntime(step.DockerContainerRuntimeOptions{
		PullImage: true,
	})
	if err != nil {
		slog.Error("failed to create container runtime", "run_id", req.RunID, "error", err)
		return
	}

	diffGenerator := step.NewFilesystemDiffGenerator(step.FilesystemDiffGeneratorOptions{})

	gateExecutor := step.NewGateExecutor()

	// Create the step runner with all components.
	runner := step.Runner{
		Workspace:  workspaceHydrator,
		Containers: containerRuntime,
		Diffs:      diffGenerator,
		Artifacts:  newSizeLimitedPublisher(artifactPublisher, maxArtifactSize),
		Gate:       gateExecutor,
	}

	// Execute the step.
	startTime := time.Now()
	result, err := runner.Run(ctx, step.Request{
		Manifest:  manifest,
		Workspace: workspaceRoot,
	})
	duration := time.Since(startTime)

	if err != nil {
		slog.Error("run execution failed",
			"run_id", req.RunID,
			"error", err,
			"duration", duration,
			"exit_code", result.ExitCode,
		)
		return
	}

	slog.Info("run execution completed",
		"run_id", req.RunID,
		"duration", duration,
		"exit_code", result.ExitCode,
		"diff_cid", result.DiffArtifact.CID,
		"log_cid", result.LogArtifact.CID,
	)
}

// buildManifestFromRequest converts a StartRunRequest into a StepManifest.
func buildManifestFromRequest(req StartRunRequest) (contracts.StepManifest, error) {
	if strings.TrimSpace(req.RunID) == "" {
		return contracts.StepManifest{}, errors.New("run_id required")
	}
	if strings.TrimSpace(req.RepoURL) == "" {
		return contracts.StepManifest{}, errors.New("repo_url required")
	}

	// Default to a basic build container if not specified in options.
	image := "ubuntu:latest"
	command := []string{"/bin/sh", "-c", "echo 'Build gate placeholder'"}
	if imgOpt, ok := req.Options["image"].(string); ok && strings.TrimSpace(imgOpt) != "" {
		image = strings.TrimSpace(imgOpt)
	}
	// Accept command as []string or single shell string.
	switch v := req.Options["command"].(type) {
	case []string:
		if len(v) > 0 {
			command = v
		}
	case string:
		if s := strings.TrimSpace(v); s != "" {
			command = []string{"/bin/sh", "-c", s}
		}
	}

	// Determine the ref to clone.
	targetRef := strings.TrimSpace(req.TargetRef)
	if targetRef == "" && strings.TrimSpace(req.BaseRef) != "" {
		targetRef = strings.TrimSpace(req.BaseRef)
	}

	// Build the repo materialization.
	repo := contracts.RepoMaterialization{
		URL:       req.RepoURL,
		BaseRef:   req.BaseRef,
		TargetRef: targetRef,
		Commit:    req.CommitSHA,
	}

	// Create a single read-write input that will be hydrated from the repository.
	// Defensive copy of env to avoid aliasing caller map.
	env := make(map[string]string, len(req.Env))
	for k, v := range req.Env {
		env[k] = v
	}

	manifest := contracts.StepManifest{
		ID:         req.RunID,
		Name:       fmt.Sprintf("Run %s", req.RunID),
		Image:      image,
		Command:    command,
		WorkingDir: "/workspace",
		Env:        env,
		Inputs: []contracts.StepInput{
			{
				Name:      "workspace",
				MountPath: "/workspace",
				Mode:      contracts.StepInputModeReadWrite,
				Hydration: &contracts.StepInputHydration{
					Repo: &repo,
				},
			},
		},
		Retention: contracts.StepRetentionSpec{
			RetainContainer: false,
			TTL:             "1h",
		},
	}

	return manifest, nil
}

const maxArtifactSize = 1 << 20 // 1 MiB

// sizeLimitedPublisher wraps an artifact publisher and enforces size caps on gzipped output.
type sizeLimitedPublisher struct {
	delegate step.ArtifactPublisher
	maxSize  int64
}

func newSizeLimitedPublisher(delegate step.ArtifactPublisher, maxSize int64) *sizeLimitedPublisher {
	return &sizeLimitedPublisher{
		delegate: delegate,
		maxSize:  maxSize,
	}
}

func (p *sizeLimitedPublisher) Publish(ctx context.Context, req step.ArtifactRequest) (step.PublishedArtifact, error) {
	// Compress and measure the artifact.
	var buf bytes.Buffer
	gzWriter := gzip.NewWriter(&buf)

	var reader io.Reader
	if strings.TrimSpace(req.Path) != "" {
		file, err := os.Open(req.Path)
		if err != nil {
			return step.PublishedArtifact{}, fmt.Errorf("open artifact: %w", err)
		}
		defer func() { _ = file.Close() }()
		reader = file
	} else if len(req.Buffer) > 0 {
		reader = bytes.NewReader(req.Buffer)
	} else {
		return step.PublishedArtifact{}, errors.New("artifact payload required")
	}

	if _, err := io.Copy(gzWriter, reader); err != nil {
		return step.PublishedArtifact{}, fmt.Errorf("compress artifact: %w", err)
	}
	if err := gzWriter.Close(); err != nil {
		return step.PublishedArtifact{}, fmt.Errorf("finalize compression: %w", err)
	}

	compressed := buf.Bytes()
	if int64(len(compressed)) > p.maxSize {
		return step.PublishedArtifact{}, fmt.Errorf("artifact exceeds size limit: %d > %d bytes (gzipped)", len(compressed), p.maxSize)
	}

	// Publish the compressed artifact.
	compressedReq := step.ArtifactRequest{
		Kind:   req.Kind,
		Buffer: compressed,
	}
	return p.delegate.Publish(ctx, compressedReq)
}
