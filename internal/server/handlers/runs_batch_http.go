package handlers

import (
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/iw2rmb/ploy/internal/blobstore"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
	"github.com/iw2rmb/ploy/internal/workflow/lifecycle"
)

// cancelRunHandlerV1 returns an HTTP handler that cancels a v1 run.
// POST /v1/runs/{id}/cancel (v1 API) — Performs transactional cancellation of a run.
// This handler delegates cancellation to store.CancelRunV1, which atomically:
// - Sets runs.status=Cancelled (for non-terminal runs)
// - Cancels all repos with status Queued or Running → Cancelled
// - Cancels waiting/running jobs (Created/Queued/Running → Cancelled)
func cancelRunHandlerV1(st store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		runID, ok := parseRequiredPathIDOrWriteError[domaintypes.RunID](w, r, "id")
		if !ok {
			return
		}

		run, ok := getRunOrFail(w, r, st, runID, "cancel run")
		if !ok {
			return
		}

		// Idempotent: if already terminal, return current state.
		if lifecycle.IsTerminalRunStatus(run.Status) {
			summary := runToSummary(run)
			if counts, _ := getRunRepoCounts(r.Context(), st, run.ID); counts != nil && counts.Total > 0 {
				summary.Counts = counts
			}
			writeJSON(w, http.StatusOK, summary)
			return
		}

		if err := st.CancelRunV1(r.Context(), runID); err != nil {
			writeHTTPError(w, http.StatusInternalServerError, "failed to cancel run: %v", err)
			slog.Error("cancel run: transactional cancel failed", "run_id", runID.String(), "err", err)
			return
		}

		// Return updated run summary.
		run, _ = st.GetRun(r.Context(), runID)
		summary := runToSummary(run)
		if counts, _ := getRunRepoCounts(r.Context(), st, run.ID); counts != nil && counts.Total > 0 {
			summary.Counts = counts
		}
		writeJSON(w, http.StatusOK, summary)
	}
}

// addRunRepoHandler adds a repo to an existing run (and to the mig repo set).
// POST /v1/runs/{id}/repos — Body {repo_url, base_ref, target_ref}.
func addRunRepoHandler(st store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		runID, ok := parseRequiredPathIDOrWriteError[domaintypes.RunID](w, r, "id")
		if !ok {
			return
		}

		run, ok := getActiveRunOrFail(w, r, st, runID, "add run repo")
		if !ok {
			return
		}

		var req struct {
			RepoURL   domaintypes.RepoURL `json:"repo_url"`
			BaseRef   domaintypes.GitRef  `json:"base_ref"`
			TargetRef domaintypes.GitRef  `json:"target_ref"`
		}
		if err := decodeRequestJSON(w, r, &req, DefaultMaxBodySize); err != nil {
			return
		}
		if err := req.RepoURL.Validate(); err != nil {
			writeHTTPError(w, http.StatusBadRequest, "repo_url: %v", err)
			return
		}
		if err := req.BaseRef.Validate(); err != nil {
			writeHTTPError(w, http.StatusBadRequest, "base_ref: %v", err)
			return
		}
		if err := req.TargetRef.Validate(); err != nil {
			writeHTTPError(w, http.StatusBadRequest, "target_ref: %v", err)
			return
		}

		migRepoID := domaintypes.NewMigRepoID()
		migRepo, err := st.CreateMigRepo(r.Context(), store.CreateMigRepoParams{
			ID:        migRepoID,
			MigID:     run.MigID,
			Url:       req.RepoURL.String(),
			BaseRef:   req.BaseRef.String(),
			TargetRef: req.TargetRef.String(),
		})
		if err != nil {
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code == "23505" {
				writeHTTPError(w, http.StatusConflict, "repo already exists for mig")
				return
			}
			writeHTTPError(w, http.StatusInternalServerError, "failed to create mig repo: %v", err)
			slog.Error("add run repo: create mig repo failed", "run_id", runID.String(), "err", err)
			return
		}

		repoURL, err := repoURLForID(r.Context(), st, migRepo.RepoID)
		if err != nil {
			writeHTTPError(w, http.StatusInternalServerError, "failed to get repo url: %v", err)
			return
		}
		sourceCommitSHA, seedErr := resolveSourceCommitSHAFromContext(r.Context(), repoURL, migRepo.BaseRef)
		if seedErr != nil {
			writeHTTPError(w, http.StatusBadRequest, "failed to resolve source commit for repo %s ref %s: %v", repoURL, migRepo.BaseRef, seedErr)
			slog.Error("add run repo: resolve source commit failed",
				"run_id", runID.String(),
				"repo_id", migRepo.RepoID,
				"repo_url", repoURL,
				"base_ref", migRepo.BaseRef,
				"err", seedErr,
			)
			return
		}

		runRepo, err := st.CreateRunRepo(r.Context(), store.CreateRunRepoParams{
			MigID:           run.MigID,
			RunID:           runID,
			RepoID:          migRepo.RepoID,
			RepoBaseRef:     migRepo.BaseRef,
			RepoTargetRef:   migRepo.TargetRef,
			SourceCommitSha: sourceCommitSHA,
			RepoSha0:        sourceCommitSHA,
		})
		if err != nil {
			writeHTTPError(w, http.StatusInternalServerError, "failed to create run repo: %v", err)
			slog.Error("add run repo: create run repo failed", "run_id", runID.String(), "repo_id", migRepo.RepoID, "err", err)
			return
		}

		writeJSON(w, http.StatusCreated, runRepoToResponse(runRepo, repoURL, false, false))
	}
}

