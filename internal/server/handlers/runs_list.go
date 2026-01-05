package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/jackc/pgx/v5"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
)

func listRunsHandler(st store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		limit := int32(50)
		offset := int32(0)

		if l := r.URL.Query().Get("limit"); l != "" {
			parsed, err := strconv.ParseInt(l, 10, 32)
			if err != nil || parsed < 1 {
				http.Error(w, "invalid limit parameter", http.StatusBadRequest)
				return
			}
			limit = int32(parsed)
			if limit > 100 {
				limit = 100
			}
		}
		if o := r.URL.Query().Get("offset"); o != "" {
			parsed, err := strconv.ParseInt(o, 10, 32)
			if err != nil || parsed < 0 {
				http.Error(w, "invalid offset parameter", http.StatusBadRequest)
				return
			}
			offset = int32(parsed)
		}

		runs, err := st.ListRuns(r.Context(), store.ListRunsParams{Limit: limit, Offset: offset})
		if err != nil {
			http.Error(w, fmt.Sprintf("failed to list runs: %v", err), http.StatusInternalServerError)
			slog.Error("list runs: fetch failed", "err", err)
			return
		}

		summaries := make([]domaintypes.RunSummary, 0, len(runs))
		for _, run := range runs {
			summaries = append(summaries, runToSummary(run))
		}

		resp := struct {
			Runs []domaintypes.RunSummary `json:"runs"`
		}{Runs: summaries}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(resp)
	}
}

func getRunHandler(st store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		runIDStr, err := requiredPathParam(r, "id")
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		run, err := st.GetRun(r.Context(), runIDStr)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				http.Error(w, "run not found", http.StatusNotFound)
				return
			}
			http.Error(w, fmt.Sprintf("failed to get run: %v", err), http.StatusInternalServerError)
			slog.Error("get run: fetch failed", "run_id", runIDStr, "err", err)
			return
		}

		summary := runToSummary(run)
		if counts, _ := getRunRepoCounts(r.Context(), st, domaintypes.RunID(run.ID)); counts != nil && counts.Total > 0 {
			summary.Counts = counts
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(summary)
	}
}
