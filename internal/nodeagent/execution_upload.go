// execution_upload.go centralizes artifact and status upload operations.
//
// This file handles diff upload, artifact bundling, /out directory upload,
// and final status reporting to the control plane. Upload operations include
// retry logic and error handling. Isolating uploads from execution orchestration
// keeps the orchestrator focused on run lifecycle while this file owns all
// HTTP interactions for result persistence.
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
	"github.com/iw2rmb/ploy/internal/workflow/runtime/step"
)

// uploadDiff generates and uploads the workspace diff to the control plane.
// It compresses the diff, uploads it with execution summary metadata, and also
// uploads the diff as an artifact bundle named "diff" for client download.
//
// The diff is uploaded to both:
//  1. The diff endpoint for execution metadata tracking
//  2. The artifact endpoint as a "diff" bundle for client convenience
//
// If the diff generator is nil or produces no changes, no uploads occur.
func (r *runController) uploadDiff(ctx context.Context, runID, stageID string, diffGenerator step.DiffGenerator, workspace string, result step.Result) {
	if diffGenerator == nil {
		return
	}

	// Generate workspace diff.
	diffBytes, err := diffGenerator.Generate(ctx, workspace)
	if err != nil {
		slog.Error("failed to generate diff", "run_id", runID, "error", err)
		return
	}

	if len(diffBytes) == 0 {
		return
	}

	// Upload diff with execution summary metadata to diff endpoint.
	diffUploader, err := NewDiffUploader(r.cfg)
	if err != nil {
		slog.Error("failed to create diff uploader", "run_id", runID, "error", err)
		return
	}

	// Build execution summary with timings for diff metadata.
	summary := types.DiffSummary{
		"exit_code": result.ExitCode,
		"timings": map[string]interface{}{
			"hydration_duration_ms":  result.Timings.HydrationDuration.Milliseconds(),
			"execution_duration_ms":  result.Timings.ExecutionDuration.Milliseconds(),
			"build_gate_duration_ms": result.Timings.BuildGateDuration.Milliseconds(),
			"diff_duration_ms":       result.Timings.DiffDuration.Milliseconds(),
			"total_duration_ms":      result.Timings.TotalDuration.Milliseconds(),
		},
	}

	if err := diffUploader.UploadDiff(ctx, runID, stageID, diffBytes, summary); err != nil {
		slog.Error("failed to upload diff", "run_id", runID, "error", err)
		return
	}

	slog.Info("diff uploaded successfully", "run_id", runID, "size", len(diffBytes))

	// Also upload diff as artifact bundle named "diff" for client download.
	r.uploadDiffArtifact(ctx, runID, stageID, diffBytes)
}

// uploadDiffArtifact uploads the diff as an artifact bundle for client download.
// Creates a temporary file, writes the diff content, and uploads as "diff" artifact.
func (r *runController) uploadDiffArtifact(ctx context.Context, runID, stageID string, diffBytes []byte) {
	diffFile, err := os.CreateTemp("", "ploy-diff-*.patch")
	if err != nil {
		return
	}
	defer func() { _ = os.Remove(diffFile.Name()) }()

	_, _ = diffFile.Write(diffBytes)
	_ = diffFile.Close()

	artUploader, err := NewArtifactUploader(r.cfg)
	if err != nil {
		return
	}

	if _, _, errU := artUploader.UploadArtifact(ctx, runID, stageID, []string{diffFile.Name()}, "diff"); errU != nil {
		slog.Warn("failed to upload diff artifact bundle", "run_id", runID, "error", errU)
	} else {
		slog.Info("diff artifact bundle uploaded", "run_id", runID)
	}
}

