package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
)

// claimRunHandler allows nodes to claim a queued run or run step for execution.
// Returns the assigned run/step or 204 No Content if no work is available.
//
// Claim strategy:
//  1. Try to claim a queued step from a multi-step run (run_steps table).
//  2. If no steps available, try to claim a whole run (legacy single-step runs).
//  3. Return run metadata with optional step_index for multi-step execution.
func claimRunHandler(st store.Store, configHolder *ConfigHolder) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Extract node id from path parameter.
		nodeIDStr := r.PathValue("id")
		if strings.TrimSpace(nodeIDStr) == "" {
			http.Error(w, "id path parameter is required", http.StatusBadRequest)
			return
		}

		// Parse and validate node_id.
		nodeID := domaintypes.ToPGUUID(nodeIDStr)
		if !nodeID.Valid {
			http.Error(w, "invalid id: invalid uuid", http.StatusBadRequest)
			return
		}

		var err error
		// Verify node exists before attempting to claim work.
		_, err = st.GetNode(r.Context(), nodeID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				http.Error(w, "node not found", http.StatusNotFound)
				return
			}
			http.Error(w, fmt.Sprintf("failed to check node: %v", err), http.StatusInternalServerError)
			slog.Error("claim: node check failed", "node_id", nodeIDStr, "err", err)
			return
		}

		// Strategy 1: Try to claim a step from a multi-step run.
		// This enables multiple nodes to execute distinct steps of the same run.
		step, stepErr := st.ClaimRunStep(r.Context(), nodeID)
		if stepErr == nil {
			// Step claimed successfully; fetch parent run metadata and return.
			run, runErr := st.GetRun(r.Context(), step.RunID)
			if runErr != nil {
				http.Error(w, fmt.Sprintf("failed to get run for claimed step: %v", runErr), http.StatusInternalServerError)
				slog.Error("claim: get run failed for step", "node_id", nodeIDStr, "step_id", uuid.UUID(step.ID.Bytes).String(), "err", runErr)
				return
			}
			// Build and send response with step information included.
			// For step-level claims, pass the step so the response uses the step's node_id.
			buildAndSendClaimResponse(w, r, st, configHolder, run, &step)
			slog.Info("step claimed",
				"step_id", uuid.UUID(step.ID.Bytes).String(),
				"run_id", uuid.UUID(run.ID.Bytes).String(),
				"step_index", step.StepIndex,
				"node_id", nodeIDStr,
			)
			return
		}

		// If no step claimed (either no steps available or error), log and continue to run claim.
		if !errors.Is(stepErr, pgx.ErrNoRows) {
			slog.Warn("claim: step claim failed, trying run claim", "node_id", nodeIDStr, "err", stepErr)
		}

		// Strategy 2: Try to claim a whole run (legacy single-step runs or runs without steps).
		run, err := st.ClaimRun(r.Context(), nodeID)
		if err != nil {
			// No queued runs or steps available; return 204 No Content.
			if errors.Is(err, pgx.ErrNoRows) {
				w.WriteHeader(http.StatusNoContent)
				slog.Debug("claim: no work available (no steps or runs)", "node_id", nodeIDStr)
				return
			}
			http.Error(w, fmt.Sprintf("failed to claim run: %v", err), http.StatusInternalServerError)
			slog.Error("claim: database error", "node_id", nodeIDStr, "err", err)
			return
		}

		// Build and send response for whole run claim (no step_index).
		buildAndSendClaimResponse(w, r, st, configHolder, run, nil)
		slog.Info("run claimed",
			"run_id", uuid.UUID(run.ID.Bytes).String(),
			"node_id", nodeIDStr,
			"repo_url", run.RepoUrl,
			"status", run.Status,
		)
	}
}