// listRunReposHandler lists repos for a run.
// GET /v1/runs/{id}/repos
func listRunReposHandler(st store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		runID, ok := parseRequiredPathIDOrWriteError[domaintypes.RunID](w, r, "id")
		if !ok {
			return
		}

		mrOnSuccess, mrOnFail, err := resolveRunMRWiring(r, st, runID)
		if err != nil {
			writeHTTPError(w, http.StatusInternalServerError, "failed to resolve run spec: %v", err)
			slog.Error("list run repos: resolve run spec failed", "run_id", runID.String(), "err", err)
			return
		}

		repos, err := st.ListRunReposWithURLByRun(r.Context(), runID)
		if err != nil {
			writeHTTPError(w, http.StatusInternalServerError, "failed to list run repos: %v", err)
			slog.Error("list run repos: fetch failed", "run_id", runID.String(), "err", err)
			return
		}

		reposResp := make([]RunRepoResponse, 0, len(repos))
		for _, rr := range repos {
			reposResp = append(reposResp, runRepoToResponse(store.RunRepo{
				MigID:           rr.MigID,
				RunID:           rr.RunID,
				RepoID:          rr.RepoID,
				RepoBaseRef:     rr.RepoBaseRef,
				RepoTargetRef:   rr.RepoTargetRef,
				SourceCommitSha: rr.SourceCommitSha,
				Status:          rr.Status,
				Attempt:         rr.Attempt,
				LastError:       rr.LastError,
				CreatedAt:       rr.CreatedAt,
				StartedAt:       rr.StartedAt,
				FinishedAt:      rr.FinishedAt,
			}, rr.RepoUrl, mrOnSuccess, mrOnFail))
		}

		resp := struct {
			Repos []RunRepoResponse `json:"repos"`
		}{Repos: reposResp}

		writeJSON(w, http.StatusOK, resp)
	}
}

