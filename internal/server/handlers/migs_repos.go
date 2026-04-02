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

	domainapi "github.com/iw2rmb/ploy/internal/domain/api"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
)

// addMigRepoHandler adds a repo to a mig's repo set.
// Endpoint: POST /v1/migs/{mig_id}/repos
// Request: {repo_url, base_ref, target_ref}
// Response: 201 Created with repo details
//
// v1 contract:
// - Adds/enables a repo in a mig.
// - Normalizes repo_url for matching (strips trailing slashes and .git suffixes).
// - Returns id (repo_id) and stored fields.
func addMigRepoHandler(st store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Parse request body with strict validation.
		var req struct {
			RepoURL   domaintypes.RepoURL `json:"repo_url"`
			BaseRef   domaintypes.GitRef  `json:"base_ref"`
			TargetRef domaintypes.GitRef  `json:"target_ref"`
		}
		if err := decodeRequestJSON(w, r, &req, DefaultMaxBodySize); err != nil {
			return
		}

		// Normalize and validate using domain types.
		normalizedURL := domaintypes.NormalizeRepoURL(string(req.RepoURL))
		req.RepoURL = domaintypes.RepoURL(normalizedURL)
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

		// Verify mig exists and is not archived.
		mig, ok := getMigByIDOrFail(w, r, st, "add mig repo")
		if !ok {
			return
		}
		migID := mig.ID
		if mig.ArchivedAt.Valid {
			writeHTTPError(w, http.StatusConflict, "cannot add repo to archived mig")
			return
		}

		// Create the mig_repo row.
		repoID := domaintypes.NewMigRepoID()
		repo, err := st.CreateMigRepo(r.Context(), store.CreateMigRepoParams{
			ID:        repoID,
			MigID:     migID,
			Url:       normalizedURL,
			BaseRef:   req.BaseRef.String(),
			TargetRef: req.TargetRef.String(),
		})
		if err != nil {
			// Check for unique constraint violation (duplicate repo_url in mig).
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code == "23505" {
				writeHTTPError(w, http.StatusConflict, "repo already exists in this mig")
				return
			}
			writeHTTPError(w, http.StatusInternalServerError, "failed to create mig repo: %v", err)
			slog.Error("add mig repo: create failed", "mig_id", migID, "repo_url", normalizedURL, "err", err)
			return
		}

		writeJSON(w, http.StatusCreated, domainapi.MigRepoSummary{
			ID:        repo.ID,
			MigID:     repo.MigID,
			RepoURL:   normalizedURL,
			BaseRef:   repo.BaseRef,
			TargetRef: repo.TargetRef,
			CreatedAt: repo.CreatedAt.Time,
		})

		slog.Info("mig repo added", "mig_id", migID, "repo_id", repo.ID, "repo_url", normalizedURL)
	}
}

// listMigReposHandler lists repos in a mig's repo set.
// Endpoint: GET /v1/migs/{mig_id}/repos
// Response: 200 OK with list of repos
//
// v1 contract:
// - Lists repos: ID, REPO_URL, BASE_REF, TARGET_REF, ADDED_AT.
func listMigReposHandler(st store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		mig, ok := getMigByIDOrFail(w, r, st, "list mig repos")
		if !ok {
			return
		}
		migID := mig.ID

		// List repos for this mig.
		repos, err := st.ListMigReposByMig(r.Context(), migID)
		if err != nil {
			writeHTTPError(w, http.StatusInternalServerError, "failed to list mig repos: %v", err)
			slog.Error("list mig repos: list failed", "mig_id", migID, "err", err)
			return
		}

		items := make([]domainapi.MigRepoSummary, 0, len(repos))
		for _, repo := range repos {
			repoURL, err := repoURLForID(r.Context(), st, repo.RepoID)
			if err != nil {
				writeHTTPError(w, http.StatusInternalServerError, "failed to get repo: %v", err)
				slog.Error("list mig repos: get repo failed", "mig_id", migID, "repo_id", repo.RepoID, "err", err)
				return
			}
			items = append(items, domainapi.MigRepoSummary{
				ID:        repo.ID,
				MigID:     repo.MigID,
				RepoURL:   repoURL,
				BaseRef:   repo.BaseRef,
				TargetRef: repo.TargetRef,
				CreatedAt: repo.CreatedAt.Time,
			})
		}

		writeJSON(w, http.StatusOK, domainapi.MigRepoListResponse{Repos: items})
	}
}

