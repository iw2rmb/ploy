package handlers

import (
	"errors"
	"net/http"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
	"github.com/iw2rmb/ploy/internal/workflow/lifecycle"
)

func cancelRunHandlerV1(st store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		runID, ok := parseRequiredPathIDOrWriteError[domaintypes.RunID](w, r, "run_id")
		if !ok {
			return
		}
		run, ok := getRunOrFail(w, r, st, runID, "cancel run")
		if !ok {
			return
		}
		if !lifecycle.IsTerminalRunStatus(run.Status) {
			if err := st.CancelRun(r.Context(), runID); err != nil {
				writeHTTPError(w, http.StatusInternalServerError, "failed to cancel run: %v", err)
				return
			}
			var err error
			run, err = st.GetRun(r.Context(), runID)
			if err != nil {
				writeHTTPError(w, http.StatusInternalServerError, "failed to load updated run: %v", err)
				return
			}
		}
		writeJSON(w, http.StatusOK, runToSummary(run))
	}
}

func restartRunHandler(st store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		runID, ok := parseRequiredPathIDOrWriteError[domaintypes.RunID](w, r, "run_id")
		if !ok {
			return
		}
		run, err := st.RestartRun(r.Context(), runID)
		if err != nil {
			switch {
			case errors.Is(err, store.ErrRunRestartActive):
				writeHTTPError(w, http.StatusConflict, "run is not terminal")
			case errors.Is(err, store.ErrRunRestartWaveCancelled):
				writeHTTPError(w, http.StatusConflict, "owning wave is cancelled")
			case isNoRowsError(err):
				writeHTTPError(w, http.StatusNotFound, "run not found")
			default:
				writeHTTPError(w, http.StatusInternalServerError, "failed to restart run: %v", err)
			}
			return
		}
		writeJSON(w, http.StatusOK, runToSummary(run))
	}
}
