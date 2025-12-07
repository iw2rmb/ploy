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

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
)

// diffItem represents a single diff in a list response.
//
// C2: Each diff is tagged with job_id and mod_type (in summary) to enable unified rehydration.
// - job_id: References the job that produced this diff; job's step_index provides ordering.
// - mod_type: "mod" for main mod diffs, "healing" for healing diffs (in summary).
// Rehydration queries fetch all diffs ordered by job step_index.
//
// NOTE: job_id is now a KSUID-backed string (no UUID parsing).
type diffItem struct {
	ID        string                  `json:"id"`
	JobID     string                  `json:"job_id"`
	CreatedAt time.Time               `json:"created_at"`
	Size      int                     `json:"gzipped_size"`
	Summary   domaintypes.DiffSummary `json:"summary,omitempty"` // Contains mod_type, timings.
}

// diffListResponse is the typed response for listing diffs.
type diffListResponse struct {
	Diffs []diffItem `json:"diffs"`
}

// diffGetResponse is the typed response for getting a single diff's metadata.
// NOTE: run_id and job_id are now KSUID-backed strings.
type diffGetResponse struct {
	ID          string                  `json:"id"`
	RunID       string                  `json:"run_id"`
	JobID       *string                 `json:"job_id,omitempty"`
	CreatedAt   time.Time               `json:"created_at"`
	GzippedSize int                     `json:"gzipped_size"`
	Summary     domaintypes.DiffSummary `json:"summary,omitempty"`
}

// listRunDiffsHandler returns a JSON list of diffs for a given Mods ticket (run id).
// GET /v1/mods/{id}/diffs
//
// Run and job IDs are now KSUID-backed strings; no UUID parsing is performed.
func listRunDiffsHandler(st store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Run IDs are KSUID strings; treated as opaque identifiers.
		idStr := strings.TrimSpace(r.PathValue("id"))
		if idStr == "" {
			http.Error(w, "ticket id is required", http.StatusBadRequest)
			return
		}

		// Use string run ID directly (no UUID parsing needed).
		diffs, err := st.ListDiffsByRun(r.Context(), idStr)
		if err != nil {
			http.Error(w, fmt.Sprintf("failed to list diffs: %v", err), http.StatusInternalServerError)
			slog.Error("list diffs: query failed", "ticket_id", idStr, "err", err)
			return
		}

		items := make([]diffItem, 0, len(diffs))
		for _, d := range diffs {
			var summary domaintypes.DiffSummary
			if len(d.Summary) > 0 {
				_ = json.Unmarshal(d.Summary, &summary)
			}
			// d.JobID is now *string (KSUID-backed).
			jobIDStr := ""
			if d.JobID != nil && *d.JobID != "" {
				jobIDStr = *d.JobID
			}
			items = append(items, diffItem{
				ID:        uuid.UUID(d.ID.Bytes).String(), // diffs.id is still UUID
				JobID:     jobIDStr,
				CreatedAt: d.CreatedAt.Time,
				Size:      len(d.Patch),
				Summary:   summary,
			})
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(diffListResponse{Diffs: items})
	}
}

// getDiffHandler returns diff bytes for a diff id. When ?download=true, writes
// the gzipped patch as application/gzip. Otherwise returns minimal JSON metadata.
// GET /v1/diffs/{id}
//
// NOTE: diffs.id is still UUID; run_id and job_id are KSUID strings.
func getDiffHandler(st store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		idStr := strings.TrimSpace(r.PathValue("id"))
		if idStr == "" {
			http.Error(w, "id is required", http.StatusBadRequest)
			return
		}
		// diffs.id is still UUID (outside scope of this task).
		diffUUID, err := uuid.Parse(idStr)
		if err != nil {
			http.Error(w, fmt.Sprintf("invalid id: %v", err), http.StatusBadRequest)
			return
		}
		d, err := st.GetDiff(r.Context(), pgtype.UUID{Bytes: diffUUID, Valid: true})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				http.Error(w, "diff not found", http.StatusNotFound)
				return
			}
			http.Error(w, fmt.Sprintf("failed to get diff: %v", err), http.StatusInternalServerError)
			slog.Error("get diff: query failed", "diff_id", idStr, "err", err)
			return
		}

		if strings.EqualFold(r.URL.Query().Get("download"), "true") {
			w.Header().Set("Content-Type", "application/gzip")
			w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=diff-%s.patch.gz", diffUUID.String()))
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(d.Patch)
			return
		}

		var summary domaintypes.DiffSummary
		if len(d.Summary) > 0 {
			_ = json.Unmarshal(d.Summary, &summary)
		}
		// d.ID is still pgtype.UUID; d.RunID and d.JobID are now strings.
		resp := diffGetResponse{
			ID:          uuid.UUID(d.ID.Bytes).String(),
			RunID:       d.RunID, // run_id is now a string (KSUID)
			CreatedAt:   d.CreatedAt.Time,
			GzippedSize: len(d.Patch),
			Summary:     summary,
		}
		if d.JobID != nil && *d.JobID != "" {
			resp.JobID = d.JobID
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}
}
