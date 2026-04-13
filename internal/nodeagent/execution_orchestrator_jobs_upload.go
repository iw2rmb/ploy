// execution_orchestrator_jobs_upload.go contains upload, status reporting,
// diff generation, and artifact helpers used by job executors.
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
	gitpkg "github.com/iw2rmb/ploy/internal/nodeagent/git"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
	"github.com/iw2rmb/ploy/internal/workflow/lifecycle"
	"github.com/iw2rmb/ploy/internal/workflow/step"
)

// uploadConfiguredArtifacts uploads artifact bundles specified in the typed RunOptions.
// Relative paths resolve in workspace; /out/* paths resolve from outDir and keep
// deterministic archive names under out/.
func (r *runController) uploadConfiguredArtifacts(ctx context.Context, req StartRunRequest, typedOpts RunOptions, manifest contracts.StepManifest, workspace, outDir string) {
	if len(typedOpts.Artifacts.Paths) == 0 {
		return
	}

	entries := make([]ArtifactBundleEntry, 0, len(typedOpts.Artifacts.Paths))
	for _, p := range typedOpts.Artifacts.Paths {
		fullPath, archivePath, ok := resolveConfiguredArtifactPath(p, workspace, outDir)
		if !ok {
			slog.Warn("artifact path rejected",
				"run_id", req.RunID, "job_id", req.JobID, "path", p,
			)
			continue
		}

		if _, err := os.Stat(fullPath); err == nil {
			entries = append(entries, ArtifactBundleEntry{
				SourcePath:  fullPath,
				ArchivePath: archivePath,
			})
		} else {
			slog.Warn("artifact path not found", "run_id", req.RunID, "path", p)
		}
	}

	if len(entries) == 0 {
		return
	}

	if _, _, err := r.artifactUploader.UploadArtifactEntries(ctx, req.RunID, req.JobID, entries, typedOpts.Artifacts.Name); err != nil {
		slog.Error("failed to upload artifact bundle", "run_id", req.RunID, "job_id", req.JobID, "error", err)
	} else {
		slog.Info("artifact bundle uploaded successfully", "run_id", req.RunID, "job_id", req.JobID, "paths", len(entries))
	}
}

func resolveConfiguredArtifactPath(rawPath, workspace, outDir string) (fullPath string, archivePath string, ok bool) {
	p := strings.TrimSpace(rawPath)
	if p == "" {
		return "", "", false
	}

	if strings.HasPrefix(p, "/out/") {
		if strings.TrimSpace(outDir) == "" {
			return "", "", false
		}
		rel := strings.TrimPrefix(p, "/out/")
		cleanRel := filepath.Clean(rel)
		if cleanRel == "." || strings.HasPrefix(cleanRel, "..") || filepath.IsAbs(cleanRel) {
			return "", "", false
		}
		return filepath.Join(outDir, cleanRel), filepath.ToSlash(filepath.Join("out", cleanRel)), true
	}

	if !isValidArtifactPath(p, workspace) {
		return "", "", false
	}

	return filepath.Clean(filepath.Join(workspace, p)), "", true
}

// uploadOutDirBundle bundles and uploads the /out directory when it contains files.
// Archive paths are rooted at out/ to preserve deterministic in-container paths.
func (r *runController) uploadOutDirBundle(ctx context.Context, runID types.RunID, jobID types.JobID, outDir, artifactName string) error {
	if r.artifactUploader == nil {
		return nil
	}
	if outDir == "" {
		return nil
	}

	hasFiles, _ := listFilesRecursive(outDir)
	if !hasFiles {
		return nil
	}
	name := strings.TrimSpace(artifactName)
	if name == "" {
		name = "mig-out"
	}

	entries := []ArtifactBundleEntry{{
		SourcePath:  outDir,
		ArchivePath: "out",
	}}
	if _, _, err := r.artifactUploader.UploadArtifactEntries(ctx, runID, jobID, entries, name); err != nil {
		return fmt.Errorf("upload /out bundle: %w", err)
	}

	return nil
}

// uploadStatus uploads terminal status and execution statistics to the control plane.
// Uses a detached context to ensure reporting even if the run context is cancelled.
func (r *runController) uploadStatus(ctx context.Context, runID, status string, exitCode *int32, stats types.RunStats, jobID types.JobID, repoSHAOut ...string) error {
	var loggedExitCode any
	if exitCode != nil {
		loggedExitCode = *exitCode
	}
	loggedRepoSHAOut := ""
	if len(repoSHAOut) > 0 {
		loggedRepoSHAOut = strings.TrimSpace(repoSHAOut[0])
	}

	statusCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if uploadErr := r.statusUploader.UploadJobStatus(statusCtx, jobID, status, exitCode, stats, loggedRepoSHAOut); uploadErr != nil {
		return fmt.Errorf("upload job status: %w", uploadErr)
	}

	slog.Info("terminal status uploaded successfully", "run_id", runID, "job_id", jobID, "status", status, "exit_code", loggedExitCode, "repo_sha_out", loggedRepoSHAOut)
	return nil
}

