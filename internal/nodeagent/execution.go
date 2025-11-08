package nodeagent

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"net/url"

	types "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/nodeagent/git"
	"github.com/iw2rmb/ploy/internal/nodeagent/gitlab"
	"github.com/iw2rmb/ploy/internal/worker/hydration"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
	"github.com/iw2rmb/ploy/internal/workflow/runtime/step"
)

func (r *runController) executeRun(ctx context.Context, req StartRunRequest) {
	defer func() {
		r.mu.Lock()
		delete(r.runs, req.RunID.String())
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

	gitFetcher, err := hydration.NewGitFetcher(hydration.GitFetcherOptions{PublishSnapshot: false})
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
		// Fallback: proceed without a container runtime (simulated execution)
		slog.Warn("docker unavailable; falling back to stub runtime", "run_id", req.RunID, "error", err)
		containerRuntime = nil
	}

	diffGenerator := step.NewFilesystemDiffGenerator(step.FilesystemDiffGeneratorOptions{})

	gateExecutor := step.NewDockerGateExecutor(containerRuntime)

	// Create log streamer to stream logs as gzipped chunks to the server.
	logStreamer := NewLogStreamer(r.cfg, req.RunID.String(), "")
	defer func() {
		if closeErr := logStreamer.Close(); closeErr != nil {
			slog.Warn("failed to close log streamer", "run_id", req.RunID, "error", closeErr)
		}
	}()

	// Create the step runner with all components.
	runner := step.Runner{
		Workspace:  workspaceHydrator,
		Containers: containerRuntime,
		Diffs:      diffGenerator,
		Gate:       gateExecutor,
		LogWriter:  logStreamer,
	}

	// Prepare /out directory for the container to write additional artifacts.
	outDir, err := os.MkdirTemp("", "ploy-mod-out-*")
	if err != nil {
		slog.Error("failed to create /out directory", "run_id", req.RunID, "error", err)
		return
	}
	defer func() { _ = os.RemoveAll(outDir) }()

	// Prepare /in directory for cross-phase inputs (created only when needed).
	var inDir string
	defer func() {
		if inDir != "" {
			_ = os.RemoveAll(inDir)
		}
	}()

	// Execute the step with possible healing loop.
	startTime := time.Now()
	execResult, execErr := r.executeWithHealing(ctx, runner, req, manifest, workspaceRoot, outDir, &inDir)
	duration := time.Since(startTime)
	result := execResult.Result

	if execErr != nil {
		slog.Error("run execution failed",
			"run_id", req.RunID,
			"error", execErr,
			"duration", duration,
			"exit_code", result.ExitCode,
		)
		// Continue to emit terminal status even on failure.
	}

	// Generate and upload diff to server if diff generator is available.
	if diffGenerator != nil {
		diffBytes, err := diffGenerator.Generate(ctx, workspaceRoot)
		if err != nil {
			slog.Error("failed to generate diff", "run_id", req.RunID, "error", err)
		} else if len(diffBytes) > 0 {
			// Create diff uploader and upload the diff.
			diffUploader, err := NewDiffUploader(r.cfg)
			if err != nil {
				slog.Error("failed to create diff uploader", "run_id", req.RunID, "error", err)
			} else {
				// Build a summary with basic metadata.
				summary := map[string]interface{}{
					"exit_code": result.ExitCode,
					"timings": map[string]interface{}{
						"hydration_duration_ms":  result.Timings.HydrationDuration.Milliseconds(),
						"execution_duration_ms":  result.Timings.ExecutionDuration.Milliseconds(),
						"build_gate_duration_ms": result.Timings.BuildGateDuration.Milliseconds(),
						"diff_duration_ms":       result.Timings.DiffDuration.Milliseconds(),
						"total_duration_ms":      result.Timings.TotalDuration.Milliseconds(),
					},
				}

				// Use stage_id provided by control plane (no backward-compat fallback).
				stageID, _ := req.Options["stage_id"].(string)

				// Upload the diff to the server.
				if err := diffUploader.UploadDiff(ctx, req.RunID.String(), stageID, diffBytes, summary); err != nil {
					slog.Error("failed to upload diff", "run_id", req.RunID, "error", err)
				} else {
					slog.Info("diff uploaded successfully", "run_id", req.RunID, "size", len(diffBytes))
				}

				// Also upload the diff as an artifact bundle named "diff" for client download.
				diffFile, err := os.CreateTemp("", "ploy-diff-*.patch")
				if err == nil {
					_, _ = diffFile.Write(diffBytes)
					_ = diffFile.Close()
					if artUploader, err2 := NewArtifactUploader(r.cfg); err2 == nil {
						if _, _, errU := artUploader.UploadArtifact(ctx, req.RunID.String(), stageID, []string{diffFile.Name()}, "diff"); errU != nil {
							slog.Warn("failed to upload diff artifact bundle", "run_id", req.RunID, "error", errU)
						} else {
							slog.Info("diff artifact bundle uploaded", "run_id", req.RunID)
						}
					}
					_ = os.Remove(diffFile.Name())
				}
			}
		}
	}

	// Upload artifact bundles where configured.
	// Check options for artifact_paths configuration.
	if artifactPaths, ok := req.Options["artifact_paths"].([]interface{}); ok && len(artifactPaths) > 0 {
		// Convert to string slice.
		var paths []string
		for _, p := range artifactPaths {
			if pathStr, ok := p.(string); ok && pathStr != "" {
				// Resolve path relative to workspace.
				fullPath := filepath.Join(workspaceRoot, pathStr)
				// Check if path exists before adding.
				if _, err := os.Stat(fullPath); err == nil {
					paths = append(paths, fullPath)
				} else {
					slog.Warn("artifact path not found", "run_id", req.RunID, "path", pathStr)
				}
			}
		}

		if len(paths) > 0 {
			// Create artifact uploader and upload the bundle.
			artifactUploader, err := NewArtifactUploader(r.cfg)
			if err != nil {
				slog.Error("failed to create artifact uploader", "run_id", req.RunID, "error", err)
			} else {
				// Use stage_id provided by control plane.
				stageID, _ := req.Options["stage_id"].(string)

				// Optional: get artifact name from options.
				artifactName := ""
				if name, ok := req.Options["artifact_name"].(string); ok {
					artifactName = name
				}

				// Upload the artifact bundle to the server.
				if _, _, err := artifactUploader.UploadArtifact(ctx, req.RunID.String(), stageID, paths, artifactName); err != nil {
					slog.Error("failed to upload artifact bundle", "run_id", req.RunID, "error", err)
				} else {
					slog.Info("artifact bundle uploaded successfully", "run_id", req.RunID, "paths", len(paths))
				}
			}
		}
	}

	// Always attempt to bundle and upload /out regardless of artifact_paths.
	if err := uploadOutDirIfPresent(ctx, r.cfg, req.RunID.String(), stageIDFromOptions(req.Options), outDir); err != nil {
		slog.Error("/out artifact upload failed", "run_id", req.RunID, "error", err)
	}

	// Emit terminal status to server.
	statusUploader, err := NewStatusUploader(r.cfg)
	if err != nil {
		slog.Error("failed to create status uploader", "run_id", req.RunID, "error", err)
	} else {
		// Determine terminal status based on execution result.
		terminalStatus := "succeeded"
		var reason *string
		if execErr != nil {
			terminalStatus = "failed"
			errMsg := execErr.Error()
			// Check if this is a build gate failure.
			if errors.Is(execErr, step.ErrBuildGateFailed) {
				// Set reason to "build-gate" for pre-mod gate failures.
				gateReason := "build-gate"
				reason = &gateReason
			} else {
				reason = &errMsg
			}
		} else if result.ExitCode != 0 {
			terminalStatus = "failed"
			failureMsg := fmt.Sprintf("exit code %d", result.ExitCode)
			reason = &failureMsg
		}

		// Phase E: Create MR via GitLab API when conditions are met.
		// Hook runs after terminal status is determined but before uploading status.
		mrURL := ""
		if shouldCreateMR(terminalStatus, manifest.Options) {
			if url, mrErr := r.createMR(ctx, req, manifest, workspaceRoot); mrErr != nil {
				slog.Error("failed to create MR", "run_id", req.RunID, "error", mrErr)
			} else {
				mrURL = url
				slog.Info("MR created successfully", "run_id", req.RunID, "mr_url", mrURL)
			}
		}

		// Build stats with execution metrics.
		stats := map[string]interface{}{
			"exit_code":   result.ExitCode,
			"duration_ms": duration.Milliseconds(),
			"timings": map[string]interface{}{
				"hydration_duration_ms":  result.Timings.HydrationDuration.Milliseconds(),
				"execution_duration_ms":  result.Timings.ExecutionDuration.Milliseconds(),
				"build_gate_duration_ms": result.Timings.BuildGateDuration.Milliseconds(),
				"diff_duration_ms":       result.Timings.DiffDuration.Milliseconds(),
				"total_duration_ms":      result.Timings.TotalDuration.Milliseconds(),
			},
		}

		// Attach MR URL to metadata if created.
		if mrURL != "" {
			stats["metadata"] = map[string]interface{}{
				"mr_url": mrURL,
			}
		}

		// Gate stats/logs: collect pass/fail, duration, resources, and upload logs artifact.
		// Include pre-gate and re-gate runs when healing was attempted.
		if execResult.PreGate != nil || len(execResult.ReGates) > 0 || result.BuildGate != nil {
			gate := map[string]any{}

			// Build gate metadata helper function.
			buildGateStats := func(meta *contracts.BuildGateStageMetadata, durationMs int64, artifactNameSuffix string) map[string]any {
				gateStats := map[string]any{
					"duration_ms": durationMs,
				}
				// Determine pass/fail.
				passed := false
				if meta != nil && len(meta.StaticChecks) > 0 {
					passed = meta.StaticChecks[0].Passed
				}
				gateStats["passed"] = passed
				if meta != nil && meta.Resources != nil {
					ru := meta.Resources
					gateStats["resources"] = map[string]any{
						"limits": map[string]any{"nano_cpus": ru.LimitNanoCPUs, "memory_bytes": ru.LimitMemoryBytes},
						"usage":  map[string]any{"cpu_total_ns": ru.CPUTotalNs, "mem_usage_bytes": ru.MemUsageBytes, "mem_max_bytes": ru.MemMaxBytes, "blkio_read_bytes": ru.BlkioReadBytes, "blkio_write_bytes": ru.BlkioWriteBytes, "size_rw_bytes": ru.SizeRwBytes},
					}
				}
				// Upload build logs as artifact when present.
				if meta != nil {
					if s := strings.TrimSpace(meta.LogsText); s != "" {
						logFile, err := os.CreateTemp("", "ploy-gate-*.log")
						if err == nil {
							_, _ = logFile.WriteString(s)
							_ = logFile.Close()
							if artUploader, err2 := NewArtifactUploader(r.cfg); err2 == nil {
								stageID, _ := req.Options["stage_id"].(string)
								artifactName := "build-gate.log"
								if artifactNameSuffix != "" {
									artifactName = "build-gate-" + artifactNameSuffix + ".log"
								}
								if id, cid, uerr := artUploader.UploadArtifact(ctx, req.RunID.String(), stageID, []string{logFile.Name()}, artifactName); uerr == nil {
									gateStats["logs_artifact_id"] = id
									gateStats["logs_bundle_cid"] = cid
								} else {
									slog.Warn("failed to upload "+artifactName, "run_id", req.RunID, "error", uerr)
								}
							}
							_ = os.Remove(logFile.Name())
						}
					}
				}
				return gateStats
			}

			// Include pre-gate stats if present.
			if execResult.PreGate != nil {
				gate["pre_gate"] = buildGateStats(execResult.PreGate.Metadata, execResult.PreGate.DurationMs, "pre")
			}

			// Include re-gate stats if present.
			if len(execResult.ReGates) > 0 {
				reGatesList := make([]map[string]any, 0, len(execResult.ReGates))
				for i, rg := range execResult.ReGates {
					suffix := fmt.Sprintf("re%d", i+1)
					reGatesList = append(reGatesList, buildGateStats(rg.Metadata, rg.DurationMs, suffix))
				}
				gate["re_gates"] = reGatesList
			}

			// Include final/post-mod gate stats if present and not already captured in pre-gate.
			// This handles the case where no healing occurred and the gate ran after the mod.
			if result.BuildGate != nil && execResult.PreGate == nil && len(execResult.ReGates) == 0 {
				gate = buildGateStats(result.BuildGate, result.Timings.BuildGateDuration.Milliseconds(), "")
			}

			stats["gate"] = gate
		}

		// No runner-provided artifact CIDs (node agent uploads artifacts directly).

		// Upload terminal status to server with a short, detached context so
		// we still attempt to report completion even if the run context is cancelled.
		statusCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if uploadErr := statusUploader.UploadStatus(statusCtx, req.RunID.String(), terminalStatus, reason, stats); uploadErr != nil {
			slog.Error("failed to upload terminal status", "run_id", req.RunID, "error", uploadErr)
		} else {
			slog.Info("terminal status uploaded successfully", "run_id", req.RunID, "status", terminalStatus)
		}
	}

	slog.Info("run execution completed",
		"run_id", req.RunID,
		"duration", duration,
		"exit_code", result.ExitCode,
	)
}

