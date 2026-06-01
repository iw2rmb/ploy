package handlers

import (
	"log/slog"
	"net/http"
	"sort"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
)

// listRunRepoArtifactsHandler lists artifact bundles produced by jobs belonging to a
// specific repo execution within a run.
// GET /v1/runs/{run_id}/artifacts
func listRunRepoArtifactsHandler(st store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		runID, ok := parseRequiredPathIDOrWriteError[domaintypes.RunID](w, r, "run_id")
		if !ok {
			return
		}
		repoID, ok := runRepoIDFromPathOrRun(w, r, st, runID)
		if !ok {
			return
		}

		rr, ok := getRunRepoOrFail(w, r, st, runID, repoID, "list run repo artifacts")
		if !ok {
			return
		}

		jobs, ok := listJobsForRunRepoOrFail(w, r, st, runID, repoID, rr.Attempt, "list run repo artifacts")
		if !ok {
			return
		}

		jobOrderByID := deriveJobOrderByChain(jobs)

		type artifactRow struct {
			summary   artifactSummary
			order     int
			createdAt int64
		}

		artifacts := make([]artifactRow, 0, len(jobs))
		seenBundle := map[string]struct{}{}
		for _, job := range jobs {
			order, ok := jobOrderByID[job.ID.String()]
			if !ok {
				continue
			}
			bundles, listErr := listArtifactBundlesByEffectiveJob(r.Context(), st, job)
			if listErr != nil {
				writeHTTPError(w, http.StatusInternalServerError, "failed to list artifacts: %v", listErr)
				slog.Error("list run repo artifacts: list bundles failed", "run_id", runID.String(), "repo_id", repoID.String(), "job_id", job.ID.String(), "err", listErr)
				return
			}
			for _, bundle := range bundles {
				key := bundle.ID.String()
				if key != "" {
					if _, exists := seenBundle[key]; exists {
						continue
					}
					seenBundle[key] = struct{}{}
				}

				createdAt := int64(0)
				if bundle.CreatedAt.Valid {
					createdAt = bundle.CreatedAt.Time.UnixNano()
				}

				artifacts = append(artifacts, artifactRow{
					summary:   bundleToSummary(bundle),
					order:     order,
					createdAt: createdAt,
				})
			}
		}

		if len(artifacts) == 0 {
			writeJSON(w, http.StatusOK, struct {
				Artifacts []artifactSummary `json:"artifacts"`
			}{Artifacts: []artifactSummary{}})
			return
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

		writeJSON(w, http.StatusOK, struct {
			Artifacts []artifactSummary `json:"artifacts"`
		}{Artifacts: out})
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
