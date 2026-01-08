package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sort"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
)

// listRunRepoArtifactsHandler lists artifact bundles produced by jobs belonging to a
// specific repo execution within a run.
// GET /v1/runs/{run_id}/repos/{repo_id}/artifacts
func listRunRepoArtifactsHandler(st store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		runIDStr, err := requiredPathParam(r, "run_id")
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		repoIDStr, err := requiredPathParam(r, "repo_id")
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		runID := domaintypes.RunID(runIDStr)
		repoID := domaintypes.ModRepoID(repoIDStr)

		rr, err := st.GetRunRepo(r.Context(), store.GetRunRepoParams{RunID: runID, RepoID: repoID})
		if err != nil {
			switch {
			case errors.Is(err, pgx.ErrNoRows):
				http.Error(w, "repo not found", http.StatusNotFound)
			default:
				http.Error(w, fmt.Sprintf("failed to get repo: %v", err), http.StatusInternalServerError)
				slog.Error("list run repo artifacts: get repo failed", "run_id", runIDStr, "repo_id", repoIDStr, "err", err)
			}
			return
		}

		jobs, err := st.ListJobsByRunRepoAttempt(r.Context(), store.ListJobsByRunRepoAttemptParams{
			RunID:   runID,
			RepoID:  repoID,
			Attempt: rr.Attempt,
		})
		if err != nil {
			http.Error(w, fmt.Sprintf("failed to list jobs: %v", err), http.StatusInternalServerError)
			slog.Error("list run repo artifacts: list jobs failed", "run_id", runIDStr, "repo_id", repoIDStr, "attempt", rr.Attempt, "err", err)
			return
		}

		jobStepIndex := make(map[string]domaintypes.StepIndex, len(jobs))
		for _, job := range jobs {
			jobStepIndex[job.ID.String()] = job.StepIndex
		}

		bundles, err := st.ListArtifactBundlesByRun(r.Context(), runID)
		if err != nil {
			http.Error(w, fmt.Sprintf("failed to list artifacts: %v", err), http.StatusInternalServerError)
			slog.Error("list run repo artifacts: list bundles failed", "run_id", runIDStr, "repo_id", repoIDStr, "err", err)
			return
		}

		type artifactSummary struct {
			ID     string  `json:"id"`
			CID    string  `json:"cid"`
			Digest string  `json:"digest"`
			Name   *string `json:"name,omitempty"`
			Size   int64   `json:"size"`
		}
		type artifactRow struct {
			summary   artifactSummary
			stepIndex domaintypes.StepIndex
			createdAt int64
		}

		artifacts := make([]artifactRow, 0, len(bundles))
		for _, bundle := range bundles {
			if bundle.JobID == nil || bundle.JobID.IsZero() {
				continue
			}
			stepIndex, ok := jobStepIndex[bundle.JobID.String()]
			if !ok {
				continue
			}

			id := ""
			if bundle.ID.Valid {
				id = uuid.UUID(bundle.ID.Bytes).String()
			}

			summary := artifactSummary{
				ID:   id,
				Size: int64(len(bundle.Bundle)),
			}
			if bundle.Cid != nil {
				summary.CID = *bundle.Cid
			}
			if bundle.Digest != nil {
				summary.Digest = *bundle.Digest
			}
			if bundle.Name != nil {
				summary.Name = bundle.Name
			}

			createdAt := int64(0)
			if bundle.CreatedAt.Valid {
				createdAt = bundle.CreatedAt.Time.UnixNano()
			}

			artifacts = append(artifacts, artifactRow{
				summary:   summary,
				stepIndex: stepIndex,
				createdAt: createdAt,
			})
		}

		sort.SliceStable(artifacts, func(i, j int) bool {
			if artifacts[i].stepIndex != artifacts[j].stepIndex {
				return artifacts[i].stepIndex < artifacts[j].stepIndex
			}
			return artifacts[i].createdAt < artifacts[j].createdAt
		})

		out := make([]artifactSummary, 0, len(artifacts))
		for _, row := range artifacts {
			out = append(out, row.summary)
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{"artifacts": out})
	}
}
