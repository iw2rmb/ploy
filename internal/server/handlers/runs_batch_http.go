package handlers

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
)

// cancelRunHandlerV1 returns an HTTP handler that cancels a v1 run.
// POST /v1/runs/{id}/cancel (v1 API) — Performs transactional cancellation of a run.
// This handler delegates cancellation to store.CancelRunV1, which atomically:
// - Sets runs.status=Cancelled (for non-terminal runs)
// - Cancels all repos with status Queued or Running → Cancelled
// - Cancels waiting/running jobs (Created/Queued/Running → Cancelled)
func cancelRunHandlerV1(st store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		runID, err := parseParam[domaintypes.RunID](r, "id")
		if err != nil {
			httpErr(w, http.StatusBadRequest, "%s", err)
			return
		}

		run, err := st.GetRun(r.Context(), runID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				httpErr(w, http.StatusNotFound, "run not found")
				return
			}
			httpErr(w, http.StatusInternalServerError, "failed to get run: %v", err)
			slog.Error("cancel run: fetch failed", "run_id", runID.String(), "err", err)
			return
		}

		// Idempotent: if already terminal, return current state.
		if run.Status == store.RunStatusFinished || run.Status == store.RunStatusCancelled {
			summary := runToSummary(run)
			if counts, _ := getRunRepoCounts(r.Context(), st, run.ID); counts != nil && counts.Total > 0 {
				summary.Counts = counts
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(summary)
			return
		}

		if err := st.CancelRunV1(r.Context(), runID); err != nil {
			httpErr(w, http.StatusInternalServerError, "failed to cancel run: %v", err)
			slog.Error("cancel run: transactional cancel failed", "run_id", runID.String(), "err", err)
			return
		}

		// Return updated run summary.
		run, _ = st.GetRun(r.Context(), runID)
		summary := runToSummary(run)
		if counts, _ := getRunRepoCounts(r.Context(), st, run.ID); counts != nil && counts.Total > 0 {
			summary.Counts = counts
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(summary)
	}
}

// addRunRepoHandler adds a repo to an existing run (and to the mig repo set).
// POST /v1/runs/{id}/repos — Body {repo_url, base_ref, target_ref}.
func addRunRepoHandler(st store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		runID, err := parseParam[domaintypes.RunID](r, "id")
		if err != nil {
			httpErr(w, http.StatusBadRequest, "%s", err)
			return
		}

		run, err := st.GetRun(r.Context(), runID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				httpErr(w, http.StatusNotFound, "run not found")
				return
			}
			httpErr(w, http.StatusInternalServerError, "failed to get run: %v", err)
			slog.Error("add run repo: get run failed", "run_id", runID.String(), "err", err)
			return
		}
		if run.Status == store.RunStatusFinished || run.Status == store.RunStatusCancelled {
			httpErr(w, http.StatusConflict, "cannot add repos to a terminal run")
			return
		}

		var req struct {
			RepoURL   domaintypes.RepoURL `json:"repo_url"`
			BaseRef   domaintypes.GitRef  `json:"base_ref"`
			TargetRef domaintypes.GitRef  `json:"target_ref"`
		}
		if err := DecodeJSON(w, r, &req, DefaultMaxBodySize); err != nil {
			return
		}
		if err := req.RepoURL.Validate(); err != nil {
			httpErr(w, http.StatusBadRequest, "repo_url: %v", err)
			return
		}
		if err := req.BaseRef.Validate(); err != nil {
			httpErr(w, http.StatusBadRequest, "base_ref: %v", err)
			return
		}
		if err := req.TargetRef.Validate(); err != nil {
			httpErr(w, http.StatusBadRequest, "target_ref: %v", err)
			return
		}

		modRepoID := domaintypes.NewMigRepoID()
		modRepo, err := st.CreateMigRepo(r.Context(), store.CreateMigRepoParams{
			ID:        modRepoID,
			MigID:     run.MigID,
			Url:       req.RepoURL.String(),
			BaseRef:   req.BaseRef.String(),
			TargetRef: req.TargetRef.String(),
		})
		if err != nil {
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code == "23505" {
				httpErr(w, http.StatusConflict, "repo already exists for mig")
				return
			}
			httpErr(w, http.StatusInternalServerError, "failed to create mig repo: %v", err)
			slog.Error("add run repo: create mig repo failed", "run_id", runID.String(), "err", err)
			return
		}

		repoURL, err := repoURLForID(r.Context(), st, modRepo.RepoID)
		if err != nil {
			httpErr(w, http.StatusInternalServerError, "failed to get repo url: %v", err)
			return
		}
		sourceCommitSHA, seedErr := resolveSourceCommitSHAFromContext(r.Context(), repoURL, modRepo.BaseRef)
		if seedErr != nil {
			httpErr(w, http.StatusBadRequest, "failed to resolve source commit for repo %s ref %s: %v", repoURL, modRepo.BaseRef, seedErr)
			slog.Error("add run repo: resolve source commit failed",
				"run_id", runID.String(),
				"repo_id", modRepo.RepoID,
				"repo_url", repoURL,
				"base_ref", modRepo.BaseRef,
				"err", seedErr,
			)
			return
		}

		runRepo, err := st.CreateRunRepo(r.Context(), store.CreateRunRepoParams{
			MigID:           run.MigID,
			RunID:           runID,
			RepoID:          modRepo.RepoID,
			RepoBaseRef:     modRepo.BaseRef,
			RepoTargetRef:   modRepo.TargetRef,
			SourceCommitSha: sourceCommitSHA,
			RepoSha0:        sourceCommitSHA,
		})
		if err != nil {
			httpErr(w, http.StatusInternalServerError, "failed to create run repo: %v", err)
			slog.Error("add run repo: create run repo failed", "run_id", runID.String(), "repo_id", modRepo.RepoID, "err", err)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(runRepoToResponse(runRepo, repoURL))
	}
}

