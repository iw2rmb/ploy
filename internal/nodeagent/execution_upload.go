// execution_upload.go centralizes artifact and status upload operations.
//
// This file handles artifact bundling, /out directory upload, and final status
// reporting to the control plane. Upload operations include retry logic and
// error handling. Isolating uploads from execution orchestration keeps the
// orchestrator focused on run lifecycle while this file owns all HTTP
// interactions for result persistence.
package nodeagent

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	types "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

// uploadConfiguredArtifacts uploads artifact bundles specified in the typed RunOptions.
// It resolves paths relative to the workspace, validates they exist, and bundles them
// with the configured artifact_name from the manifest options.
//
// Missing paths generate warnings but do not fail the upload operation.
func (r *runController) uploadConfiguredArtifacts(ctx context.Context, req StartRunRequest, typedOpts RunOptions, manifest contracts.StepManifest, workspace string) {
	// Use typed Artifacts.Paths from RunOptions (already parsed by parseRunOptions).
	if len(typedOpts.Artifacts.Paths) == 0 {
		return
	}

	// Resolve workspace-relative paths and validate existence.
	var paths []string
	for _, p := range typedOpts.Artifacts.Paths {
		fullPath := filepath.Join(workspace, p)
		if _, err := os.Stat(fullPath); err == nil {
			paths = append(paths, fullPath)
		} else {
			slog.Warn("artifact path not found", "run_id", req.RunID, "path", p)
		}
	}

	if len(paths) == 0 {
		return
	}

	artifactUploader, err := NewArtifactUploader(r.cfg)
	if err != nil {
		slog.Error("failed to create artifact uploader", "run_id", req.RunID, "error", err)
		return
	}

	// Use typed artifact name from RunOptions.
	artifactName := typedOpts.Artifacts.Name

	if _, _, err := artifactUploader.UploadArtifact(ctx, req.RunID, req.JobID, paths, artifactName); err != nil {
		slog.Error("failed to upload artifact bundle", "run_id", req.RunID, "job_id", req.JobID, "error", err)
	} else {
		slog.Info("artifact bundle uploaded successfully", "run_id", req.RunID, "job_id", req.JobID, "paths", len(paths))
	}
}

// uploadOutDir bundles and uploads the /out directory when it contains files.
// The bundle is named "mod-out" for consistency with client expectations.
func (r *runController) uploadOutDir(ctx context.Context, runID types.RunID, jobID types.JobID, outDir string) error {
	if outDir == "" {
		return nil
	}

	hasFiles, files := listFilesRecursive(outDir)
	if !hasFiles {
		return nil
	}

	artifactUploader, err := NewArtifactUploader(r.cfg)
	if err != nil {
		return fmt.Errorf("create artifact uploader: %w", err)
	}

	if _, _, err := artifactUploader.UploadArtifact(ctx, runID, jobID, files, "mod-out"); err != nil {
		return fmt.Errorf("upload /out bundle: %w", err)
	}

	return nil
}

// uploadStatus uploads terminal status and execution statistics to the control plane.
// It uses a short, detached context to ensure the status is reported even if the
// run context is cancelled. Retry logic is handled by StatusUploader.
// exitCode is the exit code from job execution (required for terminal status).
// jobID is the authoritative job identifier (avoids float equality issues with step_index).
func (r *runController) uploadStatus(ctx context.Context, runID, status string, exitCode *int32, stats types.RunStats, stepIndex types.StepIndex, jobID types.JobID) error {
	statusUploader, err := NewStatusUploader(r.cfg)
	if err != nil {
		return fmt.Errorf("create status uploader: %w", err)
	}

	// Dereference exitCode for logging so we log the actual numeric code
	// instead of the pointer address. The StatusUploader already dereferences
	// exitCode when building the JSON payload, so this only affects logs.
	var loggedExitCode any
	if exitCode != nil {
		loggedExitCode = *exitCode
	}

	// Use a short, detached context to attempt status upload even if run context is cancelled.
	// The 10-second timeout provides sufficient time for retries while preventing indefinite hangs.
	statusCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if uploadErr := statusUploader.UploadJobStatus(statusCtx, jobID, status, exitCode, stats); uploadErr != nil {
		return fmt.Errorf("upload job status: %w", uploadErr)
	}

	slog.Info("terminal status uploaded successfully", "run_id", runID, "job_id", jobID, "status", status, "exit_code", loggedExitCode, "step_index", stepIndex)
	return nil
}

// uploadGateLogsArtifact uploads build gate logs as an artifact bundle and attaches
// artifact IDs to the gate stats payload. The artifact name includes a suffix to
// distinguish pre-gate, re-gate, and final gate runs (e.g., "build-gate-pre.log").
//
// This allows clients to download detailed gate execution logs for debugging.
func (r *runController) uploadGateLogsArtifact(runID types.RunID, jobID types.JobID, logsText, artifactNameSuffix string, gateStats map[string]any) {
	logFile, err := os.CreateTemp("", "ploy-gate-*.log")
	if err != nil {
		return
	}
	defer func() { _ = os.Remove(logFile.Name()) }()

	_, _ = logFile.WriteString(logsText)
	_ = logFile.Close()

	artUploader, err := NewArtifactUploader(r.cfg)
	if err != nil {
		return
	}

	artifactName := "build-gate.log"
	if artifactNameSuffix != "" {
		artifactName = "build-gate-" + artifactNameSuffix + ".log"
	}

	// Upload with background context to ensure logs are uploaded even if run context is cancelled.
	if id, cid, uerr := artUploader.UploadArtifact(context.Background(), runID, jobID, []string{logFile.Name()}, artifactName); uerr == nil {
		gateStats["logs_artifact_id"] = id
		gateStats["logs_bundle_cid"] = cid
	} else {
		slog.Warn("failed to upload "+artifactName, "run_id", runID, "job_id", jobID, "error", uerr)
	}
}
