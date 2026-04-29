package handlers

import (
	"log/slog"
	"net/http"
	"path"

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

		var items []configHomeListItem
		for _, s := range sortedSectionNames(all) {
			entries := all[s]
			for _, e := range entries {
				items = append(items, configHomeListItem{Entry: e.Entry, Dst: e.Dst, Section: s})
			}
		}

		writeJSON(w, http.StatusOK, items)
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
		if err := contracts.ValidateHydraSection(section); err != nil {
			writeHTTPError(w, http.StatusBadRequest, "%s", err)
			return
		}

		entries := holder.GetConfigHome(section)
		items := make([]configHomeListItem, len(entries))
		for i, e := range entries {
			items[i] = configHomeListItem{Entry: e.Entry, Dst: e.Dst, Section: section}
		}

		writeJSON(w, http.StatusOK, items)
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

		// Validate entry format using Hydra parser and canonicalize.
		parsed, err := contracts.ParseStoredHomeEntry(req.Entry)
		if err != nil {
			writeHTTPError(w, http.StatusBadRequest, "%s", err)
			return
		}
		canonicalEntry := parsed.CanonicalHomeEntry()

		if err := contracts.ValidateHydraSection(req.Section); err != nil {
			writeHTTPError(w, http.StatusBadRequest, "%s", err)
			return
		}

		// Persist to store using canonicalized entry.
		if err := st.UpsertConfigHome(r.Context(), store.UpsertConfigHomeParams{
			Entry:   canonicalEntry,
			Dst:     parsed.Dst,
			Section: req.Section,
		}); err != nil {
			slog.Error("config home put: store upsert failed", "err", err, "entry", canonicalEntry)
			writeHTTPError(w, http.StatusInternalServerError, "failed to persist config home: %v", err)
			return
		}

		// Update in-memory holder.
		holder.AddConfigHome(req.Section, ConfigHomeEntry{
			Entry:   canonicalEntry,
			Dst:     parsed.Dst,
			Section: req.Section,
		})

		resp := configHomeListItem{Entry: canonicalEntry, Dst: parsed.Dst, Section: req.Section}
		writeJSON(w, http.StatusOK, resp)

		slog.Info("config home put: upserted entry", "dst", parsed.Dst, "section", req.Section)
	}
}

// deleteConfigHomeHandler removes a home entry by destination and section.
func deleteConfigHomeHandler(holder *ConfigHolder, st store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		dst, err := requiredQueryParam(r, "dst")
		if err != nil {
			writeHTTPError(w, http.StatusBadRequest, "%s", err)
			return
		}

		// Validate and normalize destination using Hydra parser rules (aligned
		// with put handler's ParseStoredHomeEntry which cleans dst before persisting).
		if err := contracts.ValidateHomeDestination(dst); err != nil {
			writeHTTPError(w, http.StatusBadRequest, "%s", err)
			return
		}
		dst = path.Clean(dst)

		section, err := requiredQueryParam(r, "section")
		if err != nil {
			writeHTTPError(w, http.StatusBadRequest, "%s", err)
			return
		}
		if err := contracts.ValidateHydraSection(section); err != nil {
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