// shouldCreateMR determines if an MR should be created based on terminal status and options.
func shouldCreateMR(terminalStatus string, options map[string]any) bool {
	if terminalStatus == "succeeded" {
		if mrOnSuccess, ok := options["mr_on_success"].(bool); ok && mrOnSuccess {
			return true
		}
	}
	if terminalStatus == "failed" {
		if mrOnFail, ok := options["mr_on_fail"].(bool); ok && mrOnFail {
			return true
		}
	}
	return false
}

// createMR pushes the branch and creates a GitLab merge request.
func (r *runController) createMR(ctx context.Context, req StartRunRequest, manifest contracts.StepManifest, workspaceRoot string) (string, error) {
	// Extract GitLab options.
	gitlabPAT, _ := manifest.Options["gitlab_pat"].(string)
	gitlabDomain, _ := manifest.Options["gitlab_domain"].(string)

	// Validate required fields.
	if strings.TrimSpace(gitlabPAT) == "" {
		return "", fmt.Errorf("gitlab_pat is required for MR creation")
	}
	// Normalize domain: accept either host or full URL; coerce to host for MR client.
	gitlabDomain = strings.TrimSpace(gitlabDomain)
	if gitlabDomain == "" {
		gitlabDomain = "gitlab.com"
	} else {
		if strings.HasPrefix(gitlabDomain, "http://") || strings.HasPrefix(gitlabDomain, "https://") {
			if u, err := url.Parse(gitlabDomain); err == nil && u.Host != "" {
				gitlabDomain = u.Host
			}
		}
		// Remove any trailing slash artifacts.
		gitlabDomain = strings.TrimSuffix(gitlabDomain, "/")
	}

	// Extract project ID from repo URL.
	projectID, err := extractProjectIDFromRepoURL(req.RepoURL.String())
	if err != nil {
		return "", fmt.Errorf("extract project id: %w", err)
	}

	// Use a unique source branch per run: ploy-<ticket-id>.
	// This avoids MR conflicts on repeated runs regardless of the submitted target ref.
	sourceBranch := fmt.Sprintf("ploy-%s", req.RunID)

	// Create a commit with any workspace changes before pushing.
	if committed, cerr := git.EnsureCommit(ctx, workspaceRoot, "ploy-bot", "ploy-bot@ploy.local", fmt.Sprintf("Ploy: apply changes for run %s", req.RunID)); cerr != nil {
		slog.Error("git commit failed", "run_id", req.RunID, "error", cerr)
	} else if !committed {
		slog.Info("no changes detected; proceeding to push branch without commit", "run_id", req.RunID)
	}

	// Push branch to origin using git push (Phase E).
	pusher := git.NewPusher()
	pushOpts := git.PushOptions{
		RepoDir:   workspaceRoot,
		TargetRef: sourceBranch,
		PAT:       gitlabPAT,
		UserName:  "ploy-bot",
		UserEmail: "ploy-bot@ploy.local",
		RemoteURL: req.RepoURL.String(),
	}

	slog.Info("pushing branch to origin", "run_id", req.RunID, "source_branch", sourceBranch, "submitted_target", req.TargetRef)
	if err := pusher.Push(ctx, pushOpts); err != nil {
		return "", fmt.Errorf("git push: %w", err)
	}

	// Create MR via GitLab API.
	mrClient := gitlab.NewMRClient()
	mrReq := gitlab.MRCreateRequest{
		Domain:       gitlabDomain,
		ProjectID:    projectID,
		PAT:          gitlabPAT,
		Title:        fmt.Sprintf("Ploy: %s", req.RunID),
		SourceBranch: sourceBranch,
		TargetBranch: req.BaseRef.String(),
		Description:  fmt.Sprintf("Automated changes from Ploy run %s", req.RunID),
		Labels:       "ploy",
	}

	slog.Info("creating merge request", "run_id", req.RunID, "source", sourceBranch, "target", req.BaseRef)
	mrURL, err := mrClient.CreateMR(ctx, mrReq)
	if err != nil {
		return "", fmt.Errorf("create mr: %w", err)
	}

	return mrURL, nil
}

