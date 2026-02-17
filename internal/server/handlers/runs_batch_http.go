package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
)

type cancelRunStore interface {
	GetRun(ctx context.Context, id domaintypes.RunID) (store.Run, error)
	CancelRunV1(ctx context.Context, runID domaintypes.RunID) error
	CountRunReposByStatus(ctx context.Context, runID domaintypes.RunID) ([]store.CountRunReposByStatusRow, error)
}

type addRunRepoStore interface {
	GetRun(ctx context.Context, id domaintypes.RunID) (store.Run, error)
	CreateModRepo(ctx context.Context, params store.CreateModRepoParams) (store.ModRepo, error)
	CreateRunRepo(ctx context.Context, params store.CreateRunRepoParams) (store.RunRepo, error)
	GetSpec(ctx context.Context, id domaintypes.SpecID) (store.Spec, error)
	CreateJob(ctx context.Context, params store.CreateJobParams) (store.Job, error)
}

type listRunReposStore interface {
	ListRunReposWithURLByRun(ctx context.Context, runID domaintypes.RunID) ([]store.ListRunReposWithURLByRunRow, error)
}

type cancelRunRepoStore interface {
	GetRunRepo(ctx context.Context, arg store.GetRunRepoParams) (store.RunRepo, error)
	GetModRepo(ctx context.Context, id domaintypes.ModRepoID) (store.ModRepo, error)
	UpdateRunRepoStatus(ctx context.Context, params store.UpdateRunRepoStatusParams) error
	ListJobsByRunRepoAttempt(ctx context.Context, arg store.ListJobsByRunRepoAttemptParams) ([]store.Job, error)
	UpdateJobStatus(ctx context.Context, params store.UpdateJobStatusParams) error
}

type restartRunRepoStore interface {
	GetRun(ctx context.Context, id domaintypes.RunID) (store.Run, error)
	GetRunRepo(ctx context.Context, arg store.GetRunRepoParams) (store.RunRepo, error)
	UpdateRunStatus(ctx context.Context, params store.UpdateRunStatusParams) error
	UpdateRunRepoRefs(ctx context.Context, params store.UpdateRunRepoRefsParams) error
	UpdateModRepoRefs(ctx context.Context, params store.UpdateModRepoRefsParams) error
	IncrementRunRepoAttempt(ctx context.Context, arg store.IncrementRunRepoAttemptParams) error
	GetSpec(ctx context.Context, id domaintypes.SpecID) (store.Spec, error)
	CreateJob(ctx context.Context, params store.CreateJobParams) (store.Job, error)
	GetModRepo(ctx context.Context, id domaintypes.ModRepoID) (store.ModRepo, error)
}

type startRunStore interface {
	GetRun(ctx context.Context, id domaintypes.RunID) (store.Run, error)
	GetSpec(ctx context.Context, id domaintypes.SpecID) (store.Spec, error)
	ListRunReposByRun(ctx context.Context, runID domaintypes.RunID) ([]store.RunRepo, error)
	ListQueuedRunReposByRun(ctx context.Context, runID domaintypes.RunID) ([]store.RunRepo, error)
	ListJobsByRunRepoAttempt(ctx context.Context, arg store.ListJobsByRunRepoAttemptParams) ([]store.Job, error)
	UpdateRunRepoError(ctx context.Context, params store.UpdateRunRepoErrorParams) error
	ScheduleNextJob(ctx context.Context, arg store.ScheduleNextJobParams) (store.Job, error)
	CreateJob(ctx context.Context, params store.CreateJobParams) (store.Job, error)
}

