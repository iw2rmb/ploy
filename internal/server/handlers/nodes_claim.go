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
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/iw2rmb/ploy/internal/store"
)

// claimRunHandler allows nodes to claim a queued run for execution.
// Returns the assigned run or 204 No Content if no runs are available.
func claimRunHandler(st store.Store, configHolder *ConfigHolder) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Extract node id from path parameter.
		nodeIDStr := r.PathValue("id")
		if strings.TrimSpace(nodeIDStr) == "" {
			http.Error(w, "id path parameter is required", http.StatusBadRequest)
			return
		}

		// Parse and validate node_id.
		nodeUUID, err := uuid.Parse(nodeIDStr)
		if err != nil {
			http.Error(w, fmt.Sprintf("invalid id: %v", err), http.StatusBadRequest)
			return
		}

		// Verify node exists before attempting to claim a run.
		_, err = st.GetNode(r.Context(), pgtype.UUID{
			Bytes: nodeUUID,
			Valid: true,
		})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				http.Error(w, "node not found", http.StatusNotFound)
				return
			}
			http.Error(w, fmt.Sprintf("failed to check node: %v", err), http.StatusInternalServerError)
			slog.Error("claim run: node check failed", "node_id", nodeIDStr, "err", err)
			return
		}

		// Attempt to claim a run using FOR UPDATE SKIP LOCKED.
		run, err := st.ClaimRun(r.Context(), pgtype.UUID{
			Bytes: nodeUUID,
			Valid: true,
		})
		if err != nil {
			// No queued runs available is a valid state; return 204 No Content.
			if errors.Is(err, pgx.ErrNoRows) {
				w.WriteHeader(http.StatusNoContent)
				slog.Debug("claim run: no queued runs available", "node_id", nodeIDStr)
				return
			}
			http.Error(w, fmt.Sprintf("failed to claim run: %v", err), http.StatusInternalServerError)
			slog.Error("claim run: database error", "node_id", nodeIDStr, "err", err)
			return
		}

		// Determine or create a stage for this run and merge stage_id into spec.
		stagesForRun, err := st.ListStagesByRun(r.Context(), run.ID)
		if err != nil {
			http.Error(w, fmt.Sprintf("failed to list stages: %v", err), http.StatusInternalServerError)
			slog.Error("claim run: list stages failed", "run_id", uuid.UUID(run.ID.Bytes).String(), "err", err)
			return
		}
		var stageIDStr string
		if len(stagesForRun) > 0 {
			stageIDStr = uuid.UUID(stagesForRun[0].ID.Bytes).String()
		} else {
			stg, err := st.CreateStage(r.Context(), store.CreateStageParams{RunID: run.ID, Name: "mods-openrewrite", Status: store.StageStatusPending, Meta: []byte("{}")})
			if err != nil {
				http.Error(w, fmt.Sprintf("failed to create stage: %v", err), http.StatusInternalServerError)
				slog.Error("claim run: create stage failed", "run_id", uuid.UUID(run.ID.Bytes).String(), "err", err)
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
		resp := struct {
			ID        string          `json:"id"`
			RepoURL   string          `json:"repo_url"`
			Status    string          `json:"status"`
			NodeID    string          `json:"node_id"`
			BaseRef   string          `json:"base_ref"`
			TargetRef string          `json:"target_ref"`
			CommitSha *string         `json:"commit_sha,omitempty"`
			StartedAt string          `json:"started_at"`
			CreatedAt string          `json:"created_at"`
			Spec      json.RawMessage `json:"spec,omitempty"`
		}{
			ID:        uuid.UUID(run.ID.Bytes).String(),
			RepoURL:   run.RepoUrl,
			Status:    string(run.Status),
			NodeID:    uuid.UUID(run.NodeID.Bytes).String(),
			BaseRef:   run.BaseRef,
			TargetRef: run.TargetRef,
			CommitSha: run.CommitSha,
			// Use RFC3339 for consistency with other API responses.
			StartedAt: run.StartedAt.Time.Format(time.RFC3339),
			CreatedAt: run.CreatedAt.Time.Format(time.RFC3339),
			Spec:      mergedSpec,
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			slog.Error("claim run: encode response failed", "err", err)
		}

		slog.Info("run claimed",
			"run_id", resp.ID,
			"node_id", nodeIDStr,
			"repo_url", resp.RepoURL,
			"status", resp.Status,
		)
	}
}
