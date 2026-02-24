package handlers

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"sort"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
)

// globalEnvListItem represents an entry in the GET /v1/config/env list response.
// For secrets, the value is redacted to prevent accidental exposure.
// Scope is serialized as string for wire compatibility.
type globalEnvListItem struct {
	Key    string `json:"key"`
	Value  string `json:"value,omitempty"` // Omitted (empty) for secrets in list view.
	Scope  string `json:"scope"`
	Secret bool   `json:"secret"`
}

// globalEnvResponse represents the response for GET /v1/config/env/{key} and PUT /v1/config/env/{key}.
// Full value is returned since these endpoints are admin-only and accessed via mTLS.
// Scope is serialized as string for wire compatibility.
type globalEnvResponse struct {
	Key    string `json:"key"`
	Value  string `json:"value"`
	Scope  string `json:"scope"`
	Secret bool   `json:"secret"`
}

// globalEnvPutRequest represents the request body for PUT /v1/config/env/{key}.
// Scope is parsed and validated at the API boundary using domaintypes.ParseGlobalEnvScope().
type globalEnvPutRequest struct {
	Value  string `json:"value"`
	Scope  string `json:"scope"`  // Raw string from wire; validated via ParseGlobalEnvScope.
	Secret *bool  `json:"secret"` // Pointer to distinguish explicit false from missing (defaults to true).
}

// listGlobalEnvHandler returns an HTTP handler that lists all global env entries.
// For secret entries, the value is redacted (empty string) in the response.
// Requires cli-admin role (enforced by middleware).
func listGlobalEnvHandler(holder *ConfigHolder) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		envMap := holder.GetGlobalEnv()

		// Build sorted list for consistent output.
		keys := make([]string, 0, len(envMap))
		for k := range envMap {
			keys = append(keys, k)
		}
		sort.Strings(keys)

		items := make([]globalEnvListItem, 0, len(keys))
		for _, k := range keys {
			v := envMap[k]
			item := globalEnvListItem{
				Key:    k,
				Scope:  v.Scope.String(), // Convert typed scope to string for wire format.
				Secret: v.Secret,
			}
			// Redact value for secrets in list view.
			if !v.Secret {
				item.Value = v.Value
			}
			items = append(items, item)
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if err := json.NewEncoder(w).Encode(items); err != nil {
			slog.Error("config env list: encode response failed", "err", err)
		}

		slog.Info("config env list: returned entries", "count", len(items))
	}
}

// getGlobalEnvHandler returns an HTTP handler that returns a single global env entry by key.
// Full value is returned regardless of secret flag (admin-only, mTLS protected).
// Requires cli-admin role (enforced by middleware).
func getGlobalEnvHandler(holder *ConfigHolder) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		key, err := requiredPathParam(r, "key")
		if err != nil {
			httpErr(w, http.StatusBadRequest, "%s", err)
			return
		}

		v, ok := holder.GetGlobalEnvVar(key)
		if !ok {
			httpErr(w, http.StatusNotFound, "global env key not found: %s", key)
			return
		}

		resp := globalEnvResponse{
			Key:    key,
			Value:  v.Value,
			Scope:  v.Scope.String(), // Convert typed scope to string for wire format.
			Secret: v.Secret,
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			slog.Error("config env get: encode response failed", "err", err, "key", key)
		}

		slog.Info("config env get: returned entry",
			"key", key,
			"scope", v.Scope.String(),
			"secret", v.Secret,
		)
	}
}

// putGlobalEnvHandler returns an HTTP handler that upserts a global env entry.
// Updates both the in-memory ConfigHolder and persists to the store.
// Requires cli-admin role (enforced by middleware).
func putGlobalEnvHandler(holder *ConfigHolder, st store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		key, err := requiredPathParam(r, "key")
		if err != nil {
			httpErr(w, http.StatusBadRequest, "%s", err)
			return
		}

		var req globalEnvPutRequest
		if err := DecodeJSON(w, r, &req, DefaultMaxBodySize); err != nil {
			return
		}

		// Parse and validate scope at API boundary using typed enum.
		// ParseGlobalEnvScope handles empty string (defaults to "all") and validates
		// against known values (all, migs, heal, gate).
		scope, err := domaintypes.ParseGlobalEnvScope(req.Scope)
		if err != nil {
			httpErr(w, http.StatusBadRequest, "%s", err)
			return
		}

		// Default secret to true if not explicitly set.
		secret := true
		if req.Secret != nil {
			secret = *req.Secret
		}

		// Persist to the store first (fail-fast if database is down).
		// Store uses string scope for wire compatibility.
		if err := st.UpsertGlobalEnv(r.Context(), store.UpsertGlobalEnvParams{
			Key:    key,
			Value:  req.Value,
			Scope:  scope.String(), // Convert typed scope to string for database storage.
			Secret: secret,
		}); err != nil {
			slog.Error("config env put: store upsert failed", "err", err, "key", key)
			httpErr(w, http.StatusInternalServerError, "failed to persist global env: %v", err)
			return
		}

		// Update in-memory holder after successful persistence.
		// In-memory representation uses typed scope.
		holder.SetGlobalEnvVar(key, GlobalEnvVar{
			Value:  req.Value,
			Scope:  scope, // Use validated typed scope.
			Secret: secret,
		})

		resp := globalEnvResponse{
			Key:    key,
			Value:  req.Value,
			Scope:  scope.String(), // Convert typed scope to string for wire format.
			Secret: secret,
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			slog.Error("config env put: encode response failed", "err", err, "key", key)
		}

		slog.Info("config env put: upserted entry",
			"key", key,
			"scope", scope.String(),
			"secret", secret,
		)
	}
}

// deleteGlobalEnvHandler returns an HTTP handler that deletes a global env entry.
// Removes from both the in-memory ConfigHolder and the store.
// Requires cli-admin role (enforced by middleware).
func deleteGlobalEnvHandler(holder *ConfigHolder, st store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		key, err := requiredPathParam(r, "key")
		if err != nil {
			httpErr(w, http.StatusBadRequest, "%s", err)
			return
		}

		// Delete from store first (idempotent operation).
		if err := st.DeleteGlobalEnv(r.Context(), key); err != nil {
			slog.Error("config env delete: store delete failed", "err", err, "key", key)
			httpErr(w, http.StatusInternalServerError, "failed to delete global env: %v", err)
			return
		}

		// Remove from in-memory holder after successful deletion.
		holder.DeleteGlobalEnvVar(key)

		w.WriteHeader(http.StatusNoContent)

		slog.Info("config env delete: removed entry", "key", key)
	}
}
