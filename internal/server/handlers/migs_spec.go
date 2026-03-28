package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
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
		// Parse request body with strict validation.
		var req struct {
			Name      string          `json:"name,omitempty"`
			Spec      json.RawMessage `json:"spec"`
			CreatedBy *string         `json:"created_by,omitempty"`
		}
		if err := decodeRequestJSON(w, r, &req, maxModSpecSize); err != nil {
			return
		}

		// Validate spec is present and non-empty.
		if len(req.Spec) == 0 {
			writeHTTPError(w, http.StatusBadRequest, "spec is required")
			return
		}

		// Validate spec structure (same validation as in createMigHandler).
		if _, err := contracts.ParseMigSpecJSON(req.Spec); err != nil {
			writeHTTPError(w, http.StatusBadRequest, "spec: %v", err)
			return
		}

		// Resolve mig by ID-or-name.
		mig, ok := getMigByRefOrFail(w, r, st, "set mig spec")
		if !ok {
			return
		}
		modID := mig.ID

		// Check if mig is archived — cannot update spec on archived migs.
		if mig.ArchivedAt.Valid {
			writeHTTPError(w, http.StatusConflict, "cannot set spec on archived mig")
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
			writeHTTPError(w, http.StatusInternalServerError, "failed to create spec: %v", err)
			slog.Error("set mig spec: create spec failed", "mig_id", modID, "err", err)
			return
		}

		// Update migs.spec_id to point at the new spec.
		if err := st.UpdateMigSpec(r.Context(), store.UpdateMigSpecParams{ID: modID, SpecID: &createdSpec.ID}); err != nil {
			writeHTTPError(w, http.StatusInternalServerError, "failed to update mig spec: %v", err)
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

// getMigLatestSpecHandler returns the latest spec payload for a mig.
// Endpoint: GET /v1/migs/{mig_ref}/specs/latest
// Response: raw spec JSON body.
func getMigLatestSpecHandler(st store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		mig, ok := getMigByRefOrFail(w, r, st, "get mig latest spec")
		if !ok {
			return
		}
		if mig.SpecID == nil || mig.SpecID.IsZero() {
			writeHTTPError(w, http.StatusNotFound, "mig has no spec")
			return
		}

		spec, err := st.GetSpec(r.Context(), *mig.SpecID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				writeHTTPError(w, http.StatusNotFound, "spec not found")
				return
			}
			writeHTTPError(w, http.StatusInternalServerError, "failed to get spec: %v", err)
			slog.Error("get mig latest spec: get spec failed", "mig_id", mig.ID, "spec_id", mig.SpecID.String(), "err", err)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", "spec-"+spec.ID.String()+".json"))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(spec.Spec)
	}
}
