package handlers

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	domainapi "github.com/iw2rmb/ploy/internal/domain/api"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/gitauth"
	"github.com/iw2rmb/ploy/internal/gitlabtoken"
	"github.com/iw2rmb/ploy/internal/server/gitlabtokens"
	"github.com/iw2rmb/ploy/internal/store"
	"github.com/iw2rmb/ploy/internal/workflow/lifecycle"
)

func cancelRunHandlerV1(st store.Store, registry *gitlabtokens.Registry) http.HandlerFunc {
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
			registry.ReleaseRuns([]domaintypes.RunID{runID})
			var err error
			run, err = st.GetRun(r.Context(), runID)
			if err != nil {
				writeHTTPError(w, http.StatusInternalServerError, "failed to load updated run: %v", err)
				return
			}
		} else {
			registry.ReleaseRuns([]domaintypes.RunID{runID})
		}
		writeJSON(w, http.StatusOK, runToSummary(run))
	}
}

func restartRunHandler(st store.Store, gitAuth gitauth.Options, registry *gitlabtokens.Registry) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		runID, ok := parseRequiredPathIDOrWriteError[domaintypes.RunID](w, r, "run_id")
		if !ok {
			return
		}
		var req domainapi.RunRestartRequest
		if err := decodeOptionalRequestJSON(w, r, &req, DefaultMaxBodySize); err != nil {
			return
		}
		existingRun, ok := getRunOrFail(w, r, st, runID, "restart run")
		if !ok {
			return
		}
		var runStats []byte
		gitLabTokenHash, gitLabToken, err := validateRestartGitLabTokenRequest(r, st, existingRun, req.GitLabToken, gitAuth)
		if err != nil {
			status := http.StatusBadRequest
			if isNoRowsError(err) {
				status = http.StatusNotFound
			}
			writeHTTPError(w, status, "%v", err)
			return
		}
		spec, err := st.GetSpec(r.Context(), existingRun.SpecID)
		if err != nil {
			writeHTTPError(w, http.StatusInternalServerError, "failed to load run spec: %v", err)
			return
		}
		plannedJobs, err := planJobsFromSpec(spec.Spec)
		if err != nil {
			writeHTTPError(w, http.StatusInternalServerError, "failed to plan restarted run jobs: %v", err)
			return
		}
		if gitLabTokenHash != "" {
			runStats, err = gitlabtoken.RunStatsWithMarker(gitLabTokenHash)
			if err != nil {
				writeHTTPError(w, http.StatusBadRequest, "%v", err)
				return
			}
			registry.Register(gitLabTokenHash, gitLabToken, []domaintypes.RunID{runID})
		}
		run, err := st.RestartRun(r.Context(), store.RestartRunParams{
			RunID:           runID,
			ExpectedAttempt: existingRun.Attempt,
			Stats:           runStats,
			Jobs:            plannedJobs,
		})
		if err != nil {
			if gitLabTokenHash != "" {
				registry.ReleaseRuns([]domaintypes.RunID{runID})
			}
			if writeActiveRepoRunConflict(w, err) {
				return
			}
			switch {
			case errors.Is(err, store.ErrRunRestartActive):
				writeHTTPError(w, http.StatusConflict, "run is not terminal; current Run ID: %s", runID)
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

func decodeOptionalRequestJSON(w http.ResponseWriter, r *http.Request, v any, maxBytes int64) error {
	if r.Body == nil {
		return nil
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			writeHTTPError(w, http.StatusRequestEntityTooLarge, "payload exceeds body size cap")
			return err
		}
		writeHTTPError(w, http.StatusBadRequest, "invalid request: %v", err)
		return err
	}
	if strings.TrimSpace(string(body)) == "" {
		return nil
	}
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		writeHTTPError(w, http.StatusBadRequest, "invalid request: %v", err)
		return err
	}
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		writeHTTPError(w, http.StatusBadRequest, "invalid request: request body must contain exactly one JSON value")
		if err == nil {
			return errors.New("request body must contain exactly one JSON value")
		}
		return err
	}
	return nil
}

func validateRestartGitLabTokenRequest(r *http.Request, st store.Store, run store.Run, token *string, gitAuth gitauth.Options) (string, string, error) {
	if token == nil {
		return "", "", nil
	}
	repo, err := st.GetRepo(r.Context(), run.RepoID)
	if err != nil {
		return "", "", err
	}
	return validateGitLabTokenRequestForRepos(token, gitAuth.GitLabDomain, []string{repo.Url})
}
