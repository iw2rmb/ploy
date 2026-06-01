package handlers

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

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
func listRunDiffsHandler(st store.Store, bs blobstore.Store) http.HandlerFunc {
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
		// When accumulated=true, returns a gzipped patch that contains all diffs
		// for this run up to and including diff_id, in list order.
		if r.URL.Query().Get("download") == "true" {
			diffID, ok := parseRequiredQueryIDOrWriteError[domaintypes.DiffID](w, r, "diff_id")
			if !ok {
				return
			}
			accumulated := strings.EqualFold(strings.TrimSpace(r.URL.Query().Get("accumulated")), "true")
			if accumulated {
				if !downloadAccumulatedRunDiff(w, r, st, bs, run, diffID) {
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

		// Build response items in the standard list format (diffListResponse).
		items := make([]diffItem, 0, len(diffs))
		for _, row := range diffs {
			d := row.Diff
			var summary domaintypes.DiffSummary
			if len(d.Summary) > 0 {
				_ = json.Unmarshal(d.Summary, &summary)
			}
			items = append(items, diffItem{
				ID:        uuid.UUID(d.ID.Bytes).String(), // diffs.id is still UUID
				JobID:     row.DisplayJobID,
				CreatedAt: d.CreatedAt.Time,
				Size:      int(d.PatchSize),
				Summary:   summary,
			})
		}

		writeJSON(w, http.StatusOK, diffListResponse{Diffs: items})
	}
}

func downloadAccumulatedRunDiff(
	w http.ResponseWriter,
	r *http.Request,
	st store.Store,
	bs blobstore.Store,
	run store.Run,
	diffID domaintypes.DiffID,
) bool {
	diffs, err := listEffectiveRunDiffs(r.Context(), st, run)
	if err != nil {
		serverError(w, "download accumulated run diff", "list diffs", err, "run_id", run.ID.String(), "repo_id", run.RepoID.String(), "diff_id", diffID.String())
		return false
	}

	diffUUID := uuid.MustParse(diffID.String())
	targetIndex := -1
	for i, item := range diffs {
		if item.Diff.ID.Valid && item.Diff.ID.Bytes == diffUUID {
			targetIndex = i
			break
		}
	}
	if targetIndex < 0 {
		writeHTTPError(w, http.StatusNotFound, "diff not found")
		return false
	}

	var plain bytes.Buffer
	for _, row := range diffs[:targetIndex+1] {
		item := row.Diff
		if item.ObjectKey == nil || strings.TrimSpace(*item.ObjectKey) == "" {
			writeHTTPError(w, http.StatusNotFound, "diff blob not found")
			slog.Error("download accumulated run diff: missing object key", "run_id", run.ID.String(), "repo_id", run.RepoID.String(), "diff_id", diffID.String())
			return false
		}

		rc, _, getErr := bs.Get(r.Context(), *item.ObjectKey)
		if getErr != nil {
			if errors.Is(getErr, blobstore.ErrNotFound) {
				writeHTTPError(w, http.StatusNotFound, "diff blob not found")
				slog.Error("download accumulated run diff: missing blob", "run_id", run.ID.String(), "repo_id", run.RepoID.String(), "diff_id", diffID.String(), "object_key", *item.ObjectKey)
				return false
			}
			writeHTTPError(w, http.StatusServiceUnavailable, "failed to retrieve diff blob")
			slog.Error("download accumulated run diff: get blob failed", "run_id", run.ID.String(), "repo_id", run.RepoID.String(), "diff_id", diffID.String(), "object_key", *item.ObjectKey, "err", getErr)
			return false
		}

		zr, zerr := gzip.NewReader(rc)
		if zerr != nil {
			_ = rc.Close()
			writeHTTPError(w, http.StatusInternalServerError, "failed to read diff blob")
			slog.Error("download accumulated run diff: open gzip reader failed", "run_id", run.ID.String(), "repo_id", run.RepoID.String(), "diff_id", diffID.String(), "object_key", *item.ObjectKey, "err", zerr)
			return false
		}

		remaining := maxAccumulatedDiffPlainBytes - int64(plain.Len())
		if remaining <= 0 {
			_ = zr.Close()
			_ = rc.Close()
			writeHTTPError(w, http.StatusRequestEntityTooLarge, "accumulated diff exceeds size limit")
			slog.Error("download accumulated run diff: uncompressed size limit reached", "run_id", run.ID.String(), "repo_id", run.RepoID.String(), "diff_id", diffID.String(), "limit_bytes", maxAccumulatedDiffPlainBytes)
			return false
		}

		n, copyErr := io.Copy(&plain, io.LimitReader(zr, remaining+1))
		if copyErr != nil {
			_ = zr.Close()
			_ = rc.Close()
			writeHTTPError(w, http.StatusInternalServerError, "failed to read diff blob")
			slog.Error("download accumulated run diff: gunzip copy failed", "run_id", run.ID.String(), "repo_id", run.RepoID.String(), "diff_id", diffID.String(), "object_key", *item.ObjectKey, "err", copyErr)
			return false
		}
		if n > remaining {
			_ = zr.Close()
			_ = rc.Close()
			writeHTTPError(w, http.StatusRequestEntityTooLarge, "accumulated diff exceeds size limit")
			slog.Error("download accumulated run diff: uncompressed size exceeded", "run_id", run.ID.String(), "repo_id", run.RepoID.String(), "diff_id", diffID.String(), "object_key", *item.ObjectKey, "limit_bytes", maxAccumulatedDiffPlainBytes)
			return false
		}

		if closeErr := zr.Close(); closeErr != nil {
			_ = rc.Close()
			writeHTTPError(w, http.StatusInternalServerError, "failed to read diff blob")
			slog.Error("download accumulated run diff: close gzip reader failed", "run_id", run.ID.String(), "repo_id", run.RepoID.String(), "diff_id", diffID.String(), "object_key", *item.ObjectKey, "err", closeErr)
			return false
		}
		if closeErr := rc.Close(); closeErr != nil {
			writeHTTPError(w, http.StatusInternalServerError, "failed to read diff blob")
			slog.Error("download accumulated run diff: close blob reader failed", "run_id", run.ID.String(), "repo_id", run.RepoID.String(), "diff_id", diffID.String(), "object_key", *item.ObjectKey, "err", closeErr)
			return false
		}
	}

	var gz bytes.Buffer
	zw := gzip.NewWriter(&gz)
	if _, err := zw.Write(plain.Bytes()); err != nil {
		_ = zw.Close()
		writeHTTPError(w, http.StatusInternalServerError, "failed to build accumulated diff")
		slog.Error("download accumulated run diff: write gzip failed", "run_id", run.ID.String(), "repo_id", run.RepoID.String(), "diff_id", diffID.String(), "err", err)
		return false
	}
	if err := zw.Close(); err != nil {
		writeHTTPError(w, http.StatusInternalServerError, "failed to build accumulated diff")
		slog.Error("download accumulated run diff: close gzip failed", "run_id", run.ID.String(), "repo_id", run.RepoID.String(), "diff_id", diffID.String(), "err", err)
		return false
	}

	streamBlob(w, bytes.NewReader(gz.Bytes()), int64(gz.Len()), fmt.Sprintf("diff-%s-accumulated.patch.gz", diffUUID.String()), "application/gzip")
	return true
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
	jobs, err := st.ListJobsByRunAttempt(ctx, store.ListJobsByRunAttemptParams{
		RunID:   run.ID,
		Attempt: run.Attempt,
	})
	if err != nil {
		return nil, err
	}
	if len(jobs) == 0 {
		return []runDiffRow{}, nil
	}

	orderByID := deriveJobOrderByChain(jobs)
	sort.SliceStable(jobs, func(i, j int) bool {
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
		sourceJob, sourceErr := resolveEffectiveSourceJob(ctx, st, job.ID)
		if sourceErr != nil {
			return nil, sourceErr
		}
		diff, getErr := st.GetLatestDiffByJob(ctx, &sourceJob.ID)
		if getErr != nil {
			if errors.Is(getErr, pgx.ErrNoRows) {
				continue
			}
			return nil, getErr
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
