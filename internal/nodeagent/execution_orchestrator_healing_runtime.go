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

func resolveHealingWorkspacePolicy(healingSpec *contracts.HealingSpec) workspaceChangePolicy {
	if healingSpec == nil {
		return workspaceChangePolicyRequire
	}
	if strings.TrimSpace(healingSpec.SelectedErrorKind) == "infra" {
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

// uploadHealingNoWorkspaceChangesFailure uploads a terminal failure status when a healing job
// exits 0 but produces no workspace changes.
func (r *runController) uploadHealingNoWorkspaceChangesFailure(ctx context.Context, req StartRunRequest, baseStats types.RunStats, duration time.Duration) {
	r.uploadHealingWorkspacePolicyFailure(ctx, req, "no_workspace_changes", duration)
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
	if uploadErr := r.uploadStatus(ctx, req.RunID.String(), JobStatusFail.String(), &exitCodeOne, stats, req.JobID); uploadErr != nil {
		slog.Error("failed to upload healing failure status", "run_id", req.RunID, "job_id", req.JobID, "warning", warning, "error", uploadErr)
	}
	slog.Info(logMsg, "run_id", req.RunID, "job_id", req.JobID, "exit_code", 1, "duration", duration)
}

// populateHealingInDir copies recovery context into the healing job /in directory.
// It attempts to hydrate build-gate.log when available and, for infra healing,
// writes gate_profile.schema.json from server-injected schema env and
// hydrates gate_profile.json when a run-local snapshot is available.
func (r *runController) populateHealingInDir(
	runID types.RunID,
	inDir string,
	healingSpec *contracts.HealingSpec,
	recoveryCtx *contracts.RecoveryClaimContext,
	schemaJSON string,
) error {
	if strings.TrimSpace(inDir) == "" {
		return nil
	}

	baseRoot := os.Getenv("PLOYD_CACHE_HOME")
	if baseRoot == "" {
		baseRoot = os.TempDir()
	}
	runDir := filepath.Join(baseRoot, "ploy", "run", runID.String())
	srcPath := filepath.Join(runDir, "build-gate-first.log")

	gateLogAvailable := false
	var data []byte
	if recoveryCtx != nil && strings.TrimSpace(recoveryCtx.BuildGateLog) != "" {
		data = []byte(recoveryCtx.BuildGateLog)
		gateLogAvailable = true
	} else {
		var err error
		data, err = os.ReadFile(srcPath)
		if err != nil {
			if os.IsNotExist(err) {
				slog.Warn("missing run-local build-gate log snapshot for healing job", "run_id", runID, "path", srcPath)
			} else {
				return fmt.Errorf("read first gate log: %w", err)
			}
		} else if len(strings.TrimSpace(string(data))) == 0 {
			slog.Warn("empty run-local build-gate log snapshot for healing job", "run_id", runID, "path", srcPath)
		} else {
			gateLogAvailable = true
		}
	}

	if gateLogAvailable {
		destPath := filepath.Join(inDir, "build-gate.log")
		if err := os.WriteFile(destPath, data, 0o644); err != nil {
			return fmt.Errorf("write /in/build-gate.log: %w", err)
		}
		slog.Info("hydrated /in/build-gate.log for healing job", "run_id", runID, "path", destPath)
	}

	if healingSpec != nil && strings.TrimSpace(healingSpec.SelectedErrorKind) == "infra" {
		effectiveSchema := schemaJSON
		if recoveryCtx != nil && strings.TrimSpace(recoveryCtx.GateProfileSchemaJSON) != "" {
			effectiveSchema = recoveryCtx.GateProfileSchemaJSON
		}
		if strings.TrimSpace(effectiveSchema) == "" {
			return fmt.Errorf("infra healing requires %s env", contracts.GateProfileSchemaJSONEnv)
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
			profilePath := filepath.Join(runDir, "build-gate-profile.json")
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
	}

	return nil
}
