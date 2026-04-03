package handlers

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"sort"

	"github.com/iw2rmb/ploy/internal/store"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

// configHomeListItem represents an entry in the GET /v1/config/home list response.
type configHomeListItem struct {
	Entry   string `json:"entry"`
	Dst     string `json:"dst"`
	Section string `json:"section"`
}

// configHomePutRequest represents the request body for PUT /v1/config/home.
type configHomePutRequest struct {
	Entry   string `json:"entry"`
	Section string `json:"section"`
}

// listConfigHomeHandler returns all home entries grouped by section.
func listConfigHomeHandler(holder *ConfigHolder) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		all := holder.GetConfigHomeAll()

		sections := make([]string, 0, len(all))
		for s := range all {
			sections = append(sections, s)
		}
		sort.Strings(sections)

		var items []configHomeListItem
		for _, s := range sections {
			entries := all[s]
			for _, e := range entries {
				items = append(items, configHomeListItem{Entry: e.Entry, Dst: e.Dst, Section: s})
			}
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if err := json.NewEncoder(w).Encode(items); err != nil {
			slog.Error("config home list: encode response failed", "err", err)
		}
	}
}

// listConfigHomeBySectionHandler returns home entries for a specific section.
func listConfigHomeBySectionHandler(holder *ConfigHolder) http.HandlerFunc {
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

		entries := holder.GetConfigHome(section)
		items := make([]configHomeListItem, len(entries))
		for i, e := range entries {
			items[i] = configHomeListItem{Entry: e.Entry, Dst: e.Dst, Section: section}
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if err := json.NewEncoder(w).Encode(items); err != nil {
			slog.Error("config home list by section: encode response failed", "err", err)
		}
	}
}

// putConfigHomeHandler upserts a home entry. Entry is validated using the Hydra home parser.
// Destination is extracted and used as dedup key.
func putConfigHomeHandler(holder *ConfigHolder, st store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req configHomePutRequest
		if err := decodeRequestJSON(w, r, &req, DefaultMaxBodySize); err != nil {
			return
		}

		// Validate entry format using Hydra parser.
		parsed, err := contracts.ParseStoredHomeEntry(req.Entry)
		if err != nil {
			writeHTTPError(w, http.StatusBadRequest, "%s", err)
			return
		}

		if err := ValidateHydraSection(req.Section); err != nil {
			writeHTTPError(w, http.StatusBadRequest, "%s", err)
			return
		}

		// Persist to store.
		if err := st.UpsertConfigHome(r.Context(), store.UpsertConfigHomeParams{
			Entry:   req.Entry,
			Dst:     parsed.Dst,
			Section: req.Section,
		}); err != nil {
			slog.Error("config home put: store upsert failed", "err", err, "entry", req.Entry)
			writeHTTPError(w, http.StatusInternalServerError, "failed to persist config home: %v", err)
			return
		}

		// Update in-memory holder.
		holder.AddConfigHome(req.Section, ConfigHomeEntry{
			Entry:   req.Entry,
			Dst:     parsed.Dst,
			Section: req.Section,
		})

		resp := configHomeListItem{Entry: req.Entry, Dst: parsed.Dst, Section: req.Section}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			slog.Error("config home put: encode response failed", "err", err)
		}

		slog.Info("config home put: upserted entry", "dst", parsed.Dst, "section", req.Section)
	}
}

// deleteConfigHomeHandler removes a home entry by destination and section.
func deleteConfigHomeHandler(holder *ConfigHolder, st store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		dst := r.URL.Query().Get("dst")
		if dst == "" {
			writeHTTPError(w, http.StatusBadRequest, "dst query parameter is required")
			return
		}

		// Validate destination using Hydra parser rules (aligned with put handler).
		if err := contracts.ValidateHomeDestination(dst); err != nil {
			writeHTTPError(w, http.StatusBadRequest, "%s", err)
			return
		}

		section := r.URL.Query().Get("section")
		if section == "" {
			writeHTTPError(w, http.StatusBadRequest, "section query parameter is required")
			return
		}
		if err := ValidateHydraSection(section); err != nil {
			writeHTTPError(w, http.StatusBadRequest, "%s", err)
			return
		}

		if err := st.DeleteConfigHome(r.Context(), store.DeleteConfigHomeParams{
			Dst:     dst,
			Section: section,
		}); err != nil {
			slog.Error("config home delete: store delete failed", "err", err, "dst", dst)
			writeHTTPError(w, http.StatusInternalServerError, "failed to delete config home: %v", err)
			return
		}

		holder.DeleteConfigHome(section, dst)

		w.WriteHeader(http.StatusNoContent)
		slog.Info("config home delete: removed entry", "dst", dst, "section", section)
	}
}
