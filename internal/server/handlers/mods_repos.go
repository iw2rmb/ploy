package handlers

import (
	"bufio"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
	"github.com/iw2rmb/ploy/internal/vcs"
)

// addModRepoHandler adds a repo to a mod's repo set.
// Endpoint: POST /v1/mods/{mod_id}/repos
// Request: {repo_url, base_ref, target_ref}
// Response: 201 Created with repo details
//
// v1 contract:
// - Adds/enables a repo in a mod.
// - Normalizes repo_url for matching (strips trailing slashes and .git suffixes).
// - Returns id (repo_id) and stored fields.
func addModRepoHandler(st store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		modIDStr, err := requiredPathParam(r, "mod_id")
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		modID := domaintypes.ModID(modIDStr)

		// Parse request body.
		var req struct {
			RepoURL   string `json:"repo_url"`
			BaseRef   string `json:"base_ref"`
			TargetRef string `json:"target_ref"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, fmt.Sprintf("invalid request: %v", err), http.StatusBadRequest)
			return
		}

		// Validate required fields.
		if req.RepoURL == "" {
			http.Error(w, "repo_url is required", http.StatusBadRequest)
			return
		}
		if req.BaseRef == "" {
			http.Error(w, "base_ref is required", http.StatusBadRequest)
			return
		}
		if req.TargetRef == "" {
			http.Error(w, "target_ref is required", http.StatusBadRequest)
			return
		}

		// Normalize and validate repo URL (strips trailing slashes and .git suffixes).
		normalizedURL := vcs.NormalizeRepoURL(req.RepoURL)
		if err := domaintypes.RepoURL(normalizedURL).Validate(); err != nil {
			http.Error(w, fmt.Sprintf("repo_url: %v", err), http.StatusBadRequest)
			return
		}

		// Verify mod exists and is not archived.
		mod, err := st.GetMod(r.Context(), modID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				http.Error(w, "mod not found", http.StatusNotFound)
				return
			}
			http.Error(w, fmt.Sprintf("failed to get mod: %v", err), http.StatusInternalServerError)
			slog.Error("add mod repo: get mod failed", "mod_id", modIDStr, "err", err)
			return
		}
		if mod.ArchivedAt.Valid {
			http.Error(w, "cannot add repo to archived mod", http.StatusConflict)
			return
		}

		// Create the mod_repo row.
		repoID := domaintypes.NewModRepoID()
		repo, err := st.CreateModRepo(r.Context(), store.CreateModRepoParams{
			ID:        repoID,
			ModID:     modID,
			RepoUrl:   normalizedURL,
			BaseRef:   req.BaseRef,
			TargetRef: req.TargetRef,
		})
		if err != nil {
			// Check for unique constraint violation (duplicate repo_url in mod).
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code == "23505" {
				http.Error(w, "repo already exists in this mod", http.StatusConflict)
				return
			}
			http.Error(w, fmt.Sprintf("failed to create mod repo: %v", err), http.StatusInternalServerError)
			slog.Error("add mod repo: create failed", "mod_id", modIDStr, "repo_url", normalizedURL, "err", err)
			return
		}

		// Build response with repo details.
		resp := struct {
			ID        string `json:"id"`
			ModID     string `json:"mod_id"`
			RepoURL   string `json:"repo_url"`
			BaseRef   string `json:"base_ref"`
			TargetRef string `json:"target_ref"`
			CreatedAt string `json:"created_at"`
		}{
			ID:        repo.ID.String(),
			ModID:     repo.ModID.String(),
			RepoURL:   repo.RepoUrl,
			BaseRef:   repo.BaseRef,
			TargetRef: repo.TargetRef,
			CreatedAt: repo.CreatedAt.Time.Format("2006-01-02T15:04:05Z07:00"),
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			slog.Error("add mod repo: encode response failed", "err", err)
		}

		slog.Info("mod repo added", "mod_id", modIDStr, "repo_id", repo.ID, "repo_url", normalizedURL)
	}
}

// listModReposHandler lists repos in a mod's repo set.
// Endpoint: GET /v1/mods/{mod_id}/repos
// Response: 200 OK with list of repos
//
// v1 contract:
// - Lists repos: ID, REPO_URL, BASE_REF, TARGET_REF, ADDED_AT.
func listModReposHandler(st store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		modIDStr, err := requiredPathParam(r, "mod_id")
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		modID := domaintypes.ModID(modIDStr)

		// Verify mod exists.
		_, err = st.GetMod(r.Context(), modID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				http.Error(w, "mod not found", http.StatusNotFound)
				return
			}
			http.Error(w, fmt.Sprintf("failed to get mod: %v", err), http.StatusInternalServerError)
			slog.Error("list mod repos: get mod failed", "mod_id", modIDStr, "err", err)
			return
		}

		// List repos for this mod.
		repos, err := st.ListModReposByMod(r.Context(), modID)
		if err != nil {
			http.Error(w, fmt.Sprintf("failed to list mod repos: %v", err), http.StatusInternalServerError)
			slog.Error("list mod repos: list failed", "mod_id", modIDStr, "err", err)
			return
		}

		type repoItem struct {
			ID        string `json:"id"`
			ModID     string `json:"mod_id"`
			RepoURL   string `json:"repo_url"`
			BaseRef   string `json:"base_ref"`
			TargetRef string `json:"target_ref"`
			CreatedAt string `json:"created_at"`
		}

		items := make([]repoItem, 0, len(repos))
		for _, repo := range repos {
			items = append(items, repoItem{
				ID:        repo.ID.String(),
				ModID:     repo.ModID.String(),
				RepoURL:   repo.RepoUrl,
				BaseRef:   repo.BaseRef,
				TargetRef: repo.TargetRef,
				CreatedAt: repo.CreatedAt.Time.Format("2006-01-02T15:04:05Z07:00"),
			})
		}

		resp := struct {
			Repos []repoItem `json:"repos"`
		}{Repos: items}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			slog.Error("list mod repos: encode response failed", "err", err)
		}
	}
}

// deleteModRepoHandler deletes a repo from a mod's repo set.
// Endpoint: DELETE /v1/mods/{mod_id}/repos/{repo_id}
// Response: 204 No Content on success
//
// v1 contract:
// - Deletes a repo from the mod repo set.
// - Refuse deletion if the repo has historical executions (run_repos.repo_id references).
func deleteModRepoHandler(st store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		modIDStr, err := requiredPathParam(r, "mod_id")
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		repoIDStr, err := requiredPathParam(r, "repo_id")
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		modID := domaintypes.ModID(modIDStr)
		repoID := domaintypes.ModRepoID(repoIDStr)

		// Verify mod exists.
		_, err = st.GetMod(r.Context(), modID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				http.Error(w, "mod not found", http.StatusNotFound)
				return
			}
			http.Error(w, fmt.Sprintf("failed to get mod: %v", err), http.StatusInternalServerError)
			slog.Error("delete mod repo: get mod failed", "mod_id", modIDStr, "err", err)
			return
		}

		// Verify repo exists and belongs to this mod.
		repo, err := st.GetModRepo(r.Context(), repoID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				http.Error(w, "repo not found", http.StatusNotFound)
				return
			}
			http.Error(w, fmt.Sprintf("failed to get repo: %v", err), http.StatusInternalServerError)
			slog.Error("delete mod repo: get repo failed", "repo_id", repoIDStr, "err", err)
			return
		}
		if repo.ModID != modID {
			http.Error(w, "repo does not belong to this mod", http.StatusNotFound)
			return
		}

		// Check if repo has historical executions (run_repos references).
		hasHistory, err := st.HasModRepoHistory(r.Context(), repoID)
		if err != nil {
			http.Error(w, fmt.Sprintf("failed to check repo history: %v", err), http.StatusInternalServerError)
			slog.Error("delete mod repo: check history failed", "repo_id", repoIDStr, "err", err)
			return
		}
		if hasHistory {
			http.Error(w, "cannot delete repo with historical executions", http.StatusConflict)
			return
		}

		// Delete the repo.
		if err := st.DeleteModRepo(r.Context(), repoID); err != nil {
			http.Error(w, fmt.Sprintf("failed to delete mod repo: %v", err), http.StatusInternalServerError)
			slog.Error("delete mod repo: delete failed", "repo_id", repoIDStr, "err", err)
			return
		}

		w.WriteHeader(http.StatusNoContent)
		slog.Info("mod repo deleted", "mod_id", modIDStr, "repo_id", repoIDStr)
	}
}

// bulkUpsertModReposHandler bulk upserts repos for a mod from CSV.
// Endpoint: POST /v1/mods/{mod_id}/repos/bulk
// Request: Content-Type: text/csv; body is UTF-8 CSV with header row: repo_url,base_ref,target_ref
// Response: 200 OK with counts {created, updated, failed} and errors array
//
// v1 contract:
// - Continues on per-line errors; may partially apply.
// - Upserts by (mod_id, repo_url): inserts new rows, updates refs for existing.
// - Does not affect historical run data (run_repos snapshots remain unchanged).
// - CSV parsing rules:
//   - delimiter: ,
//   - UTF-8 text; unicode allowed
//   - fields may be quoted with " (CSV-style)
//   - within quoted fields, " is escaped as ""
func bulkUpsertModReposHandler(st store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		modIDStr, err := requiredPathParam(r, "mod_id")
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		modID := domaintypes.ModID(modIDStr)

		// Verify mod exists and is not archived.
		mod, err := st.GetMod(r.Context(), modID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				http.Error(w, "mod not found", http.StatusNotFound)
				return
			}
			http.Error(w, fmt.Sprintf("failed to get mod: %v", err), http.StatusInternalServerError)
			slog.Error("bulk upsert mod repos: get mod failed", "mod_id", modIDStr, "err", err)
			return
		}
		if mod.ArchivedAt.Valid {
			http.Error(w, "cannot modify repos on archived mod", http.StatusConflict)
			return
		}

		// Validate Content-Type is text/csv.
		contentType := r.Header.Get("Content-Type")
		if !strings.HasPrefix(contentType, "text/csv") {
			http.Error(w, "Content-Type must be text/csv", http.StatusBadRequest)
			return
		}

		// Parse CSV body.
		reader := csv.NewReader(bufio.NewReader(r.Body))
		reader.FieldsPerRecord = 3 // repo_url, base_ref, target_ref
		reader.LazyQuotes = false  // Strict CSV parsing
		reader.TrimLeadingSpace = false

		// Collect results.
		var created, updated, failed int
		type lineError struct {
			Line    int    `json:"line"`
			Message string `json:"message"`
		}
		errs := make([]lineError, 0)

		lineNum := 0
		headerRead := false

		for {
			record, err := reader.Read()
			if err == io.EOF {
				break
			}
			lineNum++

			// Skip header row (first line).
			if !headerRead {
				if err != nil {
					http.Error(w, fmt.Sprintf("CSV parse error in header: %v", err), http.StatusBadRequest)
					return
				}
				headerRead = true
				// Validate header row has expected columns.
				if len(record) != 3 || strings.ToLower(strings.TrimSpace(record[0])) != "repo_url" ||
					strings.ToLower(strings.TrimSpace(record[1])) != "base_ref" ||
					strings.ToLower(strings.TrimSpace(record[2])) != "target_ref" {
					http.Error(w, "CSV header must be: repo_url,base_ref,target_ref", http.StatusBadRequest)
					return
				}
				continue
			}

			if err != nil {
				failed++
				errs = append(errs, lineError{Line: lineNum, Message: fmt.Sprintf("CSV parse error: %v", err)})
				continue
			}

			// Parse CSV row.
			repoURL := strings.TrimSpace(record[0])
			baseRef := strings.TrimSpace(record[1])
			targetRef := strings.TrimSpace(record[2])

			// Validate UTF-8.
			if !utf8.ValidString(repoURL) || !utf8.ValidString(baseRef) || !utf8.ValidString(targetRef) {
				failed++
				errs = append(errs, lineError{Line: lineNum, Message: "invalid UTF-8 encoding"})
				continue
			}

			// Validate required fields.
			if repoURL == "" {
				failed++
				errs = append(errs, lineError{Line: lineNum, Message: "repo_url is required"})
				continue
			}
			if baseRef == "" {
				failed++
				errs = append(errs, lineError{Line: lineNum, Message: "base_ref is required"})
				continue
			}
			if targetRef == "" {
				failed++
				errs = append(errs, lineError{Line: lineNum, Message: "target_ref is required"})
				continue
			}

			// Normalize and validate repo URL.
			normalizedURL := vcs.NormalizeRepoURL(repoURL)
			if err := domaintypes.RepoURL(normalizedURL).Validate(); err != nil {
				failed++
				errs = append(errs, lineError{Line: lineNum, Message: fmt.Sprintf("invalid repo_url: %v", err)})
				continue
			}

			// Check if repo already exists to determine if this is create or update.
			_, err = st.GetModRepoByURL(r.Context(), store.GetModRepoByURLParams{
				ModID:   modID,
				RepoUrl: normalizedURL,
			})
			isUpdate := err == nil
			if err != nil && !errors.Is(err, pgx.ErrNoRows) {
				failed++
				errs = append(errs, lineError{Line: lineNum, Message: fmt.Sprintf("lookup failed: %v", err)})
				continue
			}

			// Upsert the repo.
			_, err = st.UpsertModRepo(r.Context(), store.UpsertModRepoParams{
				ID:        domaintypes.NewModRepoID(), // Only used for insert
				ModID:     modID,
				RepoUrl:   normalizedURL,
				BaseRef:   baseRef,
				TargetRef: targetRef,
			})
			if err != nil {
				failed++
				errs = append(errs, lineError{Line: lineNum, Message: fmt.Sprintf("upsert failed: %v", err)})
				continue
			}

			if isUpdate {
				updated++
			} else {
				created++
			}
		}

		if !headerRead {
			http.Error(w, "CSV file is empty or missing header", http.StatusBadRequest)
			return
		}

		// Build response with counts and any errors.
		resp := struct {
			Created int         `json:"created"`
			Updated int         `json:"updated"`
			Failed  int         `json:"failed"`
			Errors  []lineError `json:"errors"`
		}{
			Created: created,
			Updated: updated,
			Failed:  failed,
			Errors:  errs,
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			slog.Error("bulk upsert mod repos: encode response failed", "err", err)
		}

		slog.Info("bulk upsert mod repos completed",
			"mod_id", modIDStr,
			"created", created,
			"updated", updated,
			"failed", failed,
		)
	}
}