// uploadConfiguredArtifacts uploads artifact bundles specified in the artifact_paths option.
// It resolves paths relative to the workspace, validates they exist, and bundles them
// with the configured artifact_name. Paths are accepted as either []any (from JSON deserialization)
// or []string (from programmatic callers).
//
// Missing paths generate warnings but do not fail the upload operation.
func (r *runController) uploadConfiguredArtifacts(ctx context.Context, req StartRunRequest, manifest contracts.StepManifest, workspace string) {
	// Accept either []any (from JSON) or []string (programmatic callers).
	var paths []string
	switch v := req.Options["artifact_paths"].(type) {
	case []any:
		for _, p := range v {
			if s, ok := p.(string); ok && s != "" {
				fullPath := filepath.Join(workspace, s)
				if _, err := os.Stat(fullPath); err == nil {
					paths = append(paths, fullPath)
				} else {
					slog.Warn("artifact path not found", "run_id", req.RunID, "path", s)
				}
			}
		}
	case []string:
		for _, s := range v {
			if strings.TrimSpace(s) == "" {
				continue
			}
			fullPath := filepath.Join(workspace, s)
			if _, err := os.Stat(fullPath); err == nil {
				paths = append(paths, fullPath)
			} else {
				slog.Warn("artifact path not found", "run_id", req.RunID, "path", s)
			}
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

	stageID, _ := manifest.OptionString("stage_id")
	artifactName, _ := manifest.OptionString("artifact_name")

	if _, _, err := artifactUploader.UploadArtifact(ctx, req.RunID.String(), stageID, paths, artifactName); err != nil {
		slog.Error("failed to upload artifact bundle", "run_id", req.RunID, "error", err)
	} else {
		slog.Info("artifact bundle uploaded successfully", "run_id", req.RunID, "paths", len(paths))
	}
}

// uploadOutDir bundles and uploads the /out directory when it contains files.
// The bundle is named "mod-out" for consistency with client expectations.
func (r *runController) uploadOutDir(ctx context.Context, runID, stageID, outDir string) error {
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

	if _, _, err := artifactUploader.UploadArtifact(ctx, runID, stageID, files, "mod-out"); err != nil {
		return fmt.Errorf("upload /out bundle: %w", err)
	}

	return nil
}

// uploadStatus uploads terminal status and execution statistics to the control plane.
// It uses a short, detached context to ensure the status is reported even if the
// run context is cancelled. Retry logic is handled by StatusUploader.
func (r *runController) uploadStatus(ctx context.Context, runID, status string, reason *string, stats types.RunStats) error {
	statusUploader, err := NewStatusUploader(r.cfg)
	if err != nil {
		return fmt.Errorf("create status uploader: %w", err)
	}

	// Use a short, detached context to attempt status upload even if run context is cancelled.
	// The 10-second timeout provides sufficient time for retries while preventing indefinite hangs.
	statusCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if uploadErr := statusUploader.UploadStatus(statusCtx, runID, status, reason, stats); uploadErr != nil {
		return fmt.Errorf("upload status: %w", uploadErr)
	}

	slog.Info("terminal status uploaded successfully", "run_id", runID, "status", status)
	return nil
}

// uploadGateLogsArtifact uploads build gate logs as an artifact bundle and attaches
// artifact IDs to the gate stats payload. The artifact name includes a suffix to
// distinguish pre-gate, re-gate, and final gate runs (e.g., "build-gate-pre.log").
//
// This allows clients to download detailed gate execution logs for debugging.
func (r *runController) uploadGateLogsArtifact(runID, stageID, logsText, artifactNameSuffix string, gateStats map[string]any) {
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

	// Stage ID is not available here; use empty string (server will handle).
	artifactName := "build-gate.log"
	if artifactNameSuffix != "" {
		artifactName = "build-gate-" + artifactNameSuffix + ".log"
	}

	// Upload with background context to ensure logs are uploaded even if run context is cancelled.
	if id, cid, uerr := artUploader.UploadArtifact(context.Background(), runID, stageID, []string{logFile.Name()}, artifactName); uerr == nil {
		gateStats["logs_artifact_id"] = id
		gateStats["logs_bundle_cid"] = cid
	} else {
		slog.Warn("failed to upload "+artifactName, "run_id", runID, "error", uerr)
	}
}
