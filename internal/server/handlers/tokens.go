package handlers

import (
	"crypto/sha256"
	"encoding/hex"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/iw2rmb/ploy/internal/server/auth"
	"github.com/iw2rmb/ploy/internal/store"
	"github.com/jackc/pgx/v5/pgtype"
)

// createAPITokenHandler creates a new long-lived API token for CLI usage.
// Requires cli-admin role (enforced by middleware).
//
// POST /v1/tokens
// Request: { "role": "control-plane", "description": "...", "expires_in_days": 365 }
// Response: { "token": "eyJ...", "token_id": "...", "expires_at": "..." }
func createAPITokenHandler(st store.Store, tokenSecret string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Parse request with strict validation.
		var req struct {
			Role          string `json:"role"`
			Username      string `json:"username,omitempty"`
			Description   string `json:"description"`
			ExpiresInDays int    `json:"expires_in_days"`
		}

		if err := decodeRequestJSON(w, r, &req, DefaultMaxBodySize); err != nil {
			return
		}

		// Validate role.
		normalizedRole := auth.NormalizeRole(req.Role)
		if normalizedRole == "" {
			writeHTTPError(w, http.StatusBadRequest, "invalid role: must be one of cli-admin, control-plane, or worker")
			return
		}
		username := strings.TrimSpace(req.Username)
		if normalizedRole == auth.RoleControlPlane && username == "" {
			writeHTTPError(w, http.StatusBadRequest, "username is required for control-plane tokens")
			return
		}

		// Default expiration to 365 days if not specified.
		if req.ExpiresInDays <= 0 {
			req.ExpiresInDays = 365
		}

		// Generate token.
		now := time.Now()
		expiresAt := now.AddDate(0, 0, req.ExpiresInDays)
		token, err := auth.GenerateAPIToken(tokenSecret, string(normalizedRole), expiresAt)
		if err != nil {
			writeHTTPError(w, http.StatusInternalServerError, "failed to generate token: %v", err)
			slog.Error("create api token: generation failed", "err", err)
			return
		}

		// Parse token to extract token ID.
		claims, err := auth.ValidateToken(token, tokenSecret)
		if err != nil {
			writeHTTPError(w, http.StatusInternalServerError, "failed to validate generated token: %v", err)
			slog.Error("create api token: validation failed", "err", err)
			return
		}

		// Hash the token for storage.
		hash := sha256.Sum256([]byte(token))
		tokenHash := hex.EncodeToString(hash[:])

		// Get creator identity from context.
		var createdBy *string
		if identity, ok := auth.IdentityFromContext(r.Context()); ok {
			createdBy = &identity.CommonName
		}

		// Store token in database.
		description := &req.Description
		if req.Description == "" {
			description = nil
		}

		err = st.InsertAPIToken(r.Context(), store.InsertAPITokenParams{
			TokenHash:   tokenHash,
			TokenID:     claims.ID,
			Role:        string(normalizedRole),
			Username:    stringPtrOrNil(username),
			Description: description,
			IssuedAt:    pgtype.Timestamptz{Time: now, Valid: true},
			ExpiresAt:   pgtype.Timestamptz{Time: expiresAt, Valid: true},
			CreatedBy:   createdBy,
		})
		if err != nil {
			writeHTTPError(w, http.StatusInternalServerError, "failed to store token: %v", err)
			slog.Error("create api token: database insert failed", "err", err)
			return
		}

		// Return token (only shown once).
		resp := struct {
			Token     string    `json:"token"`
			TokenID   string    `json:"token_id"`
			Role      string    `json:"role"`
			Username  *string   `json:"username,omitempty"`
			ExpiresAt time.Time `json:"expires_at"`
			Warning   string    `json:"warning"`
		}{
			Token:     token,
			TokenID:   claims.ID,
			Role:      string(normalizedRole),
			Username:  stringPtrOrNil(username),
			ExpiresAt: expiresAt,
			Warning:   "Save this token securely. It will not be shown again.",
		}

		writeJSON(w, http.StatusOK, resp)

		slog.Info("api token created",
			"token_id", claims.ID,
			"role", normalizedRole,
			"expires_at", expiresAt,
			"created_by", createdBy,
		)
	}
}

func stringPtrOrNil(value string) *string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return &value
}

// listAPITokensHandler lists all API tokens.
// Requires cli-admin role (enforced by middleware).
//
// GET /v1/tokens
// Response: { "tokens": [{ "token_id": "...", "role": "...", ... }] }
func listAPITokensHandler(st store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Query tokens from database.
		tokens, err := st.ListAPITokens(r.Context())
		if err != nil {
			writeHTTPError(w, http.StatusInternalServerError, "failed to list tokens: %v", err)
			slog.Error("list api tokens: database query failed", "err", err)
			return
		}

		// Convert to response format.
		type tokenResponse struct {
			TokenID     string     `json:"token_id"`
			Role        string     `json:"role"`
			Username    *string    `json:"username,omitempty"`
			Description *string    `json:"description,omitempty"`
			IssuedAt    time.Time  `json:"issued_at"`
			ExpiresAt   time.Time  `json:"expires_at"`
			LastUsedAt  *time.Time `json:"last_used_at,omitempty"`
			RevokedAt   *time.Time `json:"revoked_at,omitempty"`
			CreatedBy   *string    `json:"created_by,omitempty"`
		}

		responseTokens := make([]tokenResponse, 0, len(tokens))
		for _, t := range tokens {
			var lastUsedAt *time.Time
			if t.LastUsedAt.Valid {
				lastUsedAt = &t.LastUsedAt.Time
			}
			var revokedAt *time.Time
			if t.RevokedAt.Valid {
				revokedAt = &t.RevokedAt.Time
			}

			responseTokens = append(responseTokens, tokenResponse{
				TokenID:     t.TokenID,
				Role:        t.Role,
				Username:    t.Username,
				Description: t.Description,
				IssuedAt:    t.IssuedAt.Time,
				ExpiresAt:   t.ExpiresAt.Time,
				LastUsedAt:  lastUsedAt,
				RevokedAt:   revokedAt,
				CreatedBy:   t.CreatedBy,
			})
		}

		resp := struct {
			Tokens []tokenResponse `json:"tokens"`
		}{
			Tokens: responseTokens,
		}

		writeJSON(w, http.StatusOK, resp)

		slog.Info("api tokens listed", "count", len(tokens))
	}
}

// revokeAPITokenHandler revokes an API token.
// Requires cli-admin role (enforced by middleware).
//
// DELETE /v1/tokens/{id}
// Response: { "message": "Token revoked successfully" }
func revokeAPITokenHandler(st store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tokenID, ok := requiredPathParamOrWriteError(w, r, "id")
		if !ok {
			return
		}

		if err := st.RevokeAPIToken(r.Context(), tokenID); err != nil {
			writeHTTPError(w, http.StatusInternalServerError, "failed to revoke token: %v", err)
			slog.Error("revoke api token: database update failed", "token_id", tokenID, "err", err)
			return
		}

		resp := struct {
			Message string `json:"message"`
		}{
			Message: "Token revoked successfully",
		}

		writeJSON(w, http.StatusOK, resp)

		slog.Info("api token revoked", "token_id", tokenID)
	}
}