// cancelRunHandlerV1 returns an HTTP handler that cancels a v1 run.
// POST /v1/runs/{id}/cancel (v1 API) — Performs transactional cancellation of a run.
// This handler delegates cancellation to store.CancelRunV1, which atomically:
// - Sets runs.status=Cancelled (for non-terminal runs)
// - Cancels all repos with status Queued or Running → Cancelled
// - Cancels waiting/running jobs (Created/Queued/Running → Cancelled)
func cancelRunHandlerV1(st cancelRunStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		runID, err := domaintypes.ParseRunIDParam(r, "id")
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		run, err := st.GetRun(r.Context(), runID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				http.Error(w, "run not found", http.StatusNotFound)
				return
			}
			http.Error(w, fmt.Sprintf("failed to get run: %v", err), http.StatusInternalServerError)
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
			http.Error(w, fmt.Sprintf("failed to cancel run: %v", err), http.StatusInternalServerError)
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

// addRunRepoHandler adds a repo to an existing run (and to the mod repo set).
// POST /v1/runs/{id}/repos — Body {repo_url, base_ref, target_ref}.
func addRunRepoHandler(st addRunRepoStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		runID, err := domaintypes.ParseRunIDParam(r, "id")
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		run, err := st.GetRun(r.Context(), runID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				http.Error(w, "run not found", http.StatusNotFound)
				return
			}
			http.Error(w, fmt.Sprintf("failed to get run: %v", err), http.StatusInternalServerError)
			slog.Error("add run repo: get run failed", "run_id", runID.String(), "err", err)
			return
		}
		if run.Status == store.RunStatusFinished || run.Status == store.RunStatusCancelled {
			http.Error(w, "cannot add repos to a terminal run", http.StatusConflict)
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
			http.Error(w, fmt.Sprintf("repo_url: %v", err), http.StatusBadRequest)
			return
		}
		if err := req.BaseRef.Validate(); err != nil {
			http.Error(w, fmt.Sprintf("base_ref: %v", err), http.StatusBadRequest)
			return
		}
		if err := req.TargetRef.Validate(); err != nil {
			http.Error(w, fmt.Sprintf("target_ref: %v", err), http.StatusBadRequest)
			return
		}

		modRepoID := domaintypes.NewModRepoID()
		modRepo, err := st.CreateModRepo(r.Context(), store.CreateModRepoParams{
			ID:        modRepoID,
			ModID:     run.ModID,
			RepoUrl:   req.RepoURL.String(),
			BaseRef:   req.BaseRef.String(),
			TargetRef: req.TargetRef.String(),
		})
		if err != nil {
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code == "23505" {
				http.Error(w, "repo already exists for mod", http.StatusConflict)
				return
			}
			http.Error(w, fmt.Sprintf("failed to create mod repo: %v", err), http.StatusInternalServerError)
			slog.Error("add run repo: create mod repo failed", "run_id", runID.String(), "err", err)
			return
		}

		runRepo, err := st.CreateRunRepo(r.Context(), store.CreateRunRepoParams{
			ModID:         run.ModID,
			RunID:         runID,
			RepoID:        modRepo.ID,
			RepoBaseRef:   modRepo.BaseRef,
			RepoTargetRef: modRepo.TargetRef,
		})
		if err != nil {
			http.Error(w, fmt.Sprintf("failed to create run repo: %v", err), http.StatusInternalServerError)
			slog.Error("add run repo: create run repo failed", "run_id", runID.String(), "repo_id", modRepo.ID, "err", err)
			return
		}

		// v1 immediate start: create repo-scoped jobs for the new queued repo.
		spec, err := st.GetSpec(r.Context(), run.SpecID)
		if err != nil {
			http.Error(w, fmt.Sprintf("failed to load spec: %v", err), http.StatusInternalServerError)
			slog.Error("add run repo: get spec failed", "run_id", runID.String(), "spec_id", run.SpecID, "err", err)
			return
		}
		if err := createJobsFromSpec(r.Context(), st, run.ID, runRepo.RepoID, runRepo.RepoBaseRef, runRepo.Attempt, spec.Spec); err != nil {
			http.Error(w, fmt.Sprintf("failed to create jobs: %v", err), http.StatusInternalServerError)
			slog.Error("add run repo: create jobs failed", "run_id", runID.String(), "repo_id", runRepo.RepoID, "err", err)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(runRepoToResponse(runRepo, modRepo.RepoUrl))
	}
}

