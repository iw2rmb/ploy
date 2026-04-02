package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sort"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
)

var (
	errTargetNotFound = errors.New("target not found")
	errAmbiguousKey   = errors.New("ambiguous key")
)

// globalEnvListItem represents an entry in the GET /v1/config/env list response.
// For secrets, the value is redacted to prevent accidental exposure.
type globalEnvListItem struct {
	Key    string `json:"key"`
	Value  string `json:"value,omitempty"` // Omitted (empty) for secrets in list view.
	Target string `json:"target"`
	Secret bool   `json:"secret"`
}

// globalEnvResponse represents the response for GET /v1/config/env/{key} and PUT /v1/config/env/{key}.
// Full value is returned since these endpoints are admin-only and accessed via mTLS.
type globalEnvResponse struct {
	Key    string `json:"key"`
	Value  string `json:"value"`
	Target string `json:"target"`
	Secret bool   `json:"secret"`
}

// globalEnvPutRequest represents the request body for PUT /v1/config/env/{key}.
// Target is parsed and validated at the API boundary using domaintypes.ParseGlobalEnvTarget().
type globalEnvPutRequest struct {
	Value  string `json:"value"`
	Target string `json:"target"` // Raw string from wire; parsed via ParseGlobalEnvTarget.
	Secret *bool  `json:"secret"` // Pointer to distinguish explicit false from missing (defaults to true).
}

// listGlobalEnvHandler returns an HTTP handler that lists all global env entries.
// Returns all key+target pairs as a flat list sorted by key then target.
// For secret entries, the value is redacted (empty string) in the response.
// Requires cli-admin role (enforced by middleware).
func listGlobalEnvHandler(holder *ConfigHolder) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		envMap := holder.GetGlobalEnvAll()

		// Build sorted list for consistent output.
		keys := make([]string, 0, len(envMap))
		for k := range envMap {
			keys = append(keys, k)
		}
		sort.Strings(keys)

		var items []globalEnvListItem
		for _, k := range keys {
			entries := envMap[k]
			// Sort entries within key by target for deterministic order.
			sort.Slice(entries, func(i, j int) bool {
				return entries[i].Target.String() < entries[j].Target.String()
			})
			for _, v := range entries {
				item := globalEnvListItem{
					Key:    k,
					Target: v.Target.String(),
					Secret: v.Secret,
				}
				if !v.Secret {
					item.Value = v.Value
				}
				items = append(items, item)
			}
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
// When multiple targets exist for a key, the ?target= query parameter is required;
// otherwise the endpoint returns a 409 ambiguity error.
// Requires cli-admin role (enforced by middleware).
func getGlobalEnvHandler(holder *ConfigHolder) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		key, err := requiredPathParam(r, "key")
		if err != nil {
			writeHTTPError(w, http.StatusBadRequest, "%s", err)
			return
		}

		entries := holder.GetGlobalEnvEntries(key)
		if len(entries) == 0 {
			writeHTTPError(w, http.StatusNotFound, "global env key not found: %s", key)
			return
		}

		targetStr := r.URL.Query().Get("target")
		v, err := resolveAmbiguousEntry(entries, targetStr)
		if err != nil {
			switch {
			case errors.Is(err, errAmbiguousKey):
				writeHTTPError(w, http.StatusConflict, "%s", err)
			case errors.Is(err, errTargetNotFound):
				writeHTTPError(w, http.StatusNotFound, "%s", err)
			default:
				writeHTTPError(w, http.StatusBadRequest, "invalid target: %s", err)
			}
			return
		}

		resp := globalEnvResponse{
			Key:    key,
			Value:  v.Value,
			Target: v.Target.String(),
			Secret: v.Secret,
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			slog.Error("config env get: encode response failed", "err", err, "key", key)
		}

		slog.Info("config env get: returned entry",
			"key", key,
			"target", v.Target.String(),
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
			writeHTTPError(w, http.StatusBadRequest, "%s", err)
			return
		}

		var req globalEnvPutRequest
		if err := decodeRequestJSON(w, r, &req, DefaultMaxBodySize); err != nil {
			return
		}

		// Parse and validate target at API boundary using typed enum.
		target, err := domaintypes.ParseGlobalEnvTarget(req.Target)
		if err != nil {
			writeHTTPError(w, http.StatusBadRequest, "%s", err)
			return
		}

		// Default secret to true if not explicitly set.
		secret := true
		if req.Secret != nil {
			secret = *req.Secret
		}

		// Persist to the store first (fail-fast if database is down).
		if err := st.UpsertGlobalEnv(r.Context(), store.UpsertGlobalEnvParams{
			Key:    key,
			Target: target.String(),
			Value:  req.Value,
			Secret: secret,
		}); err != nil {
			slog.Error("config env put: store upsert failed", "err", err, "key", key)
			writeHTTPError(w, http.StatusInternalServerError, "failed to persist global env: %v", err)
			return
		}

		// Update in-memory holder after successful persistence.
		holder.SetGlobalEnvVar(key, GlobalEnvVar{
			Value:  req.Value,
			Target: target,
			Secret: secret,
		})

		resp := globalEnvResponse{
			Key:    key,
			Value:  req.Value,
			Target: target.String(),
			Secret: secret,
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			slog.Error("config env put: encode response failed", "err", err, "key", key)
		}

		slog.Info("config env put: upserted entry",
			"key", key,
			"target", target.String(),
			"secret", secret,
		)
	}
}