// buildAndSendClaimResponse constructs and sends the claim response for a run or step.
// If claimedStep is non-nil, includes step_index in the response and uses the step's node_id
// for multi-step execution. Otherwise, uses the run's node_id for whole-run claims.
func buildAndSendClaimResponse(
	w http.ResponseWriter,
	r *http.Request,
	st store.Store,
	configHolder *ConfigHolder,
	run store.Run,
	claimedStep *store.RunStep,
) {
	// Determine or create a stage for this run and merge stage_id into spec.
	stagesForRun, err := st.ListStagesByRun(r.Context(), run.ID)
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to list stages: %v", err), http.StatusInternalServerError)
		slog.Error("claim: list stages failed", "run_id", uuid.UUID(run.ID.Bytes).String(), "err", err)
		return
	}
	var stageIDStr string
	if len(stagesForRun) > 0 {
		stageIDStr = uuid.UUID(stagesForRun[0].ID.Bytes).String()
	} else {
		stg, err := st.CreateStage(r.Context(), store.CreateStageParams{RunID: run.ID, Name: "mods-openrewrite", Status: store.StageStatusPending, Meta: []byte("{}")})
		if err != nil {
			http.Error(w, fmt.Sprintf("failed to create stage: %v", err), http.StatusInternalServerError)
			slog.Error("claim: create stage failed", "run_id", uuid.UUID(run.ID.Bytes).String(), "err", err)
			return
		}
		stageIDStr = uuid.UUID(stg.ID.Bytes).String()
	}
	mergedSpec := mergeStageIDIntoSpec(run.Spec, stageIDStr)

	// Merge server default GitLab config (token/domain) into spec if configured.
	// Per-run overrides (already in spec) take precedence over server defaults.
	gitlabCfg := configHolder.GetGitLab()
	mergedSpec = mergeGitLabConfigIntoSpec(mergedSpec, gitlabCfg)

	// Build response with claimed run details.
	// Include step_index if this is a step claim (multi-node execution).
	// For step claims, use the step's node_id; for whole run claims, use the run's node_id.
	// Domain types ensure VCS fields have been validated at ingestion.
	// Use typed status (store.RunStatus) instead of string cast for type safety;
	// JSON encoder will serialize the underlying string value.
	var nodeIDStr string
	var stepIndex *int32
	if claimedStep != nil {
		// Step-level claim: use step's node_id and include step_index.
		nodeIDStr = uuid.UUID(claimedStep.NodeID.Bytes).String()
		stepIndex = &claimedStep.StepIndex
	} else {
		// Whole-run claim: use run's node_id, step_index remains nil.
		nodeIDStr = uuid.UUID(run.NodeID.Bytes).String()
	}

	resp := struct {
		ID        string          `json:"id"`
		RepoURL   string          `json:"repo_url"`
		Status    store.RunStatus `json:"status"` // Typed status instead of string cast
		NodeID    string          `json:"node_id"`
		BaseRef   string          `json:"base_ref"`
		TargetRef string          `json:"target_ref"`
		CommitSha *string         `json:"commit_sha,omitempty"`
		StepIndex *int32          `json:"step_index,omitempty"` // Present for multi-step execution
		StartedAt string          `json:"started_at"`
		CreatedAt string          `json:"created_at"`
		Spec      json.RawMessage `json:"spec,omitempty"`
	}{
		ID:        uuid.UUID(run.ID.Bytes).String(),
		RepoURL:   run.RepoUrl,
		Status:    run.Status,
		NodeID:    nodeIDStr,
		BaseRef:   run.BaseRef,
		TargetRef: run.TargetRef,
		CommitSha: run.CommitSha,
		StepIndex: stepIndex, // Nullable: present for step claims, nil for whole run claims
		// Use RFC3339 for consistency with other API responses.
		StartedAt: run.StartedAt.Time.Format(time.RFC3339),
		CreatedAt: run.CreatedAt.Time.Format(time.RFC3339),
		Spec:      mergedSpec,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		slog.Error("claim: encode response failed", "err", err)
	}
}
