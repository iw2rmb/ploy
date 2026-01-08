package handlers

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
)

// heartbeatHandler updates node heartbeat and resource snapshot.
func heartbeatHandler(st store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Extract node id from path parameter.
		nodeIDStr, err := requiredPathParam(r, "id")
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		nodeID := domaintypes.NodeID(nodeIDStr)

		// Decode request body with strict validation.
		var req struct {
			CPUFreeMilli  float64 `json:"cpu_free_millis"`
			CPUTotalMilli float64 `json:"cpu_total_millis"`
			MemFreeMB     float64 `json:"mem_free_mb"`
			MemTotalMB    float64 `json:"mem_total_mb"`
			DiskFreeMB    float64 `json:"disk_free_mb"`
			DiskTotalMB   float64 `json:"disk_total_mb"`
			Version       string  `json:"version,omitempty"`
		}

		if err := DecodeJSON(w, r, &req, DefaultMaxBodySize); err != nil {
			return
		}

		// Check if the node exists before attempting to update.
		_, err = st.GetNode(r.Context(), nodeID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				http.Error(w, "node not found", http.StatusNotFound)
				return
			}
			http.Error(w, fmt.Sprintf("failed to check node: %v", err), http.StatusInternalServerError)
			slog.Error("heartbeat: check failed", "node_id", nodeID, "err", err)
			return
		}

		// Convert MB to bytes (1 MB = 1048576 bytes).
		const mbToBytes = 1048576

		// Update node heartbeat with resource snapshot.
		err = st.UpdateNodeHeartbeat(r.Context(), store.UpdateNodeHeartbeatParams{
			ID: nodeID,
			LastHeartbeat: pgtype.Timestamptz{
				Time:  time.Now().UTC(),
				Valid: true,
			},
			CpuTotalMillis: int32(req.CPUTotalMilli),
			CpuFreeMillis:  int32(req.CPUFreeMilli),
			MemTotalBytes:  int64(req.MemTotalMB * mbToBytes),
			MemFreeBytes:   int64(req.MemFreeMB * mbToBytes),
			DiskTotalBytes: int64(req.DiskTotalMB * mbToBytes),
			DiskFreeBytes:  int64(req.DiskFreeMB * mbToBytes),
			Version:        strings.TrimSpace(req.Version),
		})
		if err != nil {
			http.Error(w, fmt.Sprintf("failed to update heartbeat: %v", err), http.StatusInternalServerError)
			slog.Error("heartbeat: update failed", "node_id", nodeID, "err", err)
			return
		}

		w.WriteHeader(http.StatusNoContent)
		slog.Debug("heartbeat updated",
			"node_id", nodeID,
			"cpu_free_millis", req.CPUFreeMilli,
			"mem_free_mb", req.MemFreeMB,
			"disk_free_mb", req.DiskFreeMB,
			"version", req.Version,
		)
	}
}
