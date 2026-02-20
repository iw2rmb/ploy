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
// Paths are resolved relative to workspace and validated against path traversal.
func (r *runController) uploadConfiguredArtifacts(ctx context.Context, req StartRunRequest, typedOpts RunOptions, manifest contracts.StepManifest, workspace string) {
	if len(typedOpts.Artifacts.Paths) == 0 {
		return
	}

	var paths []string
	for _, p := range typedOpts.Artifacts.Paths {
		if !isValidArtifactPath(p, workspace) {
			slog.Warn("artifact path rejected: path traversal attempt or absolute path",
				"run_id", req.RunID, "job_id", req.JobID, "path", p)
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

	if err := r.ensureUploaders(); err != nil {
		slog.Error("failed to initialize uploaders", "run_id", req.RunID, "error", err)
		return
	}

	if _, _, err := r.artifactUploader.UploadArtifact(ctx, req.RunID, req.JobID, paths, typedOpts.Artifacts.Name); err != nil {
		slog.Error("failed to upload artifact bundle", "run_id", req.RunID, "job_id", req.JobID, "error", err)
	} else {
		slog.Info("artifact bundle uploaded successfully", "run_id", req.RunID, "job_id", req.JobID, "paths", len(paths))
	}
}

// uploadOutDir bundles and uploads the /out directory when it contains files.
func (r *runController) uploadOutDir(ctx context.Context, runID types.RunID, jobID types.JobID, outDir string) error {
	if outDir == "" {
		return nil
	}

	hasFiles, files := listFilesRecursive(outDir)
	if !hasFiles {
		return nil
	}

	if err := r.ensureUploaders(); err != nil {
		return fmt.Errorf("initialize uploaders: %w", err)
	}

	if _, _, err := r.artifactUploader.UploadArtifact(ctx, runID, jobID, files, "mod-out"); err != nil {
		return fmt.Errorf("upload /out bundle: %w", err)
	}

	return nil
}

// uploadStatus uploads terminal status and execution statistics to the control plane.
// Uses a detached context to ensure reporting even if the run context is cancelled.
func (r *runController) uploadStatus(ctx context.Context, runID, status string, exitCode *int32, stats types.RunStats, stepIndex types.StepIndex, jobID types.JobID) error {
	if err := r.ensureUploaders(); err != nil {
		return fmt.Errorf("initialize uploaders: %w", err)
	}

	var loggedExitCode any
	if exitCode != nil {
		loggedExitCode = *exitCode
	}

	statusCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if uploadErr := r.statusUploader.UploadJobStatus(statusCtx, jobID, status, exitCode, stats); uploadErr != nil {
		return fmt.Errorf("upload job status: %w", uploadErr)
	}

	slog.Info("terminal status uploaded successfully", "run_id", runID, "job_id", jobID, "status", status, "exit_code", loggedExitCode, "step_index", stepIndex)
	return nil
}

// uploadGateLogsArtifact uploads build gate logs as an artifact bundle and attaches
// artifact IDs to the gate stats payload.
func (r *runController) uploadGateLogsArtifact(runID types.RunID, jobID types.JobID, logsText, artifactNameSuffix string, phase *types.RunStatsGatePhase) {
	if phase == nil {
		return
	}

	if err := r.ensureUploaders(); err != nil {
		slog.Warn("failed to initialize uploaders, skipping gate logs upload", "run_id", runID, "job_id", jobID, "error", err)
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

	if id, cid, uerr := r.artifactUploader.UploadArtifact(context.Background(), runID, jobID, []string{logFile.Name()}, artifactName); uerr == nil {
		phase.LogsArtifactID = id
		phase.LogsBundleCID = cid
	} else {
		slog.Warn("failed to upload "+artifactName, "run_id", runID, "job_id", jobID, "error", uerr)
	}
}

// isValidArtifactPath validates that an artifact path is safe for upload.
// Prevents path traversal by ensuring the path is relative and contained within workspace.
func isValidArtifactPath(artifactPath string, workspace string) bool {
	if artifactPath == "" || strings.TrimSpace(artifactPath) == "" {
		return false
	}
	if filepath.IsAbs(artifactPath) {
		return false
	}

	fullPath := filepath.Clean(filepath.Join(workspace, artifactPath))
	rel, err := filepath.Rel(workspace, fullPath)
	if err != nil {
		return false
	}

	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return false
	}

	return true
}