// cancelRunRepoHandlerV1 cancels a repo execution within a run (v1 API).
// POST /v1/runs/{run_id}/repos/{repo_id}/cancel
func cancelRunRepoHandlerV1(st store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		runID, ok := parseRequiredPathIDOrWriteError[domaintypes.RunID](w, r, "run_id")
		if !ok {
			return
		}
		repoID, ok := parseRequiredPathIDOrWriteError[domaintypes.RepoID](w, r, "repo_id")
		if !ok {
			return
		}

		rr, err := st.GetRunRepo(r.Context(), store.GetRunRepoParams{RunID: runID, RepoID: repoID})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				writeHTTPError(w, http.StatusNotFound, "repo not found")
				return
			}
			writeHTTPError(w, http.StatusInternalServerError, "failed to get repo: %v", err)
			slog.Error("cancel run repo: get repo failed", "run_id", runID.String(), "repo_id", repoID.String(), "err", err)
			return
		}

		if lifecycle.IsTerminalRunRepoStatus(rr.Status) {
			repoURL := ""
			if resolvedURL, err := repoURLForID(r.Context(), st, rr.RepoID); err == nil {
				repoURL = resolvedURL
			}
			writeJSON(w, http.StatusOK, runRepoToResponse(rr, repoURL, false, false))
			return
		}

		if err := st.UpdateRunRepoStatus(r.Context(), store.UpdateRunRepoStatusParams{RunID: runID, RepoID: repoID, Status: domaintypes.RunRepoStatusCancelled}); err != nil {
			slog.Error("cancel run repo: update status failed", "run_id", runID.String(), "repo_id", repoID.String(), "err", err)
		}

		now := time.Now().UTC()
		jobs, err := st.ListJobsByRunRepoAttempt(r.Context(), store.ListJobsByRunRepoAttemptParams{RunID: runID, RepoID: repoID, Attempt: rr.Attempt})
		if err == nil {
			for _, job := range jobs {
				if job.Status != domaintypes.JobStatusCreated && job.Status != domaintypes.JobStatusQueued && job.Status != domaintypes.JobStatusRunning {
					continue
				}
				dur := int64(0)
				if job.StartedAt.Valid {
					if d := now.Sub(job.StartedAt.Time).Milliseconds(); d > 0 {
						dur = d
					}
				}
				_ = st.UpdateJobStatus(r.Context(), store.UpdateJobStatusParams{
					ID:         job.ID,
					Status:     domaintypes.JobStatusCancelled,
					StartedAt:  job.StartedAt,
					FinishedAt: pgtype.Timestamptz{Time: now, Valid: true},
					DurationMs: dur,
				})
			}
		}

		rr, _ = st.GetRunRepo(r.Context(), store.GetRunRepoParams{RunID: runID, RepoID: repoID})
		repoURL := ""
		if resolvedURL, err := repoURLForID(r.Context(), st, rr.RepoID); err == nil {
			repoURL = resolvedURL
		}
		writeJSON(w, http.StatusOK, runRepoToResponse(rr, repoURL, false, false))
	}
}

// restartRunRepoHandler restarts a repo execution by incrementing attempt and creating new repo-scoped jobs.
// POST /v1/runs/{id}/repos/{repo_id}/restart
func restartRunRepoHandler(st store.Store, hookBlobstores ...blobstore.Store) http.HandlerFunc {
	var bs blobstore.Store
	if len(hookBlobstores) > 0 {
		bs = hookBlobstores[0]
	}
	return func(w http.ResponseWriter, r *http.Request) {
		runID, ok := parseRequiredPathIDOrWriteError[domaintypes.RunID](w, r, "id")
		if !ok {
			return
		}
		repoID, ok := parseRequiredPathIDOrWriteError[domaintypes.RepoID](w, r, "repo_id")
		if !ok {
			return
		}

		run, ok := getRunOrFail(w, r, st, runID, "restart run repo")
		if !ok {
			return
		}

		runRepo, err := st.GetRunRepo(r.Context(), store.GetRunRepoParams{RunID: runID, RepoID: repoID})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				writeHTTPError(w, http.StatusNotFound, "repo not found")
				return
			}
			writeHTTPError(w, http.StatusInternalServerError, "failed to get repo: %v", err)
			slog.Error("restart run repo: get repo failed", "run_id", runID.String(), "repo_id", repoID.String(), "err", err)
			return
		}

		var req struct {
			BaseRef   *domaintypes.GitRef `json:"base_ref,omitempty"`
			TargetRef *domaintypes.GitRef `json:"target_ref,omitempty"`
		}
		if r.ContentLength > 0 || r.Header.Get("Transfer-Encoding") == "chunked" {
			if err := decodeRequestJSON(w, r, &req, DefaultMaxBodySize); err != nil {
				return
			}
			if req.BaseRef != nil {
				if err := req.BaseRef.Validate(); err != nil {
					writeHTTPError(w, http.StatusBadRequest, "base_ref: %v", err)
					return
				}
			}
			if req.TargetRef != nil {
				if err := req.TargetRef.Validate(); err != nil {
					writeHTTPError(w, http.StatusBadRequest, "target_ref: %v", err)
					return
				}
			}
		}

		// If the run is terminal, reopen it to Started for the restart attempt.
		if lifecycle.IsTerminalRunStatus(run.Status) {
			if err := st.UpdateRunStatus(r.Context(), store.UpdateRunStatusParams{ID: runID, Status: domaintypes.RunStatusStarted}); err != nil {
				writeHTTPError(w, http.StatusInternalServerError, "failed to reopen run: %v", err)
				return
			}
		}

		if req.BaseRef != nil || req.TargetRef != nil {
			newBase := runRepo.RepoBaseRef
			if req.BaseRef != nil {
				newBase = req.BaseRef.String()
			}
			newTarget := runRepo.RepoTargetRef
			if req.TargetRef != nil {
				newTarget = req.TargetRef.String()
			}
			_ = st.UpdateRunRepoRefs(r.Context(), store.UpdateRunRepoRefsParams{RunID: runID, RepoID: repoID, RepoBaseRef: newBase, RepoTargetRef: newTarget})
			if migRepos, listErr := st.ListMigReposByMig(r.Context(), run.MigID); listErr == nil {
				for _, migRepo := range migRepos {
					if migRepo.RepoID == repoID {
						_ = st.UpdateMigRepoRefs(r.Context(), store.UpdateMigRepoRefsParams{ID: migRepo.ID, BaseRef: newBase, TargetRef: newTarget})
						break
					}
				}
			}
		}

		if err := st.IncrementRunRepoAttempt(r.Context(), store.IncrementRunRepoAttemptParams{RunID: runID, RepoID: repoID}); err != nil {
			writeHTTPError(w, http.StatusInternalServerError, "failed to restart repo: %v", err)
			slog.Error("restart run repo: increment attempt failed", "run_id", runID.String(), "repo_id", repoID.String(), "err", err)
			return
		}

		runRepo, err = st.GetRunRepo(r.Context(), store.GetRunRepoParams{RunID: runID, RepoID: repoID})
		if err != nil {
			writeHTTPError(w, http.StatusInternalServerError, "failed to reload repo: %v", err)
			return
		}

		spec, err := st.GetSpec(r.Context(), run.SpecID)
		if err != nil {
			writeHTTPError(w, http.StatusInternalServerError, "failed to load spec: %v", err)
			return
		}
		if err := createJobsFromSpec(r.Context(), st, runID, runRepo.RepoID, runRepo.RepoBaseRef, runRepo.Attempt, runRepo.RepoSha0, spec.Spec, bs); err != nil {
			writeHTTPError(w, http.StatusInternalServerError, "failed to create jobs: %v", err)
			return
		}

		repoURL := ""
		if resolvedURL, err := repoURLForID(r.Context(), st, runRepo.RepoID); err == nil {
			repoURL = resolvedURL
		}

		writeJSON(w, http.StatusOK, runRepoToResponse(runRepo, repoURL, false, false))
	}
}

