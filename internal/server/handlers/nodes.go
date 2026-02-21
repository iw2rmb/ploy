package handlers

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/iw2rmb/ploy/internal/store"
)

// drainNodeHandler marks a node as drained.
func drainNodeHandler(st store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		nodeID, err := ParseNodeIDParam(r, "id")
		if err != nil {
			httpErr(w, http.StatusBadRequest, "%s", err)
			return
		}

		// Verify node exists.
		node, err := st.GetNode(r.Context(), nodeID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				httpErr(w, http.StatusNotFound, "node not found")
				return
			}
			httpErr(w, http.StatusInternalServerError, "failed to get node: %v", err)
			slog.Error("drain node: lookup failed", "node_id", nodeID, "err", err)
			return
		}

		// Check if already drained (409 Conflict).
		if node.Drained {
			httpErr(w, http.StatusConflict, "node is already drained")
			return
		}

		// Update drained flag.
		err = st.UpdateNodeDrained(r.Context(), store.UpdateNodeDrainedParams{ID: nodeID, Drained: true})
		if err != nil {
			httpErr(w, http.StatusInternalServerError, "failed to drain node: %v", err)
			slog.Error("drain node: update failed", "node_id", nodeID, "err", err)
			return
		}

		w.WriteHeader(http.StatusNoContent)
		slog.Info("node drained", "node_id", nodeID, "name", node.Name)
	}
}

// undrainNodeHandler marks a node as undrained (active).
func undrainNodeHandler(st store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		nodeID, err := ParseNodeIDParam(r, "id")
		if err != nil {
			httpErr(w, http.StatusBadRequest, "%s", err)
			return
		}

		// Verify node exists.
		node, err := st.GetNode(r.Context(), nodeID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				httpErr(w, http.StatusNotFound, "node not found")
				return
			}
			httpErr(w, http.StatusInternalServerError, "failed to get node: %v", err)
			slog.Error("undrain node: lookup failed", "node_id", nodeID, "err", err)
			return
		}

		// Check if already undrained (409 Conflict).
		if !node.Drained {
			httpErr(w, http.StatusConflict, "node is not drained")
			return
		}

		// Update drained flag.
		err = st.UpdateNodeDrained(r.Context(), store.UpdateNodeDrainedParams{ID: nodeID, Drained: false})
		if err != nil {
			httpErr(w, http.StatusInternalServerError, "failed to undrain node: %v", err)
			slog.Error("undrain node: update failed", "node_id", nodeID, "err", err)
			return
		}

		w.WriteHeader(http.StatusNoContent)
		slog.Info("node undrained", "node_id", nodeID, "name", node.Name)
	}
}

// listNodesHandler returns all nodes.
func listNodesHandler(st store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		nodes, err := st.ListNodes(r.Context())
		if err != nil {
			httpErr(w, http.StatusInternalServerError, "failed to list nodes: %v", err)
			slog.Error("list nodes: query failed", "err", err)
			return
		}

		// Build response slice.
		type nodeResponse struct {
			ID              string  `json:"id"`
			Name            string  `json:"name"`
			IPAddress       string  `json:"ip_address"`
			Version         *string `json:"version,omitempty"`
			Concurrency     int32   `json:"concurrency"`
			CPUTotalMillis  int32   `json:"cpu_total_millis"`
			CPUFreeMillis   int32   `json:"cpu_free_millis"`
			MemTotalBytes   int64   `json:"mem_total_bytes"`
			MemFreeBytes    int64   `json:"mem_free_bytes"`
			DiskTotalBytes  int64   `json:"disk_total_bytes"`
			DiskFreeBytes   int64   `json:"disk_free_bytes"`
			CertSerial      *string `json:"cert_serial,omitempty"`
			CertFingerprint *string `json:"cert_fingerprint,omitempty"`
			CertNotBefore   *string `json:"cert_not_before,omitempty"`
			CertNotAfter    *string `json:"cert_not_after,omitempty"`
			LastHeartbeat   *string `json:"last_heartbeat,omitempty"`
			Drained         bool    `json:"drained"`
			CreatedAt       string  `json:"created_at"`
		}

		resp := make([]nodeResponse, 0, len(nodes))
		for _, node := range nodes {
			nr := nodeResponse{
				ID:              node.ID.String(),
				Name:            node.Name,
				IPAddress:       node.IpAddress.String(),
				Version:         node.Version,
				Concurrency:     node.Concurrency,
				CPUTotalMillis:  node.CpuTotalMillis,
				CPUFreeMillis:   node.CpuFreeMillis,
				MemTotalBytes:   node.MemTotalBytes,
				MemFreeBytes:    node.MemFreeBytes,
				DiskTotalBytes:  node.DiskTotalBytes,
				DiskFreeBytes:   node.DiskFreeBytes,
				CertSerial:      node.CertSerial,
				CertFingerprint: node.CertFingerprint,
				Drained:         node.Drained,
				CreatedAt:       node.CreatedAt.Time.Format(time.RFC3339),
			}

			if node.CertNotBefore.Valid {
				s := node.CertNotBefore.Time.Format(time.RFC3339)
				nr.CertNotBefore = &s
			}
			if node.CertNotAfter.Valid {
				s := node.CertNotAfter.Time.Format(time.RFC3339)
				nr.CertNotAfter = &s
			}
			if node.LastHeartbeat.Valid {
				s := node.LastHeartbeat.Time.Format(time.RFC3339)
				nr.LastHeartbeat = &s
			}

			resp = append(resp, nr)
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			slog.Error("list nodes: encode response failed", "err", err)
		}
	}
}