// reportTerminalStatus uploads the final job status based on execution outcome.
// Handles runtime errors and maps process exit code to terminal status.
func (r *runController) reportTerminalStatus(
	ctx context.Context,
	req StartRunRequest,
	runErr error,
	result step.Result,
	stats types.RunStats,
	repoSHAOut string,
	duration time.Duration,
) {
	var status types.JobStatus
	var exitCode *int32

	if runErr != nil {
		status = lifecycle.JobStatusFromRunError(runErr)
		if status == types.JobStatusError {
			v := int32(-1)
			exitCode = &v
		}
		r.emitRunException(req, "node runtime execution error", runErr, map[string]any{
			"component": "run_controller", "status": status.String(), "duration_ms": duration.Milliseconds(),
		})
	} else {
		status = lifecycle.JobStatusFromExitCodeForJobType(req.JobType, result.ExitCode)
		ec := int32(result.ExitCode)
		exitCode = &ec
	}

	if uploadErr := r.uploadStatus(ctx, req.RunID.String(), status.String(), exitCode, stats, req.JobID, repoSHAOut); uploadErr != nil {
		slog.Error("failed to upload terminal status", "run_id", req.RunID, "job_id", req.JobID, "error", uploadErr)
	}
	slog.Info("job terminated", "run_id", req.RunID, "job_id", req.JobID, "status", status,
		"exit_code", result.ExitCode, "duration", duration)
}

// uploadGateLogsArtifact uploads build gate logs as an artifact bundle and attaches
// artifact IDs to the gate stats payload.
func (r *runController) uploadGateLogsArtifact(runID types.RunID, jobID types.JobID, logsText, artifactNameSuffix string, phase *types.RunStatsGatePhase) {
	if phase == nil {
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

func (r *runController) computeRepoSHAOut(ctx context.Context, req StartRunRequest, workspace string, inputTree string) string {
	repoSHAIn := strings.TrimSpace(req.RepoSHAIn.String())
	if repoSHAIn == "" {
		slog.Warn("repo_sha_in missing on claimed job; skipping repo_sha_out calculation", "run_id", req.RunID, "job_id", req.JobID)
		return ""
	}
	preTree := strings.TrimSpace(inputTree)
	repoSHAOut, err := gitpkg.ComputeRepoSHAV1(ctx, workspace, repoSHAIn, preTree)
	if err != nil {
		slog.Warn("failed to compute repo_sha_out", "run_id", req.RunID, "job_id", req.JobID, "repo_sha_in", repoSHAIn, "error", err)
		return ""
	}
	return repoSHAOut
}

// uploadDiffWithBaseline generates and uploads a diff between a baseline snapshot
// and the post-execution workspace. Silent no-op when baseDir is empty.
// When warnOnMissing is true, logs a warning for the missing baseline.
func (r *runController) uploadDiffWithBaseline(
	ctx context.Context,
	runID types.RunID,
	jobID types.JobID,
	jobName string,
	diffGenerator step.DiffGenerator,
	baseDir string,
	workspace string,
	result step.Result,
	diffType types.DiffJobType,
	warnOnMissing bool,
) {
	if strings.TrimSpace(baseDir) == "" {
		if warnOnMissing {
			slog.Warn(diffType.String()+" diff skipped: baseline snapshot missing",
				"run_id", runID, "job_id", jobID, "job_name", jobName)
		}
		return
	}
	r.uploadJobDiff(ctx, runID, jobID, diffGenerator, baseDir, workspace, result, diffType)
}

// uploadJobDiff is the shared implementation for generating, summarizing, and uploading
// a diff between a baseline snapshot and the post-execution workspace.
func (r *runController) uploadJobDiff(
	ctx context.Context,
	runID types.RunID,
	jobID types.JobID,
	diffGenerator step.DiffGenerator,
	baseDir, workspace string,
	result step.Result,
	diffType types.DiffJobType,
) {
	if diffGenerator == nil {
		return
	}

	label := diffType.String()

	diffBytes, err := diffGenerator.GenerateBetween(ctx, baseDir, workspace)
	if err != nil {
		slog.Error("failed to generate "+label+" diff", "run_id", runID, "job_id", jobID, "error", err)
		return
	}
	if len(diffBytes) == 0 {
		slog.Info("no diff to upload (no changes between baseline and workspace)", "run_id", runID, "job_id", jobID, "diff_type", label)
		return
	}

	patchStats := step.CountPatchStats(diffBytes)
	summary := types.NewDiffSummaryBuilder().
		JobType(label).
		ExitCode(result.ExitCode).
		FilesChanged(patchStats.FilesChanged).
		LinesAdded(patchStats.LinesAdded).
		LinesRemoved(patchStats.LinesRemoved).
		Timings(
			time.Duration(result.Timings.HydrationDuration).Milliseconds(),
			time.Duration(result.Timings.ExecutionDuration).Milliseconds(),
			time.Duration(result.Timings.DiffDuration).Milliseconds(),
			time.Duration(result.Timings.TotalDuration).Milliseconds(),
		).
		MustBuild()

	if err := r.diffUploader.UploadDiff(ctx, runID, jobID, diffBytes, summary); err != nil {
		slog.Error("failed to upload "+label+" diff", "run_id", runID, "job_id", jobID, "error", err)
		return
	}

	slog.Info(label+" diff uploaded successfully", "run_id", runID, "job_id", jobID, "size", len(diffBytes))
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