// deleteGlobalEnvHandler returns an HTTP handler that deletes a global env entry.
// Removes from both the in-memory ConfigHolder and the store.
// When multiple targets exist for a key, the ?target= query parameter is required;
// otherwise the endpoint returns a 409 ambiguity error.
// If only one target exists for the key, target is inferred automatically.
// Requires cli-admin role (enforced by middleware).
func deleteGlobalEnvHandler(holder *ConfigHolder, st store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		key, err := requiredPathParam(r, "key")
		if err != nil {
			writeHTTPError(w, http.StatusBadRequest, "%s", err)
			return
		}

		// Resolve target: explicit query param or inferred from single entry.
		targetStr := r.URL.Query().Get("target")
		var target domaintypes.GlobalEnvTarget

		entries := holder.GetGlobalEnvEntries(key)
		if targetStr != "" {
			target, err = domaintypes.ParseGlobalEnvTarget(targetStr)
			if err != nil {
				writeHTTPError(w, http.StatusBadRequest, "%s", err)
				return
			}
		} else {
			switch len(entries) {
			case 0:
				// Idempotent: key does not exist, nothing to delete.
				w.WriteHeader(http.StatusNoContent)
				return
			case 1:
				target = entries[0].Target
			default:
				writeHTTPError(w, http.StatusConflict,
					"ambiguous key %q: exists for targets %s; specify ?target= to disambiguate",
					key, targetList(entries))
				return
			}
		}

		// Delete from store first (idempotent operation).
		if err := st.DeleteGlobalEnv(r.Context(), store.DeleteGlobalEnvParams{Key: key, Target: target.String()}); err != nil {
			slog.Error("config env delete: store delete failed", "err", err, "key", key)
			writeHTTPError(w, http.StatusInternalServerError, "failed to delete global env: %v", err)
			return
		}

		// Remove from in-memory holder after successful deletion.
		holder.DeleteGlobalEnvVar(key, target)

		w.WriteHeader(http.StatusNoContent)

		slog.Info("config env delete: removed entry", "key", key, "target", target.String())
	}
}

// resolveAmbiguousEntry selects a single entry from a non-empty slice.
// If targetStr is provided, the matching entry is returned.
// If targetStr is empty and exactly one entry exists, that entry is returned.
// If targetStr is empty and multiple entries exist, an ambiguity error is returned.
//
// Errors returned wrap one of:
//   - parse error from ParseGlobalEnvTarget (invalid target value)
//   - errTargetNotFound (valid target but not present for this key)
//   - errAmbiguousKey (multiple targets, no selector provided)
func resolveAmbiguousEntry(entries []GlobalEnvVar, targetStr string) (GlobalEnvVar, error) {
	if targetStr != "" {
		target, err := domaintypes.ParseGlobalEnvTarget(targetStr)
		if err != nil {
			return GlobalEnvVar{}, err
		}
		for _, e := range entries {
			if e.Target == target {
				return e, nil
			}
		}
		return GlobalEnvVar{}, fmt.Errorf("%w: %q", errTargetNotFound, targetStr)
	}

	if len(entries) == 1 {
		return entries[0], nil
	}

	return GlobalEnvVar{}, fmt.Errorf(
		"%w: exists for targets %s; specify ?target= to disambiguate",
		errAmbiguousKey, targetList(entries))
}

// targetList formats entry targets as a comma-separated string for error messages.
func targetList(entries []GlobalEnvVar) string {
	targets := make([]string, len(entries))
	for i, e := range entries {
		targets[i] = e.Target.String()
	}
	sort.Strings(targets)
	result := ""
	for i, t := range targets {
		if i > 0 {
			result += ", "
		}
		result += t
	}
	return result
}

// GetGlobalEnvAll returns a copy of all global environment entries grouped by key.
// Each key maps to a slice of entries (one per target).
func (h *ConfigHolder) GetGlobalEnvAll() map[string][]GlobalEnvVar {
	h.mu.RLock()
	defer h.mu.RUnlock()
	envCopy := make(map[string][]GlobalEnvVar, len(h.globalEnv))
	for k, entries := range h.globalEnv {
		cp := make([]GlobalEnvVar, len(entries))
		copy(cp, entries)
		envCopy[k] = cp
	}
	return envCopy
}
