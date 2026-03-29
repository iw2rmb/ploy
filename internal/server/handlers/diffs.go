package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/iw2rmb/ploy/internal/blobstore"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
)

// diffItem represents a single diff in a list response.
//
// C2: Each diff is tagged with job_id and job_type (in summary) to enable unified rehydration.
// - job_id: References the job that produced this diff; job's next_id provides ordering.
// - job_type: "mig" for main mig diffs, "healing" for healing diffs (in summary).
// Rehydration queries fetch all diffs ordered by job next_id.
//
// NOTE: job_id is now a KSUID-backed JobID type (no UUID parsing).
type diffItem struct {
	ID        string                  `json:"id"`
	JobID     domaintypes.JobID       `json:"job_id"` // Job ID (KSUID-backed)
	CreatedAt time.Time               `json:"created_at"`
	Size      int                     `json:"gzipped_size"`
	Summary   domaintypes.DiffSummary `json:"summary,omitempty"` // Contains job_type, timings.
}

// diffListResponse is the typed response for listing diffs.
type diffListResponse struct {
	Diffs []diffItem `json:"diffs"`
}

// listRunRepoDiffsHandler returns a JSON list of diffs for a specific repo execution
// within a run. This is the v1 repo-scoped endpoint replacing the legacy run-scoped
// diffs listing endpoint.
//
// GET /v1/runs/{run_id}/repos/{repo_id}/diffs
//
// Download mode:
// - GET /v1/runs/{run_id}/repos/{repo_id}/diffs?download=true&diff_id=<uuid>
// - Returns the gzipped patch bytes for the requested diff, streamed from object storage.
//
// v1 repo-scoped diffs listing:
// - Repo attribution comes from joining diffs.job_id → jobs.repo_id
// - Diffs for repo A are excluded from repo B listing
// - Response shape is unchanged from legacy endpoint (diffListResponse)
//
// Run and job IDs are KSUID-backed strings; repo IDs are NanoID-backed strings.
func listRunRepoDiffsHandler(st store.Store, bs blobstore.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Parse the run ID from the URL path parameter using the shared helper.
		runID, err := parseRequiredPathID[domaintypes.RunID](r, "run_id")
		if err != nil {
			writeHTTPError(w, http.StatusBadRequest, "%s", err)
			return
		}

		// Parse the repo ID from the URL path parameter using the shared helper.
		repoID, err := parseRequiredPathID[domaintypes.RepoID](r, "repo_id")
		if err != nil {
			writeHTTPError(w, http.StatusBadRequest, "%s", err)
			return
		}

		// Optional download mode: serve the gzipped patch for a specific diff.
		if r.URL.Query().Get("download") == "true" {
			diffID, err := parseQuery[domaintypes.DiffID](r, "diff_id")
			if err != nil {
				writeHTTPError(w, http.StatusBadRequest, "%s", err)
				return
			}
			diffUUID := uuid.MustParse(diffID.String())

			d, err := st.GetDiff(r.Context(), pgtype.UUID{Bytes: diffUUID, Valid: true})
			if err != nil {
				if errors.Is(err, pgx.ErrNoRows) {
					writeHTTPError(w, http.StatusNotFound, "diff not found")
					return
				}
				writeHTTPError(w, http.StatusInternalServerError, "failed to get diff: %v", err)
				slog.Error("download run repo diff: get diff failed", "run_id", runID, "repo_id", repoID, "diff_id", diffID.String(), "err", err)
				return
			}
			// Ensure the diff belongs to this run.
			if d.RunID != runID {
				writeHTTPError(w, http.StatusNotFound, "diff not found")
				return
			}
			// Ensure the diff belongs to this repo via job attribution.
			if d.JobID == nil || d.JobID.IsZero() {
				writeHTTPError(w, http.StatusNotFound, "diff not found")
				return
			}
			job, err := st.GetJob(r.Context(), *d.JobID)
			if err != nil {
				if errors.Is(err, pgx.ErrNoRows) {
					writeHTTPError(w, http.StatusNotFound, "diff not found")
					return
				}
				writeHTTPError(w, http.StatusInternalServerError, "failed to get diff job: %v", err)
				slog.Error("download run repo diff: get job failed", "run_id", runID.String(), "repo_id", repoID.String(), "diff_id", diffID.String(), "job_id", d.JobID.String(), "err", err)
				return
			}
			if job.RepoID != repoID {
				writeHTTPError(w, http.StatusNotFound, "diff not found")
				return
			}

			// Stream from object storage.
			rc, size, ok := openBlobForHTTP(w, r, bs, d.ObjectKey, "diff",
				"run_id", runID.String(), "repo_id", repoID.String(), "diff_id", diffID.String())
			if !ok {
				return
			}
			defer rc.Close()

			streamBlob(w, rc, size, fmt.Sprintf("diff-%s.patch.gz", diffUUID.String()), "application/gzip")
			return
		}

		// Query diff metadata filtered by repo attribution.
		diffs, err := st.ListDiffsByRunRepo(r.Context(), store.ListDiffsByRunRepoParams{
			MetadataOnly: false,
			RunID:        runID,
			RepoID:       repoID,
		})
		if err != nil {
			writeHTTPError(w, http.StatusInternalServerError, "failed to list diffs: %v", err)
			slog.Error("list run repo diffs: query failed", "run_id", runID.String(), "repo_id", repoID.String(), "err", err)
			return
		}

		// Build response items in the standard list format (diffListResponse).
		items := make([]diffItem, 0, len(diffs))
		for _, d := range diffs {
			var summary domaintypes.DiffSummary
			if len(d.Summary) > 0 {
				_ = json.Unmarshal(d.Summary, &summary)
			}
			var jobID domaintypes.JobID
			if d.JobID != nil && !d.JobID.IsZero() {
				jobID = *d.JobID
			}
			items = append(items, diffItem{
				ID:        uuid.UUID(d.ID.Bytes).String(), // diffs.id is still UUID
				JobID:     jobID,                          // KSUID-backed domain type
				CreatedAt: d.CreatedAt.Time,
				Size:      int(d.PatchSize),
				Summary:   summary,
			})
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(diffListResponse{Diffs: items})
	}
}
