package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/iw2rmb/ploy/internal/store"
)

// submitTicketHandler returns an HTTP handler that submits a new ticket (mods run).
// POST /v1/mods — Accepts TicketSubmitRequest, returns TicketSummary (ticket_id == run UUID).
// Accepts repo URL/refs directly (no pre-registered mod/repo required).
func submitTicketHandler(st store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Decode request body.
		var req struct {
			RepoURL   string           `json:"repo_url"`
			BaseRef   string           `json:"base_ref"`
			TargetRef string           `json:"target_ref"`
			CommitSha *string          `json:"commit_sha,omitempty"`
			Spec      *json.RawMessage `json:"spec,omitempty"`
			CreatedBy *string          `json:"created_by,omitempty"`
		}

		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, fmt.Sprintf("invalid request: %v", err), http.StatusBadRequest)
			return
		}

		// Validate required fields.
		if strings.TrimSpace(req.RepoURL) == "" {
			http.Error(w, "repo_url field is required", http.StatusBadRequest)
			return
		}
		if strings.TrimSpace(req.BaseRef) == "" {
			http.Error(w, "base_ref field is required", http.StatusBadRequest)
			return
		}
		if strings.TrimSpace(req.TargetRef) == "" {
			http.Error(w, "target_ref field is required", http.StatusBadRequest)
			return
		}

		// Get or create repo by URL.
		repo, err := st.GetRepoByURL(r.Context(), req.RepoURL)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				// Repo does not exist; create it.
				repo, err = st.CreateRepo(r.Context(), store.CreateRepoParams{
					Url:       req.RepoURL,
					Branch:    nil, // No default branch hint for ticket-submitted repos.
					CommitSha: nil, // No default commit for ticket-submitted repos.
				})
				if err != nil {
					// Check if this is a unique constraint violation (race condition).
					var pgErr *pgconn.PgError
					if errors.As(err, &pgErr) && pgErr.Code == "23505" { // unique_violation
						// Retry the get; another request just created the repo.
						repo, err = st.GetRepoByURL(r.Context(), req.RepoURL)
						if err != nil {
							http.Error(w, fmt.Sprintf("failed to get repo after race: %v", err), http.StatusInternalServerError)
							slog.Error("submit ticket: get repo after race failed", "repo_url", req.RepoURL, "err", err)
							return
						}
					} else {
						http.Error(w, fmt.Sprintf("failed to create repo: %v", err), http.StatusInternalServerError)
						slog.Error("submit ticket: create repo failed", "repo_url", req.RepoURL, "err", err)
						return
					}
				}
			} else {
				http.Error(w, fmt.Sprintf("failed to get repo: %v", err), http.StatusInternalServerError)
				slog.Error("submit ticket: get repo failed", "repo_url", req.RepoURL, "err", err)
				return
			}
		}

		// Create mod with the repo_id and spec (default to empty JSON object if not provided).
		spec := []byte("{}")
		if req.Spec != nil && len(*req.Spec) > 0 {
			spec = *req.Spec
		}
		mod, err := st.CreateMod(r.Context(), store.CreateModParams{
			RepoID:    repo.ID,
			Spec:      spec,
			CreatedBy: req.CreatedBy,
		})
		if err != nil {
			http.Error(w, fmt.Sprintf("failed to create mod: %v", err), http.StatusInternalServerError)
			slog.Error("submit ticket: create mod failed", "repo_id", uuid.UUID(repo.ID.Bytes).String(), "err", err)
			return
		}

		// Create the run with status=queued.
		run, err := st.CreateRun(r.Context(), store.CreateRunParams{
			ModID:     mod.ID,
			Status:    store.RunStatusQueued,
			BaseRef:   req.BaseRef,
			TargetRef: req.TargetRef,
			CommitSha: req.CommitSha,
		})
		if err != nil {
			http.Error(w, fmt.Sprintf("failed to create run: %v", err), http.StatusInternalServerError)
			slog.Error("submit ticket: create run failed", "mod_id", uuid.UUID(mod.ID.Bytes).String(), "err", err)
			return
		}

		// Build response with TicketSummary (ticket_id == run UUID).
		resp := struct {
			TicketID  string `json:"ticket_id"`
			Status    string `json:"status"`
			RepoURL   string `json:"repo_url"`
			BaseRef   string `json:"base_ref"`
			TargetRef string `json:"target_ref"`
		}{
			TicketID:  uuid.UUID(run.ID.Bytes).String(),
			Status:    string(run.Status),
			RepoURL:   repo.Url,
			BaseRef:   run.BaseRef,
			TargetRef: run.TargetRef,
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			slog.Error("submit ticket: encode response failed", "err", err)
		}

		slog.Info("ticket submitted",
			"ticket_id", resp.TicketID,
			"repo_url", req.RepoURL,
			"base_ref", req.BaseRef,
			"target_ref", req.TargetRef,
			"status", "queued",
		)
	}
}
