package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
)

// createDiffHandler stores gzipped diff in diffs table (≤1 MiB), rejects oversize.
func createDiffHandler(st store.Store) http.HandlerFunc {
	// Accept up to 2 MiB for the JSON body to accommodate base64 overhead
	// while still enforcing a strict 1 MiB cap on the decoded patch bytes.
	const maxBodySize = 2 << 20  // 2 MiB
	const maxPatchSize = 1 << 20 // 1 MiB
	return func(w http.ResponseWriter, r *http.Request) {
		// Extract node id from path parameter.
		nodeIDStr := r.PathValue("id")
		if strings.TrimSpace(nodeIDStr) == "" {
			http.Error(w, "id path parameter is required", http.StatusBadRequest)
			return
		}

		// Parse and validate node_id.
		nodeID := domaintypes.ToPGUUID(nodeIDStr)
		if !nodeID.Valid {
			http.Error(w, "invalid id: invalid uuid", http.StatusBadRequest)
			return
		}

		// Extract stage id from path parameter.
		stageIDStr := r.PathValue("stage")
		if strings.TrimSpace(stageIDStr) == "" {
			http.Error(w, "stage path parameter is required", http.StatusBadRequest)
			return
		}

		// Parse and validate stage_id.
		stageID := domaintypes.ToPGUUID(stageIDStr)
		if !stageID.Valid {
			http.Error(w, "invalid stage: invalid uuid", http.StatusBadRequest)
			return
		}

		// Check payload size before reading body.
		if r.ContentLength > maxBodySize {
			http.Error(w, "payload exceeds body size cap", http.StatusRequestEntityTooLarge)
			return
		}

		// Limit request body size to avoid memory exhaustion but allow base64 overhead.
		r.Body = http.MaxBytesReader(w, r.Body, maxBodySize)

		// Decode request body.
		var req struct {
			RunID     string                  `json:"run_id"`
			Patch     []byte                  `json:"patch"`      // gzipped diff (raw bytes)
			Summary   domaintypes.DiffSummary `json:"summary"`    // optional summary metadata
			StepIndex *int32                  `json:"step_index"` // optional step identity (0-based) for multi-step runs
		}

		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			// Return 413 when MaxBytesReader trips the size cap.
			var maxErr *http.MaxBytesError
			if errors.As(err, &maxErr) {
				http.Error(w, "payload exceeds body size cap", http.StatusRequestEntityTooLarge)
				return
			}
			http.Error(w, fmt.Sprintf("invalid request: %v", err), http.StatusBadRequest)
			return
		}

		// Validate run_id is present.
		if strings.TrimSpace(req.RunID) == "" {
			http.Error(w, "run_id is required", http.StatusBadRequest)
			return
		}

		// Validate run_id is a valid UUID.
		runID := domaintypes.ToPGUUID(req.RunID)
		if !runID.Valid {
			http.Error(w, "invalid run_id: invalid uuid", http.StatusBadRequest)
			return
		}

		// Validate patch is present.
		if len(req.Patch) == 0 {
			http.Error(w, "patch is required", http.StatusBadRequest)
			return
		}

		// Enforce decoded patch size cap (≤ 1 MiB gzipped, base64-decoded here).
		if len(req.Patch) > maxPatchSize {
			http.Error(w, "diff size exceeds 1 MiB cap", http.StatusRequestEntityTooLarge)
			return
		}

		// Check if the node exists before processing.
		var err error
		_, err = st.GetNode(r.Context(), nodeID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				http.Error(w, "node not found", http.StatusNotFound)
				return
			}
			http.Error(w, fmt.Sprintf("failed to check node: %v", err), http.StatusInternalServerError)
			slog.Error("diff: node check failed", "node_id", nodeIDStr, "err", err)
			return
		}

		// Check if the run exists.
		_, err = st.GetRun(r.Context(), runID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				http.Error(w, "run not found", http.StatusNotFound)
				return
			}
			http.Error(w, fmt.Sprintf("failed to check run: %v", err), http.StatusInternalServerError)
			slog.Error("diff: run check failed", "run_id", req.RunID, "err", err)
			return
		}

		// Check if the stage exists.
		stage, err := st.GetStage(r.Context(), stageID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				http.Error(w, "stage not found", http.StatusNotFound)
				return
			}
			http.Error(w, fmt.Sprintf("failed to check stage: %v", err), http.StatusInternalServerError)
			slog.Error("diff: stage check failed", "stage_id", stageIDStr, "err", err)
			return
		}

		// Ensure the stage belongs to the provided run.
		if stage.RunID != runID {
			http.Error(w, "stage does not belong to run", http.StatusBadRequest)
			return
		}

		// Create diff params.
		summaryBytes, err := json.Marshal(req.Summary)
		if err != nil {
			http.Error(w, fmt.Sprintf("failed to marshal summary: %v", err), http.StatusBadRequest)
			return
		}

		// Validate step_index if provided: must be >= -1 per DB constraint.
		// C2: -1 is used for pre_gate diffs (healed baseline before step 0).
		if req.StepIndex != nil && *req.StepIndex < -1 {
			http.Error(w, "step_index must be >= -1", http.StatusBadRequest)
			return
		}

		params := store.CreateDiffParams{
			RunID:     runID,
			StageID:   stageID,
			Patch:     req.Patch,
			Summary:   summaryBytes,
			StepIndex: req.StepIndex,
		}

		// Persist diff to DB.
		diff, err := st.CreateDiff(r.Context(), params)
		if err != nil {
			// Check if the error is a constraint violation (size cap exceeded).
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code == "23514" { // check_violation
				http.Error(w, "diff size exceeds 1 MiB cap", http.StatusRequestEntityTooLarge)
				return
			}
			http.Error(w, fmt.Sprintf("failed to create diff: %v", err), http.StatusInternalServerError)
			slog.Error("diff: create failed", "node_id", nodeIDStr, "run_id", req.RunID, "stage_id", stageIDStr, "err", err)
			return
		}

		// Return success response with diff_id.
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		if err := json.NewEncoder(w).Encode(map[string]interface{}{"diff_id": uuid.UUID(diff.ID.Bytes).String()}); err != nil {
			slog.Error("diff: encode response failed", "err", err)
		}

		slog.Debug("diff created",
			"node_id", nodeIDStr,
			"run_id", req.RunID,
			"stage_id", stageIDStr,
			"diff_id", diff.ID.Bytes,
		)
	}
}
