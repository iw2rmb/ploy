package handlers

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"sort"
	"strings"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

// configCAListItem represents an entry in the GET /v1/config/ca list response.
type configCAListItem struct {
	Hash    string `json:"hash"`
	Section string `json:"section"`
}

// configCAPutRequest represents the request body for PUT /v1/config/ca/{hash}.
type configCAPutRequest struct {
	Section  string `json:"section"`
	BundleID string `json:"bundle_id,omitempty"`
}

// listConfigCAHandler returns all CA entries grouped by section.
func listConfigCAHandler(holder *ConfigHolder) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		all := holder.GetConfigCAAll()

		sections := make([]string, 0, len(all))
		for s := range all {
			sections = append(sections, s)
		}
		sort.Strings(sections)

		var items []configCAListItem
		for _, s := range sections {
			hashes := all[s]
			for _, h := range hashes {
				items = append(items, configCAListItem{Hash: h, Section: s})
			}
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if err := json.NewEncoder(w).Encode(items); err != nil {
			slog.Error("config ca list: encode response failed", "err", err)
		}
	}
}

// listConfigCABySectionHandler returns CA entries for a specific section.
func listConfigCABySectionHandler(holder *ConfigHolder) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		section, err := requiredPathParam(r, "section")
		if err != nil {
			writeHTTPError(w, http.StatusBadRequest, "%s", err)
			return
		}
		if err := ValidateHydraSection(section); err != nil {
			writeHTTPError(w, http.StatusBadRequest, "%s", err)
			return
		}

		hashes := holder.GetConfigCA(section)
		items := make([]configCAListItem, len(hashes))
		for i, h := range hashes {
			items[i] = configCAListItem{Hash: h, Section: section}
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if err := json.NewEncoder(w).Encode(items); err != nil {
			slog.Error("config ca list by section: encode response failed", "err", err)
		}
	}
}

// putConfigCAHandler upserts a CA entry.
func putConfigCAHandler(holder *ConfigHolder, st store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		hash, err := requiredPathParam(r, "hash")
		if err != nil {
			writeHTTPError(w, http.StatusBadRequest, "%s", err)
			return
		}

		// Validate and normalize hash using Hydra parser.
		normalized, err := contracts.ParseStoredCAEntry(hash)
		if err != nil {
			writeHTTPError(w, http.StatusBadRequest, "%s", err)
			return
		}
		hash = normalized

		var req configCAPutRequest
		if err := decodeRequestJSON(w, r, &req, DefaultMaxBodySize); err != nil {
			return
		}

		if err := ValidateHydraSection(req.Section); err != nil {
			writeHTTPError(w, http.StatusBadRequest, "%s", err)
			return
		}

		bundleID := strings.TrimSpace(req.BundleID)
		if bundleID != "" {
			var id domaintypes.SpecBundleID
			if err := id.UnmarshalText([]byte(bundleID)); err != nil {
				writeHTTPError(w, http.StatusBadRequest, "invalid bundle_id: %v", err)
				return
			}
			bundleID = id.String()
			if err := st.UpsertConfigBundleMap(r.Context(), store.UpsertConfigBundleMapParams{
				Hash:     hash,
				BundleID: bundleID,
			}); err != nil {
				slog.Error("config ca put: bundle mapping upsert failed", "err", err, "hash", hash, "bundle_id", bundleID)
				writeHTTPError(w, http.StatusInternalServerError, "failed to persist bundle mapping: %v", err)
				return
			}
			holder.AddBundleMapping(hash, bundleID)
		} else {
			bundleMap := holder.GetBundleMap()
			if _, ok := bundleMap[hash]; !ok {
				writeHTTPError(w, http.StatusBadRequest, "bundle mapping missing for hash %s (provide bundle_id or upload via --file)", hash)
				return
			}
		}

		// Persist to store.
		if err := st.UpsertConfigCA(r.Context(), store.UpsertConfigCAParams{
			Hash:    hash,
			Section: req.Section,
		}); err != nil {
			slog.Error("config ca put: store upsert failed", "err", err, "hash", hash)
			writeHTTPError(w, http.StatusInternalServerError, "failed to persist config ca: %v", err)
			return
		}

		// Update in-memory holder.
		holder.AddConfigCA(req.Section, hash)

		resp := configCAListItem{Hash: hash, Section: req.Section}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			slog.Error("config ca put: encode response failed", "err", err)
		}

		slog.Info("config ca put: upserted entry", "hash", hash, "section", req.Section)
	}
}

// deleteConfigCAHandler removes a CA entry.
func deleteConfigCAHandler(holder *ConfigHolder, st store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		hash, err := requiredPathParam(r, "hash")
		if err != nil {
			writeHTTPError(w, http.StatusBadRequest, "%s", err)
			return
		}

		// Validate and normalize hash using Hydra parser (aligned with put handler).
		normalized, err := contracts.ParseStoredCAEntry(hash)
		if err != nil {
			writeHTTPError(w, http.StatusBadRequest, "%s", err)
			return
		}
		hash = normalized

		section := r.URL.Query().Get("section")
		if section == "" {
			writeHTTPError(w, http.StatusBadRequest, "section query parameter is required")
			return
		}
		if err := ValidateHydraSection(section); err != nil {
			writeHTTPError(w, http.StatusBadRequest, "%s", err)
			return
		}

		if err := st.DeleteConfigCA(r.Context(), store.DeleteConfigCAParams{
			Hash:    hash,
			Section: section,
		}); err != nil {
			slog.Error("config ca delete: store delete failed", "err", err, "hash", hash)
			writeHTTPError(w, http.StatusInternalServerError, "failed to delete config ca: %v", err)
			return
		}

		holder.DeleteConfigCA(section, hash)

		w.WriteHeader(http.StatusNoContent)
		slog.Info("config ca delete: removed entry", "hash", hash, "section", section)
	}
}
