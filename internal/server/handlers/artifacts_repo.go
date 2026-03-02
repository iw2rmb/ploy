package handlers

import (
	"encoding/json"
	"errors"
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
		runID, err := parseParam[domaintypes.RunID](r, "run_id")
		if err != nil {
			httpErr(w, http.StatusBadRequest, "%s", err)
			return
		}
		repoID, err := parseParam[domaintypes.RepoID](r, "repo_id")
		if err != nil {
			httpErr(w, http.StatusBadRequest, "%s", err)
			return
		}

		rr, err := st.GetRunRepo(r.Context(), store.GetRunRepoParams{RunID: runID, RepoID: repoID})
		if err != nil {
			switch {
			case errors.Is(err, pgx.ErrNoRows):
				httpErr(w, http.StatusNotFound, "repo not found")
			default:
				httpErr(w, http.StatusInternalServerError, "failed to get repo: %v", err)
				slog.Error("list run repo artifacts: get repo failed", "run_id", runID.String(), "repo_id", repoID.String(), "err", err)
			}
			return
		}

		jobs, err := st.ListJobsByRunRepoAttempt(r.Context(), store.ListJobsByRunRepoAttemptParams{
			RunID:   runID,
			RepoID:  repoID,
			Attempt: rr.Attempt,
		})
		if err != nil {
			httpErr(w, http.StatusInternalServerError, "failed to list jobs: %v", err)
			slog.Error("list run repo artifacts: list jobs failed", "run_id", runID.String(), "repo_id", repoID.String(), "attempt", rr.Attempt, "err", err)
			return
		}

		jobOrderByID := deriveJobOrderByChain(jobs)

		// Fetch artifact bundle metadata only (exclude bundle bytes).
		bundles, err := st.ListArtifactBundlesMetaByRun(r.Context(), runID)
		if err != nil {
			httpErr(w, http.StatusInternalServerError, "failed to list artifacts: %v", err)
			slog.Error("list run repo artifacts: list bundles failed", "run_id", runID.String(), "repo_id", repoID.String(), "err", err)
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
			order     int
			createdAt int64
		}

		artifacts := make([]artifactRow, 0, len(bundles))
		for _, bundle := range bundles {
			if bundle.JobID == nil || bundle.JobID.IsZero() {
				continue
			}
			order, ok := jobOrderByID[bundle.JobID.String()]
			if !ok {
				continue
			}

			id := ""
			if bundle.ID.Valid {
				id = uuid.UUID(bundle.ID.Bytes).String()
			}

			summary := artifactSummary{
				ID:   id,
				Size: bundle.BundleSize,
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
				order:     order,
				createdAt: createdAt,
			})
		}

		sort.SliceStable(artifacts, func(i, j int) bool {
			if artifacts[i].order != artifacts[j].order {
				return artifacts[i].order < artifacts[j].order
			}
			return artifacts[i].createdAt < artifacts[j].createdAt
		})

		out := make([]artifactSummary, 0, len(artifacts))
		for _, row := range artifacts {
			out = append(out, row.summary)
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(struct {
			Artifacts []artifactSummary `json:"artifacts"`
		}{
			Artifacts: out,
		})
	}
}

func deriveJobOrderByChain(jobs []store.Job) map[string]int {
	orderByID := make(map[string]int, len(jobs))
	if len(jobs) == 0 {
		return orderByID
	}

	jobByID := make(map[string]store.Job, len(jobs))
	inDegree := make(map[string]int, len(jobs))
	for _, job := range jobs {
		id := job.ID.String()
		jobByID[id] = job
		inDegree[id] = 0
	}
	for _, job := range jobs {
		if job.NextID == nil || job.NextID.IsZero() {
			continue
		}
		nextID := job.NextID.String()
		if _, ok := inDegree[nextID]; ok {
			inDegree[nextID]++
		}
	}

	heads := make([]store.Job, 0, len(jobs))
	for _, job := range jobs {
		if inDegree[job.ID.String()] == 0 {
			heads = append(heads, job)
		}
	}
	sort.SliceStable(heads, func(i, j int) bool {
		return heads[i].ID.String() < heads[j].ID.String()
	})

	visited := make(map[string]bool, len(jobs))
	order := 0
	for _, head := range heads {
		current := head
		for {
			id := current.ID.String()
			if visited[id] {
				break
			}
			visited[id] = true
			orderByID[id] = order
			order++

			if current.NextID == nil || current.NextID.IsZero() {
				break
			}
			next, ok := jobByID[current.NextID.String()]
			if !ok {
				break
			}
			current = next
		}
	}

	remaining := make([]string, 0, len(jobs))
	for _, job := range jobs {
		id := job.ID.String()
		if !visited[id] {
			remaining = append(remaining, id)
		}
	}
	sort.Strings(remaining)
	for _, id := range remaining {
		orderByID[id] = order
		order++
	}

	return orderByID
}
