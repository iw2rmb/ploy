package handlers

import (
	"net/http"

	"github.com/iw2rmb/ploy/internal/blobstore"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/gitauth"
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

func addRunRepoHandler(store.Store, gitauth.Options) http.HandlerFunc {
	return removedRunRepoSurface
}

func listRunReposHandler(st store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		runID, ok := parseRequiredPathIDOrWriteError[domaintypes.RunID](w, r, "run_id")
		if !ok {
			return
		}
		run, ok := getRunOrFail(w, r, st, runID, "list run repos")
		if !ok {
			return
		}
		repoURL := ""
		if repo, err := st.GetRepo(r.Context(), run.RepoID); err == nil {
			repoURL = repo.Url
		}
		writeJSON(w, http.StatusOK, struct {
			Repos []RunRepoResponse `json:"repos"`
		}{Repos: []RunRepoResponse{runRepoToResponse(run, repoURL)}})
	}
}

func cancelRunRepoHandlerV1(store.Store) http.HandlerFunc {
	return removedRunRepoSurface
}

func restartRunRepoHandler(store.Store, blobstore.Store) http.HandlerFunc {
	return removedRunRepoSurface
}

func removedRunRepoSurface(w http.ResponseWriter, _ *http.Request) {
	writeHTTPError(w, http.StatusGone, "repo-scoped run endpoint was removed; use run-scoped endpoints")
}
