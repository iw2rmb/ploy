package handlers

import (
	"errors"
	"io"
	"net/http"
	"strings"
	"unicode/utf8"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
)

func saveJobReportHandler(st store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		jobID, ok := parseRequiredPathIDOrWriteError[domaintypes.JobID](w, r, "job_id")
		if !ok {
			return
		}
		nodeID, ok := requireNodeUUIDHeader(w, r)
		if !ok {
			return
		}
		job, ok := getJobOrFail(w, r, st, jobID, "save job report")
		if !ok || !assertJobAssignedToNode(w, job, nodeID) {
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, DefaultMaxBodySize)
		body, err := io.ReadAll(r.Body)
		if err != nil {
			var tooLarge *http.MaxBytesError
			if errors.As(err, &tooLarge) {
				writeHTTPError(w, http.StatusRequestEntityTooLarge, "report exceeds body size cap")
			} else {
				writeHTTPError(w, http.StatusBadRequest, "cannot read report")
			}
			return
		}
		if !utf8.Valid(body) || strings.ContainsRune(string(body), '\x00') {
			writeHTTPError(w, http.StatusBadRequest, "report must be UTF-8 text without NUL characters")
			return
		}
		n, err := st.UpdateJobReport(r.Context(), store.UpdateJobReportParams{ID: jobID, Report: string(body)})
		if err != nil {
			serverError(w, "save job report", "save job report", err)
			return
		}
		if n == 0 {
			writeHTTPError(w, http.StatusNotFound, "job not found")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}