// deleteMigRepoHandler deletes a repo from a mig's repo set.
// Endpoint: DELETE /v1/migs/{mig_id}/repos/{repo_id}
// Response: 204 No Content on success
//
// v1 contract:
// - Deletes a repo from the mig repo set.
// - Refuse deletion if the repo has historical executions (run_repos.repo_id references).
func deleteMigRepoHandler(st store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		mig, ok := getMigByIDOrFail(w, r, st, "delete mig repo")
		if !ok {
			return
		}
		migID := mig.ID

		repoID, err := parseRequiredPathID[domaintypes.MigRepoID](r, "repo_id")
		if err != nil {
			writeHTTPError(w, http.StatusBadRequest, "%s", err)
			return
		}

		// Verify repo exists and belongs to this mig.
		repo, err := st.GetMigRepo(r.Context(), repoID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				writeHTTPError(w, http.StatusNotFound, "repo not found")
				return
			}
			writeHTTPError(w, http.StatusInternalServerError, "failed to get repo: %v", err)
			slog.Error("delete mig repo: get repo failed", "repo_id", repoID, "err", err)
			return
		}
		if repo.MigID != migID {
			writeHTTPError(w, http.StatusNotFound, "repo does not belong to this mig")
			return
		}

		// Check if repo has historical executions (run_repos references).
		hasHistory, err := st.HasMigRepoHistory(r.Context(), repo.RepoID)
		if err != nil {
			writeHTTPError(w, http.StatusInternalServerError, "failed to check repo history: %v", err)
			slog.Error("delete mig repo: check history failed", "repo_id", repoID, "err", err)
			return
		}
		if hasHistory {
			writeHTTPError(w, http.StatusConflict, "cannot delete repo with historical executions")
			return
		}

		// Delete the repo.
		if err := st.DeleteMigRepo(r.Context(), repoID); err != nil {
			writeHTTPError(w, http.StatusInternalServerError, "failed to delete mig repo: %v", err)
			slog.Error("delete mig repo: delete failed", "repo_id", repoID, "err", err)
			return
		}

		w.WriteHeader(http.StatusNoContent)
		slog.Info("mig repo deleted", "mig_id", migID, "repo_id", repoID)
	}
}

// bulkUpsertMigReposHandler bulk upserts repos for a mig from CSV.
// Endpoint: POST /v1/migs/{mig_id}/repos/bulk
// Request: Content-Type: text/csv; body is UTF-8 CSV with header row: repo_url,base_ref,target_ref
// Response: 200 OK with counts {created, updated, failed} and errors array
//
// v1 contract:
// - Continues on per-line errors; may partially apply.
// - Upserts by (mig_id, repo_url): inserts new rows, updates refs for existing.
// - Does not affect historical run data (run_repos snapshots remain unchanged).
// - CSV parsing rules:
//   - delimiter: ,
//   - UTF-8 text; unicode allowed
//   - fields may be quoted with " (CSV-style)
//   - within quoted fields, " is escaped as ""
func bulkUpsertMigReposHandler(st store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		mig, ok := getMigByIDOrFail(w, r, st, "bulk upsert mig repos")
		if !ok {
			return
		}
		migID := mig.ID
		if mig.ArchivedAt.Valid {
			writeHTTPError(w, http.StatusConflict, "cannot modify repos on archived mig")
			return
		}

		// Validate Content-Type is text/csv.
		contentType := r.Header.Get("Content-Type")
		if !strings.HasPrefix(contentType, "text/csv") {
			writeHTTPError(w, http.StatusBadRequest, "Content-Type must be text/csv")
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
					writeHTTPError(w, http.StatusBadRequest, "CSV parse error in header: %v", err)
					return
				}
				headerRead = true
				// Validate header row has expected columns.
				if len(record) != 3 || strings.ToLower(strings.TrimSpace(record[0])) != "repo_url" ||
					strings.ToLower(strings.TrimSpace(record[1])) != "base_ref" ||
					strings.ToLower(strings.TrimSpace(record[2])) != "target_ref" {
					writeHTTPError(w, http.StatusBadRequest, "CSV header must be: repo_url,base_ref,target_ref")
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
			normalizedURL := domaintypes.NormalizeRepoURL(repoURL)
			if err := domaintypes.RepoURL(normalizedURL).Validate(); err != nil {
				failed++
				errs = append(errs, lineError{Line: lineNum, Message: fmt.Sprintf("invalid repo_url: %v", err)})
				continue
			}

			// Check if repo already exists to determine if this is create or update.
			_, err = st.GetMigRepoByURL(r.Context(), store.GetMigRepoByURLParams{
				MigID: migID,
				Url:   normalizedURL,
			})
			isUpdate := err == nil
			if err != nil && !errors.Is(err, pgx.ErrNoRows) {
				failed++
				errs = append(errs, lineError{Line: lineNum, Message: fmt.Sprintf("lookup failed: %v", err)})
				continue
			}

			// Upsert the repo.
			_, err = st.UpsertMigRepo(r.Context(), store.UpsertMigRepoParams{
				ID:        domaintypes.NewMigRepoID(), // Only used for insert
				MigID:     migID,
				Url:       normalizedURL,
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
			writeHTTPError(w, http.StatusBadRequest, "CSV file is empty or missing header")
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
			slog.Error("bulk upsert mig repos: encode response failed", "err", err)
		}

		slog.Info("bulk upsert mig repos completed",
			"mig_id", migID,
			"created", created,
			"updated", updated,
			"failed", failed,
		)
	}
}
