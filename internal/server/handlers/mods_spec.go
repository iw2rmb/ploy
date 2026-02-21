package handlers

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/jackc/pgx/v5"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

// setModSpecHandler creates a new spec row and updates mods.spec_id to point at it.
// Endpoint: POST /v1/mods/{mod_ref}/specs
// Request: {name?, spec}
// Response: 201 Created with spec details (id, created_at)
//
// v1 contract:
// - Specs are append-only: each call inserts a new specs row.
// - mods.spec_id is updated to point at the newly created spec.
// - This is the canonical way to "set" or "update" a mod's spec.
func setModSpecHandler(st store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		modRef, err := ParseModRefParam(r, "mod_ref")
		if err != nil {
			httpErr(w, http.StatusBadRequest, "%s", err)
			return
		}

		// Parse request body with strict validation.
		var req struct {
			Name      string          `json:"name,omitempty"`
			Spec      json.RawMessage `json:"spec"`
			CreatedBy *string         `json:"created_by,omitempty"`
		}
		if err := DecodeJSON(w, r, &req, maxModSpecSize); err != nil {
			return
		}

		// Validate spec is present and non-empty.
		if len(req.Spec) == 0 {
			httpErr(w, http.StatusBadRequest, "spec is required")
			return
		}

		// Validate spec structure (same validation as in createModHandler).
		if _, err := contracts.ParseModsSpecJSON(req.Spec); err != nil {
			httpErr(w, http.StatusBadRequest, "spec: %v", err)
			return
		}

		// Resolve mod by ID-or-name.
		mod, err := resolveModByRef(r.Context(), st, modRef)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				httpErr(w, http.StatusNotFound, "mod not found")
				return
			}
			httpErr(w, http.StatusInternalServerError, "failed to get mod: %v", err)
			slog.Error("set mod spec: get mod failed", "mod_ref", modRef, "err", err)
			return
		}
		modID := mod.ID

		// Check if mod is archived — cannot update spec on archived mods.
		if mod.ArchivedAt.Valid {
			httpErr(w, http.StatusConflict, "cannot set spec on archived mod")
			return
		}

		// Create new spec row (append-only).
		specID := domaintypes.NewSpecID()
		createdSpec, err := st.CreateSpec(r.Context(), store.CreateSpecParams{
			ID:        specID,
			Name:      req.Name,
			Spec:      req.Spec,
			CreatedBy: req.CreatedBy,
		})
		if err != nil {
			httpErr(w, http.StatusInternalServerError, "failed to create spec: %v", err)
			slog.Error("set mod spec: create spec failed", "mod_id", modID, "err", err)
			return
		}

		// Update mods.spec_id to point at the new spec.
		if err := st.UpdateModSpec(r.Context(), store.UpdateModSpecParams{ID: modID, SpecID: &createdSpec.ID}); err != nil {
			httpErr(w, http.StatusInternalServerError, "failed to update mod spec: %v", err)
			slog.Error("set mod spec: update mod failed", "mod_id", modID, "spec_id", createdSpec.ID, "err", err)
			return
		}

		// Build response with spec ID and creation timestamp.
		resp := struct {
			ID        string `json:"id"`
			CreatedAt string `json:"created_at"`
		}{
			ID:        createdSpec.ID.String(),
			CreatedAt: createdSpec.CreatedAt.Time.Format("2006-01-02T15:04:05Z07:00"),
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			slog.Error("set mod spec: encode response failed", "err", err)
		}

		slog.Info("mod spec set", "mod_id", modID, "spec_id", createdSpec.ID.String())
	}
}
