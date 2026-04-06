package nodeagent

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	types "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

type workspaceChangePolicy string

const (
	workspaceChangePolicyIgnore  workspaceChangePolicy = "ignore"
	workspaceChangePolicyRequire workspaceChangePolicy = "require_changes"
	workspaceChangePolicyForbid  workspaceChangePolicy = "forbid_changes"
)

func resolveHealingWorkspacePolicy(recoveryCtx *contracts.RecoveryClaimContext) workspaceChangePolicy {
	if recoveryCtx == nil {
		return workspaceChangePolicyRequire
	}
	if strings.TrimSpace(recoveryCtx.GateProfileSchemaJSON) != "" {
		return workspaceChangePolicyForbid
	}
	return workspaceChangePolicyRequire
}

func validateWorkspacePolicy(policy workspaceChangePolicy, preStatus, postStatus string) (warning string, violated bool) {
	switch policy {
	case workspaceChangePolicyRequire:
		if postStatus == preStatus {
			return "no_workspace_changes", true
		}
	case workspaceChangePolicyForbid:
		if postStatus != preStatus {
			return "unexpected_workspace_changes", true
		}
	}
	return "", false
}

func (r *runController) uploadHealingWorkspacePolicyFailure(ctx context.Context, req StartRunRequest, warning string, duration time.Duration) {
	logMsg := fmt.Sprintf("healing job failed (%s)", warning)
	stats := types.NewRunStatsBuilder().
		ExitCode(1).
		DurationMs(duration.Milliseconds()).
		HealingWarning(warning).
		MustBuild()

	// v1 uses capitalized job status values: Success, Fail, Cancelled.
	var exitCodeOne int32 = 1
	if uploadErr := r.uploadStatus(ctx, req.RunID.String(), types.JobStatusFail.String(), &exitCodeOne, stats, req.JobID); uploadErr != nil {
		slog.Error("failed to upload healing failure status", "run_id", req.RunID, "job_id", req.JobID, "warning", warning, "error", uploadErr)
	}
	slog.Info(logMsg, "run_id", req.RunID, "job_id", req.JobID, "exit_code", 1, "duration", duration)
}

// populateHealingInDir copies recovery context into the healing job /in directory.
// It requires recovery_context.build_gate_log and, for infra healing, writes
// gate_profile.schema.json from server-injected schema env and hydrates
// gate_profile.json when a run-local snapshot is available.
func (r *runController) populateHealingInDir(
	runID types.RunID,
	inDir string,
	recoveryCtx *contracts.RecoveryClaimContext,
	schemaJSON string,
) error {
	if strings.TrimSpace(inDir) == "" {
		return nil
	}

	cacheDir := runCacheDir(runID)
	if recoveryCtx == nil || strings.TrimSpace(recoveryCtx.BuildGateLog) == "" {
		return fmt.Errorf("missing recovery_context.build_gate_log for healing job")
	}
	destPath := filepath.Join(inDir, "build-gate.log")
	if err := os.WriteFile(destPath, []byte(recoveryCtx.BuildGateLog), 0o644); err != nil {
		return fmt.Errorf("write /in/build-gate.log: %w", err)
	}
	slog.Info("hydrated /in/build-gate.log for healing job", "run_id", runID, "path", destPath)

	// Hydrate deps healing inputs when recovery context carries deps state.
	if recoveryCtx != nil {
		if recoveryCtx.DepsBumps != nil {
			depsBumpsRaw, err := json.Marshal(recoveryCtx.DepsBumps)
			if err != nil {
				return fmt.Errorf("marshal recovery_context.deps_bumps: %w", err)
			}
			inDepsBumpsPath := filepath.Join(inDir, "deps-bumps.json")
			if err := os.WriteFile(inDepsBumpsPath, depsBumpsRaw, 0o644); err != nil {
				return fmt.Errorf("write /in/deps-bumps.json: %w", err)
			}
			slog.Info("hydrated /in/deps-bumps.json for deps healing job", "run_id", runID, "path", inDepsBumpsPath)
		}
		if endpoint := strings.TrimSpace(recoveryCtx.DepsCompatEndpoint); endpoint != "" {
			inDepsCompatPath := filepath.Join(inDir, "deps-compat-url.txt")
			if err := os.WriteFile(inDepsCompatPath, []byte(endpoint), 0o644); err != nil {
				return fmt.Errorf("write /in/deps-compat-url.txt: %w", err)
			}
			slog.Info("hydrated /in/deps-compat-url.txt for deps healing job", "run_id", runID, "path", inDepsCompatPath)
		}
	}

	// Hydrate infra healing inputs when schema JSON is available.
	effectiveSchema := schemaJSON
	if recoveryCtx != nil && strings.TrimSpace(recoveryCtx.GateProfileSchemaJSON) != "" {
		effectiveSchema = recoveryCtx.GateProfileSchemaJSON
	}
	if strings.TrimSpace(effectiveSchema) == "" {
		return nil
	}
	schemaRaw := []byte(effectiveSchema)
	if !json.Valid(schemaRaw) {
		return fmt.Errorf("infra healing schema env %s is not valid JSON", contracts.GateProfileSchemaJSONEnv)
	}
	inSchemaPath := filepath.Join(inDir, "gate_profile.schema.json")
	if err := os.WriteFile(inSchemaPath, schemaRaw, 0o644); err != nil {
		return fmt.Errorf("write /in/gate_profile.schema.json: %w", err)
	}
	slog.Info("hydrated /in/gate_profile.schema.json for healing job", "run_id", runID, "path", inSchemaPath)

	var profileRaw []byte
	var err error
	if recoveryCtx != nil && len(recoveryCtx.GateProfile) > 0 {
		profileRaw = append([]byte(nil), recoveryCtx.GateProfile...)
	} else {
		profilePath := filepath.Join(cacheDir, "build-gate-profile.json")
		profileRaw, err = os.ReadFile(profilePath)
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return fmt.Errorf("read gate profile snapshot: %w", err)
		}
	}
	if len(strings.TrimSpace(string(profileRaw))) == 0 {
		return nil
	}
	if _, err := contracts.ParseGateProfileJSON(profileRaw); err != nil {
		return fmt.Errorf("parse gate profile snapshot: %w", err)
	}
	inProfilePath := filepath.Join(inDir, "gate_profile.json")
	if err := os.WriteFile(inProfilePath, profileRaw, 0o644); err != nil {
		return fmt.Errorf("write /in/gate_profile.json: %w", err)
	}
	slog.Info("hydrated /in/gate_profile.json for healing job", "run_id", runID, "path", inProfilePath)

	return nil
}
