package handlers

import (
	"errors"
	"log/slog"
	"math"
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
		nodeID, err := parseRequiredPathID[domaintypes.NodeID](r, "id")
		if err != nil {
			writeHTTPError(w, http.StatusBadRequest, "%s", err)
			return
		}

		// Decode request body with strict validation.
		var req struct {
			CPUFreeMillis  int64  `json:"cpu_free_millis"`
			CPUTotalMillis int64  `json:"cpu_total_millis"`
			MemFreeBytes   int64  `json:"mem_free_bytes"`
			MemTotalBytes  int64  `json:"mem_total_bytes"`
			DiskFreeBytes  int64  `json:"disk_free_bytes"`
			DiskTotalBytes int64  `json:"disk_total_bytes"`
			Version        string `json:"version,omitempty"`
		}

		if err := decodeRequestJSON(w, r, &req, DefaultMaxBodySize); err != nil {
			return
		}

		// Validate fit-range and invariants.
		if req.CPUFreeMillis < 0 || req.CPUTotalMillis < 0 {
			writeHTTPError(w, http.StatusBadRequest, "invalid request: cpu millis must be non-negative")
			return
		}
		if req.CPUFreeMillis > req.CPUTotalMillis {
			writeHTTPError(w, http.StatusBadRequest, "invalid request: cpu_free_millis must be <= cpu_total_millis")
			return
		}
		if req.CPUFreeMillis > math.MaxInt32 || req.CPUTotalMillis > math.MaxInt32 {
			writeHTTPError(w, http.StatusBadRequest, "invalid request: cpu millis out of range")
			return
		}
		if req.MemFreeBytes < 0 || req.MemTotalBytes < 0 {
			writeHTTPError(w, http.StatusBadRequest, "invalid request: mem bytes must be non-negative")
			return
		}
		if req.MemFreeBytes > req.MemTotalBytes {
			writeHTTPError(w, http.StatusBadRequest, "invalid request: mem_free_bytes must be <= mem_total_bytes")
			return
		}
		if req.DiskFreeBytes < 0 || req.DiskTotalBytes < 0 {
			writeHTTPError(w, http.StatusBadRequest, "invalid request: disk bytes must be non-negative")
			return
		}
		if req.DiskFreeBytes > req.DiskTotalBytes {
			writeHTTPError(w, http.StatusBadRequest, "invalid request: disk_free_bytes must be <= disk_total_bytes")
			return
		}

		// Check if the node exists before attempting to update.
		_, err = st.GetNode(r.Context(), nodeID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				writeHTTPError(w, http.StatusNotFound, "node not found")
				return
			}
			writeHTTPError(w, http.StatusInternalServerError, "failed to check node: %v", err)
			slog.Error("heartbeat: check failed", "node_id", nodeID, "err", err)
			return
		}

		// Update node heartbeat with resource snapshot.
		err = st.UpdateNodeHeartbeat(r.Context(), store.UpdateNodeHeartbeatParams{
			ID: nodeID,
			LastHeartbeat: pgtype.Timestamptz{
				Time:  time.Now().UTC(),
				Valid: true,
			},
			CpuTotalMillis: int32(req.CPUTotalMillis),
			CpuFreeMillis:  int32(req.CPUFreeMillis),
			MemTotalBytes:  req.MemTotalBytes,
			MemFreeBytes:   req.MemFreeBytes,
			DiskTotalBytes: req.DiskTotalBytes,
			DiskFreeBytes:  req.DiskFreeBytes,
			Version:        strings.TrimSpace(req.Version),
		})
		if err != nil {
			writeHTTPError(w, http.StatusInternalServerError, "failed to update heartbeat: %v", err)
			slog.Error("heartbeat: update failed", "node_id", nodeID, "err", err)
			return
		}

		w.WriteHeader(http.StatusNoContent)
		slog.Debug("heartbeat updated",
			"node_id", nodeID,
			"cpu_free_millis", req.CPUFreeMillis,
			"mem_free_bytes", req.MemFreeBytes,
			"disk_free_bytes", req.DiskFreeBytes,
			"version", req.Version,
		)
	}
}