// listRunReposHandler lists repos for a run.
// GET /v1/runs/{id}/repos
func listRunReposHandler(st listRunReposStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		runID, err := domaintypes.ParseRunIDParam(r, "id")
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		repos, err := st.ListRunReposWithURLByRun(r.Context(), runID)
		if err != nil {
			http.Error(w, fmt.Sprintf("failed to list run repos: %v", err), http.StatusInternalServerError)
			slog.Error("list run repos: fetch failed", "run_id", runID.String(), "err", err)
			return
		}

		reposResp := make([]RunRepoResponse, 0, len(repos))
		for _, rr := range repos {
			reposResp = append(reposResp, runRepoToResponse(store.RunRepo{
				ModID:         rr.ModID,
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
func cancelRunRepoHandlerV1(st cancelRunRepoStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		runID, err := domaintypes.ParseRunIDParam(r, "run_id")
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		repoID, err := domaintypes.ParseModRepoIDParam(r, "repo_id")
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		rr, err := st.GetRunRepo(r.Context(), store.GetRunRepoParams{RunID: runID, RepoID: repoID})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				http.Error(w, "repo not found", http.StatusNotFound)
				return
			}
			http.Error(w, fmt.Sprintf("failed to get repo: %v", err), http.StatusInternalServerError)
			slog.Error("cancel run repo: get repo failed", "run_id", runID.String(), "repo_id", repoID.String(), "err", err)
			return
		}

		if rr.Status == store.RunRepoStatusCancelled || rr.Status == store.RunRepoStatusSuccess || rr.Status == store.RunRepoStatusFail {
			repoURL := ""
			if mr, err := st.GetModRepo(r.Context(), rr.RepoID); err == nil {
				repoURL = mr.RepoUrl
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
		if mr, err := st.GetModRepo(r.Context(), rr.RepoID); err == nil {
			repoURL = mr.RepoUrl
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(runRepoToResponse(rr, repoURL))
	}
}

// restartRunRepoHandler restarts a repo execution by incrementing attempt and creating new repo-scoped jobs.
// POST /v1/runs/{id}/repos/{repo_id}/restart
func restartRunRepoHandler(st restartRunRepoStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		runID, err := domaintypes.ParseRunIDParam(r, "id")
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		repoID, err := domaintypes.ParseModRepoIDParam(r, "repo_id")
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		run, err := st.GetRun(r.Context(), runID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				http.Error(w, "run not found", http.StatusNotFound)
				return
			}
			http.Error(w, fmt.Sprintf("failed to get run: %v", err), http.StatusInternalServerError)
			slog.Error("restart run repo: get run failed", "run_id", runID.String(), "err", err)
			return
		}

		runRepo, err := st.GetRunRepo(r.Context(), store.GetRunRepoParams{RunID: runID, RepoID: repoID})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				http.Error(w, "repo not found", http.StatusNotFound)
				return
			}
			http.Error(w, fmt.Sprintf("failed to get repo: %v", err), http.StatusInternalServerError)
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
					http.Error(w, fmt.Sprintf("base_ref: %v", err), http.StatusBadRequest)
					return
				}
			}
			if req.TargetRef != nil {
				if err := req.TargetRef.Validate(); err != nil {
					http.Error(w, fmt.Sprintf("target_ref: %v", err), http.StatusBadRequest)
					return
				}
			}
		}

		// If the run is terminal, reopen it to Started for the restart attempt.
		if run.Status == store.RunStatusFinished || run.Status == store.RunStatusCancelled {
			if err := st.UpdateRunStatus(r.Context(), store.UpdateRunStatusParams{ID: runID, Status: store.RunStatusStarted}); err != nil {
				http.Error(w, fmt.Sprintf("failed to reopen run: %v", err), http.StatusInternalServerError)
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
			_ = st.UpdateModRepoRefs(r.Context(), store.UpdateModRepoRefsParams{ID: repoID, BaseRef: newBase, TargetRef: newTarget})
		}

		if err := st.IncrementRunRepoAttempt(r.Context(), store.IncrementRunRepoAttemptParams{RunID: runID, RepoID: repoID}); err != nil {
			http.Error(w, fmt.Sprintf("failed to restart repo: %v", err), http.StatusInternalServerError)
			slog.Error("restart run repo: increment attempt failed", "run_id", runID.String(), "repo_id", repoID.String(), "err", err)
			return
		}

		runRepo, err = st.GetRunRepo(r.Context(), store.GetRunRepoParams{RunID: runID, RepoID: repoID})
		if err != nil {
			http.Error(w, fmt.Sprintf("failed to reload repo: %v", err), http.StatusInternalServerError)
			return
		}

		spec, err := st.GetSpec(r.Context(), run.SpecID)
		if err != nil {
			http.Error(w, fmt.Sprintf("failed to load spec: %v", err), http.StatusInternalServerError)
			return
		}
		if err := createJobsFromSpec(r.Context(), st, runID, runRepo.RepoID, runRepo.RepoBaseRef, runRepo.Attempt, spec.Spec); err != nil {
			http.Error(w, fmt.Sprintf("failed to create jobs: %v", err), http.StatusInternalServerError)
			return
		}

		repoURL := ""
		if mr, err := st.GetModRepo(r.Context(), runRepo.RepoID); err == nil {
			repoURL = mr.RepoUrl
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
func startRunHandler(st startRunStore) http.HandlerFunc {
	starter := NewBatchRepoStarter(st)

	return func(w http.ResponseWriter, r *http.Request) {
		runID, err := domaintypes.ParseRunIDParam(r, "id")
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		run, err := st.GetRun(r.Context(), runID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				http.Error(w, "run not found", http.StatusNotFound)
				return
			}
			http.Error(w, fmt.Sprintf("failed to get run: %v", err), http.StatusInternalServerError)
			slog.Error("start run: fetch failed", "run_id", runID.String(), "err", err)
			return
		}

		if isTerminalRunStatus(run.Status) {
			http.Error(w, "cannot start repos in a terminal run", http.StatusConflict)
			return
		}

		result, err := starter.StartPendingRepos(r.Context(), runID)
		if err != nil {
			http.Error(w, fmt.Sprintf("failed to start queued repos: %v", err), http.StatusInternalServerError)
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