// listRunReposHandler lists repos for a run.
// GET /v1/runs/{id}/repos
func listRunReposHandler(st store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		runID, err := parseParam[domaintypes.RunID](r, "id")
		if err != nil {
			httpErr(w, http.StatusBadRequest, "%s", err)
			return
		}

		repos, err := st.ListRunReposWithURLByRun(r.Context(), runID)
		if err != nil {
			httpErr(w, http.StatusInternalServerError, "failed to list run repos: %v", err)
			slog.Error("list run repos: fetch failed", "run_id", runID.String(), "err", err)
			return
		}

		reposResp := make([]RunRepoResponse, 0, len(repos))
		for _, rr := range repos {
			reposResp = append(reposResp, runRepoToResponse(store.RunRepo{
				MigID:         rr.MigID,
				RunID:         rr.RunID,
				RepoID:        rr.RepoID,
				RepoBaseRef:   rr.RepoBaseRef,
				RepoTargetRef: rr.RepoTargetRef,
				Status:        rr.Status,
				Attempt:       rr.Attempt,
				LastError:     rr.LastError,
				CreatedAt:     rr.CreatedAt,
				StartedAt:     rr.StartedAt,
				FinishedAt:    rr.FinishedAt,
			}, rr.RepoUrl))
		}

		resp := struct {
			Repos []RunRepoResponse `json:"repos"`
		}{Repos: reposResp}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(resp)
	}
}

