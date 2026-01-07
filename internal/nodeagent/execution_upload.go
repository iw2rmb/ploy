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
	"strings"
	"time"

	types "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

// uploadConfiguredArtifacts uploads artifact bundles specified in the typed RunOptions.
// It resolves paths relative to the workspace, validates they exist, and bundles them
// with the configured artifact_name from the manifest options.
//
// Missing paths generate warnings but do not fail the upload operation.
//
// Security: Artifact paths are validated to prevent path traversal attacks.
// Paths must be:
//   - Relative (not absolute)
//   - Contained within the workspace (no "../.." escaping)
//
// Invalid paths are skipped with a warning and no upload attempt is made.
func (r *runController) uploadConfiguredArtifacts(ctx context.Context, req StartRunRequest, typedOpts RunOptions, manifest contracts.StepManifest, workspace string) {
	// Use typed Artifacts.Paths from RunOptions (already parsed from spec).
	if len(typedOpts.Artifacts.Paths) == 0 {
		return
	}

	// Resolve workspace-relative paths and validate existence.
	// Security: Each path is validated against path traversal attacks before processing.
	var paths []string
	for _, p := range typedOpts.Artifacts.Paths {
		// Validate artifact path to prevent path traversal outside workspace.
		// This blocks attacks like artifact_paths: ["../../etc/passwd"]
		if !isValidArtifactPath(p, workspace) {
			slog.Warn("artifact path rejected: path traversal attempt or absolute path",
				"run_id", req.RunID,
				"job_id", req.JobID,
				"path", p,
			)
			continue
		}

		fullPath := filepath.Clean(filepath.Join(workspace, p))
		if _, err := os.Stat(fullPath); err == nil {
			paths = append(paths, fullPath)
		} else {
			slog.Warn("artifact path not found", "run_id", req.RunID, "path", p)
		}
	}

	if len(paths) == 0 {
		return
	}

	// Use the shared artifact uploader instead of creating a new one per call.
	// The uploader is initialized once per runController and reused across all jobs.
	// This enables HTTP connection pooling and reduces per-upload overhead.
	if r.artifactUploader == nil {
		slog.Error("artifact uploader not initialized", "run_id", req.RunID)
		return
	}

	// Use typed artifact name from RunOptions.
	artifactName := typedOpts.Artifacts.Name

	if _, _, err := r.artifactUploader.UploadArtifact(ctx, req.RunID, req.JobID, paths, artifactName); err != nil {
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

	// Use the shared artifact uploader instead of creating a new one per call.
	// The uploader is initialized once per runController and reused across all jobs.
	if r.artifactUploader == nil {
		return fmt.Errorf("artifact uploader not initialized")
	}

	if _, _, err := r.artifactUploader.UploadArtifact(ctx, runID, jobID, files, "mod-out"); err != nil {
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
	// Use the shared status uploader instead of creating a new one per call.
	// The uploader is initialized once per runController and reused across all jobs.
	if r.statusUploader == nil {
		return fmt.Errorf("status uploader not initialized")
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

	if uploadErr := r.statusUploader.UploadJobStatus(statusCtx, jobID, status, exitCode, stats); uploadErr != nil {
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
func (r *runController) uploadGateLogsArtifact(runID types.RunID, jobID types.JobID, logsText, artifactNameSuffix string, phase *types.RunStatsGatePhase) {
	if phase == nil {
		return
	}

	// Use the shared artifact uploader instead of creating a new one per call.
	if r.artifactUploader == nil {
		slog.Warn("artifact uploader not initialized, skipping gate logs upload", "run_id", runID, "job_id", jobID)
		return
	}

	logFile, err := os.CreateTemp("", "ploy-gate-*.log")
	if err != nil {
		return
	}
	defer func() { _ = os.Remove(logFile.Name()) }()

	_, _ = logFile.WriteString(logsText)
	_ = logFile.Close()

	artifactName := "build-gate.log"
	if artifactNameSuffix != "" {
		artifactName = "build-gate-" + artifactNameSuffix + ".log"
	}

	// Upload with background context to ensure logs are uploaded even if run context is cancelled.
	if id, cid, uerr := r.artifactUploader.UploadArtifact(context.Background(), runID, jobID, []string{logFile.Name()}, artifactName); uerr == nil {
		phase.LogsArtifactID = id
		phase.LogsBundleCID = cid
	} else {
		slog.Warn("failed to upload "+artifactName, "run_id", runID, "job_id", jobID, "error", uerr)
	}
}

// isValidArtifactPath validates that an artifact path is safe for upload.
// It prevents path traversal attacks by ensuring:
//   - The path is not absolute (no "/etc/passwd" style paths)
//   - The path does not escape the workspace (no "../../" traversal)
//   - The resolved full path is contained within the workspace directory
//
// This security check protects against malicious artifact_paths specifications
// that could exfiltrate arbitrary files from the host system.
//
// The function uses filepath.Rel to compute the relative path from workspace
// to the joined full path. If the result starts with ".." then the path escapes
// the workspace boundary.
func isValidArtifactPath(artifactPath string, workspace string) bool {
	// Reject empty paths.
	if artifactPath == "" || strings.TrimSpace(artifactPath) == "" {
		return false
	}

	// Reject absolute paths — artifact paths must be workspace-relative.
	// filepath.IsAbs handles OS-specific absolute path detection (e.g., "/" on Unix, "C:\" on Windows).
	if filepath.IsAbs(artifactPath) {
		return false
	}

	// Join the workspace and artifact path, then clean it to resolve any ".." components.
	// filepath.Join already calls filepath.Clean internally, but we call Clean explicitly
	// to make the intent clear.
	fullPath := filepath.Clean(filepath.Join(workspace, artifactPath))

	// Compute the relative path from workspace to the resolved full path.
	// If this fails or the result starts with "..", the path escapes the workspace.
	rel, err := filepath.Rel(workspace, fullPath)
	if err != nil {
		// filepath.Rel can fail if paths are on different volumes (Windows).
		// Treat any error as invalid for safety.
		return false
	}

	// Check if the relative path starts with ".." which indicates traversal outside workspace.
	// We check for both ".." exactly and "../" prefix to catch all escape attempts.
	// After Clean, any path escaping the workspace will have ".." at the start.
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return false
	}

	return true
}