func resolveRunMRWiring(r *http.Request, st store.Store, runID domaintypes.RunID) (bool, bool, error) {
	run, err := st.GetRun(r.Context(), runID)
	if err != nil {
		return false, false, err
	}
	spec, err := st.GetSpec(r.Context(), run.SpecID)
	if err != nil {
		return false, false, err
	}
	parsed, err := contracts.ParseMigSpecJSON(spec.Spec)
	if err != nil {
		return false, false, err
	}

	mrOnSuccess := parsed.MROnSuccess != nil && *parsed.MROnSuccess
	mrOnFail := parsed.MROnFail != nil && *parsed.MROnFail
	return mrOnSuccess, mrOnFail, nil
}

// StartRunResponse contains the result of starting a batch run.
type StartRunResponse struct {
	RunID       domaintypes.RunID `json:"run_id"`
	Started     int               `json:"started"`
	AlreadyDone int               `json:"already_done"`
	Pending     int               `json:"pending"`
}

// startRunHandler delegates to BatchRepoStarter.StartPendingRepos (shared with the background scheduler).
// POST /v1/runs/{id}/start
func startRunHandler(st store.Store, hookBlobstores ...blobstore.Store) http.HandlerFunc {
	starter := NewBatchRepoStarter(st, hookBlobstores...)

	return func(w http.ResponseWriter, r *http.Request) {
		runID, ok := parseRequiredPathIDOrWriteError[domaintypes.RunID](w, r, "id")
		if !ok {
			return
		}

		if _, ok := getActiveRunOrFail(w, r, st, runID, "start run"); !ok {
			return
		}

		result, err := starter.StartPendingRepos(r.Context(), runID)
		if err != nil {
			writeHTTPError(w, http.StatusInternalServerError, "failed to start queued repos: %v", err)
			slog.Error("start run: start queued repos failed", "run_id", runID.String(), "err", err)
			return
		}

		resp := StartRunResponse{
			RunID:       runID,
			Started:     result.Started,
			AlreadyDone: result.AlreadyDone,
			Pending:     result.Pending,
		}

		writeJSON(w, http.StatusOK, resp)
	}
}