// cancelRunRepoHandlerV1 cancels a repo execution within a run (v1 API).
// POST /v1/runs/{run_id}/repos/{repo_id}/cancel
func cancelRunRepoHandlerV1(st store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		runID, err := parseParam[domaintypes.RunID](r, "run_id")
		if err != nil {
			httpErr(w, http.StatusBadRequest, "%s", err)
			return
		}
		repoID, err := parseParam[domaintypes.RepoID](r, "repo_id")
		if err != nil {
			httpErr(w, http.StatusBadRequest, "%s", err)
			return
		}

		rr, err := st.GetRunRepo(r.Context(), store.GetRunRepoParams{RunID: runID, RepoID: repoID})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				httpErr(w, http.StatusNotFound, "repo not found")
				return
			}
			httpErr(w, http.StatusInternalServerError, "failed to get repo: %v", err)
			slog.Error("cancel run repo: get repo failed", "run_id", runID.String(), "repo_id", repoID.String(), "err", err)
			return
		}

		if rr.Status == store.RunRepoStatusCancelled || rr.Status == store.RunRepoStatusSuccess || rr.Status == store.RunRepoStatusFail {
			repoURL := ""
			if resolvedURL, err := repoURLForID(r.Context(), st, rr.RepoID); err == nil {
				repoURL = resolvedURL
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(runRepoToResponse(rr, repoURL))
			return
		}

		_ = st.UpdateRunRepoStatus(r.Context(), store.UpdateRunRepoStatusParams{RunID: runID, RepoID: repoID, Status: store.RunRepoStatusCancelled})

		now := time.Now().UTC()
		jobs, err := st.ListJobsByRunRepoAttempt(r.Context(), store.ListJobsByRunRepoAttemptParams{RunID: runID, RepoID: repoID, Attempt: rr.Attempt})
		if err == nil {
			for _, job := range jobs {
				if job.Status != store.JobStatusCreated && job.Status != store.JobStatusQueued && job.Status != store.JobStatusRunning {
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
					Status:     store.JobStatusCancelled,
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
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(runRepoToResponse(rr, repoURL))
	}
}

// restartRunRepoHandler restarts a repo execution by incrementing attempt and creating new repo-scoped jobs.
// POST /v1/runs/{id}/repos/{repo_id}/restart
func restartRunRepoHandler(st store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		runID, err := parseParam[domaintypes.RunID](r, "id")
		if err != nil {
			httpErr(w, http.StatusBadRequest, "%s", err)
			return
		}
		repoID, err := parseParam[domaintypes.RepoID](r, "repo_id")
		if err != nil {
			httpErr(w, http.StatusBadRequest, "%s", err)
			return
		}

		run, err := st.GetRun(r.Context(), runID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				httpErr(w, http.StatusNotFound, "run not found")
				return
			}
			httpErr(w, http.StatusInternalServerError, "failed to get run: %v", err)
			slog.Error("restart run repo: get run failed", "run_id", runID.String(), "err", err)
			return
		}

		runRepo, err := st.GetRunRepo(r.Context(), store.GetRunRepoParams{RunID: runID, RepoID: repoID})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				httpErr(w, http.StatusNotFound, "repo not found")
				return
			}
			httpErr(w, http.StatusInternalServerError, "failed to get repo: %v", err)
			slog.Error("restart run repo: get repo failed", "run_id", runID.String(), "repo_id", repoID.String(), "err", err)
			return
		}

		var req struct {
			BaseRef   *domaintypes.GitRef `json:"base_ref,omitempty"`
			TargetRef *domaintypes.GitRef `json:"target_ref,omitempty"`
		}
		if r.ContentLength > 0 || r.Header.Get("Transfer-Encoding") == "chunked" {
			if err := DecodeJSON(w, r, &req, DefaultMaxBodySize); err != nil {
				return
			}
			if req.BaseRef != nil {
				if err := req.BaseRef.Validate(); err != nil {
					httpErr(w, http.StatusBadRequest, "base_ref: %v", err)
					return
				}
			}
			if req.TargetRef != nil {
				if err := req.TargetRef.Validate(); err != nil {
					httpErr(w, http.StatusBadRequest, "target_ref: %v", err)
					return
				}
			}
		}

		// If the run is terminal, reopen it to Started for the restart attempt.
		if run.Status == store.RunStatusFinished || run.Status == store.RunStatusCancelled {
			if err := st.UpdateRunStatus(r.Context(), store.UpdateRunStatusParams{ID: runID, Status: store.RunStatusStarted}); err != nil {
				httpErr(w, http.StatusInternalServerError, "failed to reopen run: %v", err)
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
			if modRepos, listErr := st.ListMigReposByMig(r.Context(), run.MigID); listErr == nil {
				for _, modRepo := range modRepos {
					if modRepo.RepoID == repoID {
						_ = st.UpdateMigRepoRefs(r.Context(), store.UpdateMigRepoRefsParams{ID: modRepo.ID, BaseRef: newBase, TargetRef: newTarget})
						break
					}
				}
			}
		}

		if err := st.IncrementRunRepoAttempt(r.Context(), store.IncrementRunRepoAttemptParams{RunID: runID, RepoID: repoID}); err != nil {
			httpErr(w, http.StatusInternalServerError, "failed to restart repo: %v", err)
			slog.Error("restart run repo: increment attempt failed", "run_id", runID.String(), "repo_id", repoID.String(), "err", err)
			return
		}

		runRepo, err = st.GetRunRepo(r.Context(), store.GetRunRepoParams{RunID: runID, RepoID: repoID})
		if err != nil {
			httpErr(w, http.StatusInternalServerError, "failed to reload repo: %v", err)
			return
		}

		spec, err := st.GetSpec(r.Context(), run.SpecID)
		if err != nil {
			httpErr(w, http.StatusInternalServerError, "failed to load spec: %v", err)
			return
		}
		if err := createJobsFromSpec(r.Context(), st, runID, runRepo.RepoID, runRepo.RepoBaseRef, runRepo.Attempt, spec.Spec); err != nil {
			httpErr(w, http.StatusInternalServerError, "failed to create jobs: %v", err)
			return
		}

		repoURL := ""
		if resolvedURL, err := repoURLForID(r.Context(), st, runRepo.RepoID); err == nil {
			repoURL = resolvedURL
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(runRepoToResponse(runRepo, repoURL))
	}
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
func startRunHandler(st store.Store) http.HandlerFunc {
	starter := NewBatchRepoStarter(st)

	return func(w http.ResponseWriter, r *http.Request) {
		runID, err := parseParam[domaintypes.RunID](r, "id")
		if err != nil {
			httpErr(w, http.StatusBadRequest, "%s", err)
			return
		}

		run, err := st.GetRun(r.Context(), runID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				httpErr(w, http.StatusNotFound, "run not found")
				return
			}
			httpErr(w, http.StatusInternalServerError, "failed to get run: %v", err)
			slog.Error("start run: fetch failed", "run_id", runID.String(), "err", err)
			return
		}

		if isTerminalRunStatus(run.Status) {
			httpErr(w, http.StatusConflict, "cannot start repos in a terminal run")
			return
		}

		result, err := starter.StartPendingRepos(r.Context(), runID)
		if err != nil {
			httpErr(w, http.StatusInternalServerError, "failed to start queued repos: %v", err)
			slog.Error("start run: start queued repos failed", "run_id", runID.String(), "err", err)
			return
		}

		resp := StartRunResponse{
			RunID:       runID,
			Started:     result.Started,
			AlreadyDone: result.AlreadyDone,
			Pending:     result.Pending,
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(resp)
	}
}
