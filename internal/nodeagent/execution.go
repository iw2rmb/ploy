package nodeagent

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/iw2rmb/ploy/internal/worker/hydration"
	"github.com/iw2rmb/ploy/internal/workflow/runtime/step"
)

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
		// Fallback: proceed without a container runtime (simulated execution)
		slog.Warn("docker unavailable; falling back to stub runtime", "run_id", req.RunID, "error", err)
		containerRuntime = nil
	}

	diffGenerator := step.NewFilesystemDiffGenerator(step.FilesystemDiffGeneratorOptions{})

	gateExecutor := step.NewDockerGateExecutor(containerRuntime)

	// Create log streamer to stream logs as gzipped chunks to the server.
	logStreamer := NewLogStreamer(r.cfg, req.RunID, "")
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
		Artifacts:  newSizeLimitedPublisher(artifactPublisher, maxArtifactSize),
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

	// Execute the step.
	startTime := time.Now()
	result, execErr := runner.Run(ctx, step.Request{
		Manifest:  manifest,
		Workspace: workspaceRoot,
		OutDir:    outDir,
	})
	duration := time.Since(startTime)

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
						"publish_duration_ms":    result.Timings.PublishDuration.Milliseconds(),
						"total_duration_ms":      result.Timings.TotalDuration.Milliseconds(),
					},
				}

				// Use stage_id provided by control plane (no backward-compat fallback).
				stageID, _ := req.Options["stage_id"].(string)

				// Upload the diff to the server.
				if err := diffUploader.UploadDiff(ctx, req.RunID, stageID, diffBytes, summary); err != nil {
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
						if errU := artUploader.UploadArtifact(ctx, req.RunID, stageID, []string{diffFile.Name()}, "diff"); errU != nil {
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
				if err := artifactUploader.UploadArtifact(ctx, req.RunID, stageID, paths, artifactName); err != nil {
					slog.Error("failed to upload artifact bundle", "run_id", req.RunID, "error", err)
				} else {
					slog.Info("artifact bundle uploaded successfully", "run_id", req.RunID, "paths", len(paths))
				}
			}
		}
	}

	// Always attempt to bundle and upload /out regardless of artifact_paths.
	if err := uploadOutDirIfPresent(ctx, r.cfg, req.RunID, stageIDFromOptions(req.Options), outDir); err != nil {
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
			reason = &errMsg
		} else if result.ExitCode != 0 {
			terminalStatus = "failed"
			failureMsg := fmt.Sprintf("exit code %d", result.ExitCode)
			reason = &failureMsg
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
				"publish_duration_ms":    result.Timings.PublishDuration.Milliseconds(),
				"total_duration_ms":      result.Timings.TotalDuration.Milliseconds(),
			},
		}

		// Add artifact CIDs if available.
		if result.DiffArtifact.CID != "" {
			stats["diff_cid"] = result.DiffArtifact.CID
		}
		if result.LogArtifact.CID != "" {
			stats["log_cid"] = result.LogArtifact.CID
		}

		// Upload terminal status to server with a short, detached context so
		// we still attempt to report completion even if the run context is cancelled.
		statusCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if uploadErr := statusUploader.UploadStatus(statusCtx, req.RunID, terminalStatus, reason, stats); uploadErr != nil {
			slog.Error("failed to upload terminal status", "run_id", req.RunID, "error", uploadErr)
		} else {
			slog.Info("terminal status uploaded successfully", "run_id", req.RunID, "status", terminalStatus)
		}
	}

	slog.Info("run execution completed",
		"run_id", req.RunID,
		"duration", duration,
		"exit_code", result.ExitCode,
		"diff_cid", result.DiffArtifact.CID,
		"log_cid", result.LogArtifact.CID,
	)
}