// extractProjectIDFromRepoURL extracts the URL-encoded project ID from a GitLab repo URL.
func extractProjectIDFromRepoURL(repoURL string) (string, error) {
	return gitlab.ExtractProjectIDFromURL(repoURL)
}

// gateRunMetadata captures gate execution metadata and timing for stats reporting.
type gateRunMetadata struct {
	Metadata   *contracts.BuildGateStageMetadata
	DurationMs int64
}

// executionResult wraps step.Result with additional gate run history for stats.
type executionResult struct {
	step.Result
	// PreGate captures the initial gate run metadata (if gate was executed).
	PreGate *gateRunMetadata
	// ReGates captures re-gate attempts after healing (if healing was attempted).
	ReGates []gateRunMetadata
}

// executeWithHealing runs the main step with optional healing loop when the build gate fails.
// It handles the gate-heal-regate orchestration as specified in build_gate_healing options.
func (r *runController) executeWithHealing(
	ctx context.Context,
	runner step.Runner,
	req StartRunRequest,
	manifest contracts.StepManifest,
	workspace string,
	outDir string,
	inDir *string,
) (executionResult, error) {
	// First execution attempt (includes pre-mod gate check).
	result, err := runner.Run(ctx, step.Request{
		TicketID:  types.TicketID(req.RunID),
		Manifest:  manifest,
		Workspace: workspace,
		OutDir:    outDir,
		InDir:     *inDir,
	})

	// Capture pre-gate metadata for stats (if gate was executed).
	var preGate *gateRunMetadata
	if result.BuildGate != nil {
		preGate = &gateRunMetadata{
			Metadata:   result.BuildGate,
			DurationMs: result.Timings.BuildGateDuration.Milliseconds(),
		}
	}

	// If execution succeeded or error is not a build gate failure, return immediately.
	if err == nil || !errors.Is(err, step.ErrBuildGateFailed) {
		return executionResult{Result: result, PreGate: preGate}, err
	}

	// Build gate failed. Check if healing is configured.
	healingConfig, hasHealing := req.Options["build_gate_healing"].(map[string]any)
	if !hasHealing {
		// No healing configured; return the gate failure.
		return executionResult{Result: result, PreGate: preGate}, err
	}

	// Extract healing parameters.
	retries := 1 // Default to 1 retry
	if r, ok := healingConfig["retries"].(int); ok && r > 0 {
		retries = r
	} else if rf, ok := healingConfig["retries"].(float64); ok && rf > 0 {
		retries = int(rf)
	}

	healingMods, ok := healingConfig["mods"].([]any)
	if !ok || len(healingMods) == 0 {
		slog.Warn("build_gate_healing configured but no mods provided", "run_id", req.RunID)
		return executionResult{Result: result, PreGate: preGate}, err
	}

	// Create /in directory if not already created (for build-gate.log).
	if *inDir == "" {
		tmpInDir, dirErr := os.MkdirTemp("", "ploy-mod-in-*")
		if dirErr != nil {
			slog.Error("failed to create /in directory for healing", "run_id", req.RunID, "error", dirErr)
			return executionResult{Result: result, PreGate: preGate}, err
		}
		*inDir = tmpInDir
		// Caller handles cleanup via defer.

		// Write build-gate.log to /in for healing containers.
		if result.BuildGate != nil && result.BuildGate.LogsText != "" {
			inLogPath := filepath.Join(*inDir, "build-gate.log")
			if writeErr := os.WriteFile(inLogPath, []byte(result.BuildGate.LogsText), 0o644); writeErr != nil {
				slog.Warn("failed to write /in/build-gate.log", "run_id", req.RunID, "error", writeErr)
			} else {
				slog.Info("build-gate.log persisted to /in for healing", "run_id", req.RunID, "path", inLogPath)
			}
		}
	}

	// Track re-gate runs for stats.
	var reGates []gateRunMetadata

	// Attempt healing loop.
	for attempt := 1; attempt <= retries; attempt++ {
		slog.Info("starting healing attempt", "run_id", req.RunID, "attempt", attempt, "max_retries", retries)

		// Execute each healing mod in sequence.
		for idx, modEntry := range healingMods {
			healManifest, buildErr := buildHealingManifest(req, modEntry, idx)
			if buildErr != nil {
				slog.Error("failed to build healing manifest", "run_id", req.RunID, "mod_index", idx, "error", buildErr)
				return executionResult{Result: result, PreGate: preGate, ReGates: reGates}, fmt.Errorf("build healing manifest[%d]: %w", idx, buildErr)
			}

			slog.Info("executing healing mod", "run_id", req.RunID, "attempt", attempt, "mod_index", idx, "image", healManifest.Image)

			// Run the healing mod container.
			healResult, healErr := runner.Run(ctx, step.Request{
				TicketID:  types.TicketID(req.RunID),
				Manifest:  healManifest,
				Workspace: workspace,
				OutDir:    outDir,
				InDir:     *inDir,
			})

			if healErr != nil {
				slog.Error("healing mod execution failed", "run_id", req.RunID, "mod_index", idx, "error", healErr)
				return executionResult{Result: result, PreGate: preGate, ReGates: reGates}, fmt.Errorf("healing mod[%d] failed: %w", idx, healErr)
			}

			if healResult.ExitCode != 0 {
				slog.Warn("healing mod exited with non-zero code", "run_id", req.RunID, "mod_index", idx, "exit_code", healResult.ExitCode)
				// Continue with remaining mods; we'll check gate after all mods run.
			}

			// Upload /out artifacts for this healing mod if present.
			if uploadErr := uploadOutDirIfPresent(ctx, r.cfg, req.RunID.String(), stageIDFromOptions(req.Options), outDir); uploadErr != nil {
				slog.Warn("failed to upload /out for healing mod", "run_id", req.RunID, "mod_index", idx, "error", uploadErr)
			}
		}

		// Re-run the gate after healing mods.
		slog.Info("re-running build gate after healing", "run_id", req.RunID, "attempt", attempt)

		gateSpec := manifest.Gate
		//lint:ignore SA1019 Backward compatibility: support deprecated Shift by mapping to Gate.
		if gateSpec == nil && manifest.Shift != nil {
			gateSpec = &contracts.StepGateSpec{
				Enabled: manifest.Shift.Enabled, //lint:ignore SA1019 compat field access
				Profile: manifest.Shift.Profile, //lint:ignore SA1019 compat field access
				Env:     manifest.Shift.Env,     //lint:ignore SA1019 compat field access
			}
		}

		if runner.Gate != nil && gateSpec != nil && gateSpec.Enabled {
			regateStart := time.Now()
			gateMetadata, gateErr := runner.Gate.Execute(ctx, gateSpec, workspace)
			regateDuration := time.Since(regateStart)

			if gateErr != nil {
				slog.Error("re-gate execution failed", "run_id", req.RunID, "error", gateErr)
				return executionResult{Result: result, PreGate: preGate, ReGates: reGates}, fmt.Errorf("re-gate execution failed: %w", gateErr)
			}

			result.BuildGate = gateMetadata

			// Capture re-gate metadata for stats.
			reGates = append(reGates, gateRunMetadata{
				Metadata:   gateMetadata,
				DurationMs: regateDuration.Milliseconds(),
			})

			// Check if gate passed.
			gatePassed := false
			if len(gateMetadata.StaticChecks) > 0 {
				gatePassed = gateMetadata.StaticChecks[0].Passed
			}

			if gatePassed {
				slog.Info("build gate passed after healing", "run_id", req.RunID, "attempt", attempt)
				// Gate passed; proceed to main mod execution.
				// Disable the gate in the manifest since we've already validated the codebase.
				manifestForMainMod := manifest
				manifestForMainMod.Gate = &contracts.StepGateSpec{Enabled: false}
				//lint:ignore SA1019 Backward compatibility: also disable deprecated Shift field.
				manifestForMainMod.Shift = nil

				// Execute the main mod without re-running the gate.
				mainResult, mainErr := runner.Run(ctx, step.Request{
					TicketID:  types.TicketID(req.RunID),
					Manifest:  manifestForMainMod,
					Workspace: workspace,
					OutDir:    outDir,
					InDir:     *inDir,
				})
				// Return with all gate history.
				return executionResult{Result: mainResult, PreGate: preGate, ReGates: reGates}, mainErr
			}

			// Re-gate failed; continue to next retry or exit when exhausted.
			slog.Warn("build gate still failing after healing", "run_id", req.RunID, "attempt", attempt)
		}
	}

	// Retries exhausted; return the gate failure.
	slog.Error("healing retries exhausted, build gate still failing", "run_id", req.RunID)
	return executionResult{Result: result, PreGate: preGate, ReGates: reGates}, fmt.Errorf("%w: healing retries exhausted", step.ErrBuildGateFailed)
}
