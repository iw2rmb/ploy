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

// setMigSpecHandler creates a new spec row and updates migs.spec_id to point at it.
// Endpoint: POST /v1/migs/{mig_ref}/specs
// Request: {name?, spec}
// Response: 201 Created with spec details (id, created_at)
//
// v1 contract:
// - Specs are append-only: each call inserts a new specs row.
// - migs.spec_id is updated to point at the newly created spec.
// - This is the canonical way to "set" or "update" a mig's spec.
func setMigSpecHandler(st store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		modRef, err := parseParam[domaintypes.MigRef](r, "mig_ref")
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

		// Validate spec structure (same validation as in createMigHandler).
		if _, err := contracts.ParseModsSpecJSON(req.Spec); err != nil {
			httpErr(w, http.StatusBadRequest, "spec: %v", err)
			return
		}

		// Resolve mig by ID-or-name.
		mig, err := resolveMigByRef(r.Context(), st, modRef)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				httpErr(w, http.StatusNotFound, "mig not found")
				return
			}
			httpErr(w, http.StatusInternalServerError, "failed to get mig: %v", err)
			slog.Error("set mig spec: get mig failed", "mig_ref", modRef, "err", err)
			return
		}
		modID := mig.ID

		// Check if mig is archived — cannot update spec on archived migs.
		if mig.ArchivedAt.Valid {
			httpErr(w, http.StatusConflict, "cannot set spec on archived mig")
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
			slog.Error("set mig spec: create spec failed", "mig_id", modID, "err", err)
			return
		}

		// Update migs.spec_id to point at the new spec.
		if err := st.UpdateMigSpec(r.Context(), store.UpdateMigSpecParams{ID: modID, SpecID: &createdSpec.ID}); err != nil {
			httpErr(w, http.StatusInternalServerError, "failed to update mig spec: %v", err)
			slog.Error("set mig spec: update mig failed", "mig_id", modID, "spec_id", createdSpec.ID, "err", err)
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
			slog.Error("set mig spec: encode response failed", "err", err)
		}

		slog.Info("mig spec set", "mig_id", modID, "spec_id", createdSpec.ID.String())
	}
}
