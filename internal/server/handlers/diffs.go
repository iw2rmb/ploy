package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sort"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/iw2rmb/ploy/internal/blobstore"
	domainapi "github.com/iw2rmb/ploy/internal/domain/api"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
)

const maxAccumulatedDiffPlainBytes int64 = 64 << 20

// listRunDiffsHandler returns a JSON list of diffs for a specific run.
//
// GET /v1/runs/{run_id}/diffs
//
// Download mode:
// - GET /v1/runs/{run_id}/diffs?download=true&diff_id=<uuid>
// - Returns the gzipped patch bytes for the requested diff, streamed from object storage.
//
// Run and job IDs are KSUID-backed strings.
func listRunDiffsHandler(st store.Store, bs blobstore.Store, snapshots repoSnapshotWriter) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		runID, ok := parseRequiredPathIDOrWriteError[domaintypes.RunID](w, r, "run_id")
		if !ok {
			return
		}
		run, ok := getRunOrFail(w, r, st, runID, "list run diffs")
		if !ok {
			return
		}

		// Optional download mode: serve a specific gzipped patch artifact.
		// Accumulated exports replay committed history against the source snapshot.
		if r.URL.Query().Get("download") == "true" {
			diffID, ok := parseRequiredQueryIDOrWriteError[domaintypes.DiffID](w, r, "diff_id")
			if !ok {
				return
			}
			accumulated := strings.EqualFold(strings.TrimSpace(r.URL.Query().Get("accumulated")), "true")
			if accumulated {
				if !downloadAccumulatedRunDiff(w, r, st, bs, snapshots, run, diffID) {
					return
				}
				return
			}
			diffs, err := listEffectiveRunDiffs(r.Context(), st, run)
			if err != nil {
				serverError(w, "download run diff", "list diffs", err, "run_id", runID.String(), "repo_id", run.RepoID.String(), "diff_id", diffID.String())
				return
			}
			diffUUID := uuid.MustParse(diffID.String())
			var d store.Diff
			found := false
			for _, item := range diffs {
				if item.Diff.ID.Valid && item.Diff.ID.Bytes == diffUUID {
					d = item.Diff
					found = true
					break
				}
			}
			if !found {
				writeHTTPError(w, http.StatusNotFound, "diff not found")
				return
			}

			// Stream from object storage.
			rc, size, ok := openBlobForHTTP(w, r, bs, d.ObjectKey, "diff",
				"run_id", runID.String(), "repo_id", run.RepoID.String(), "diff_id", diffID.String())
			if !ok {
				return
			}
			defer rc.Close()

			streamBlob(w, rc, size, fmt.Sprintf("diff-%s.patch.gz", diffUUID.String()), "application/gzip")
			return
		}

		diffs, err := listEffectiveRunDiffs(r.Context(), st, run)
		if err != nil {
			writeHTTPError(w, http.StatusInternalServerError, "failed to list diffs: %v", err)
			slog.Error("list run diffs: query failed", "run_id", runID.String(), "repo_id", run.RepoID.String(), "err", err)
			return
		}

		// Build response items in the canonical list format.
		items := make([]domainapi.DiffListItem, 0, len(diffs))
		for _, row := range diffs {
			d := row.Diff
			var summary domaintypes.DiffSummary
			if len(d.Summary) > 0 {
				_ = json.Unmarshal(d.Summary, &summary)
			}
			items = append(items, domainapi.DiffListItem{
				ID:        domaintypes.DiffID(uuid.UUID(d.ID.Bytes).String()),
				JobID:     row.DisplayJobID,
				CreatedAt: d.CreatedAt.Time,
				Size:      int(d.PatchSize),
				Summary:   summary,
			})
		}

		writeJSON(w, http.StatusOK, domainapi.DiffListResponse{Diffs: items})
	}
}

type runDiffRow struct {
	Diff         store.Diff
	DisplayJobID domaintypes.JobID
}

func listEffectiveRunDiffs(
	ctx context.Context,
	st store.Store,
	run store.Run,
) ([]runDiffRow, error) {
	jobs, err := st.ListJobsByRun(ctx, run.ID)
	if err != nil {
		return nil, err
	}
	if len(jobs) == 0 {
		return []runDiffRow{}, nil
	}

	firstAttempt := replayFirstAttempt(jobs, run.Attempt, run.SourceCommitSha)
	orderByID := deriveJobOrderByChain(jobs)
	sort.SliceStable(jobs, func(i, j int) bool {
		if jobs[i].Attempt != jobs[j].Attempt {
			return jobs[i].Attempt < jobs[j].Attempt
		}
		oi := orderByID[jobs[i].ID.String()]
		oj := orderByID[jobs[j].ID.String()]
		if oi != oj {
			return oi < oj
		}
		return jobs[i].ID.String() < jobs[j].ID.String()
	})

	out := make([]runDiffRow, 0, len(jobs))
	seenDiffIDs := map[string]struct{}{}
	for _, job := range jobs {
		if job.Attempt < firstAttempt || job.Attempt > run.Attempt || (job.Attempt < run.Attempt && job.Status != domaintypes.JobStatusSuccess) {
			continue
		}
		diff, getErr := st.GetLatestDiffByJob(ctx, &job.ID)
		if getErr != nil {
			if errors.Is(getErr, pgx.ErrNoRows) {
				continue
			}
			return nil, getErr
		}
		if job.Attempt < run.Attempt {
			code, ok := domaintypes.DiffSummary(diff.Summary).ExitCode()
			if !ok || code != 0 {
				continue
			}
		}
		diffID := uuid.UUID(diff.ID.Bytes).String()
		if _, exists := seenDiffIDs[diffID]; exists {
			continue
		}
		seenDiffIDs[diffID] = struct{}{}
		out = append(out, runDiffRow{
			Diff:         diff,
			DisplayJobID: job.ID,
		})
	}
	return out, nil
}
