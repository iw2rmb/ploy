package handlers

import (
	"log/slog"
	"net/http"
	"time"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
)

// nodeResponse is the JSON shape returned by GET /v1/nodes.
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

func nodeToResponse(node store.Node) nodeResponse {
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
	return nr
}

// drainNodeHandler marks a node as drained.
func drainNodeHandler(st store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		nodeID, ok := parseRequiredPathIDOrWriteError[domaintypes.NodeID](w, r, "id")
		if !ok {
			return
		}

		node, ok := getNodeOrFail(w, r, st, nodeID, "drain node")
		if !ok {
			return
		}

		if node.Drained {
			writeHTTPError(w, http.StatusConflict, "node is already drained")
			return
		}

		if err := st.UpdateNodeDrained(r.Context(), store.UpdateNodeDrainedParams{ID: nodeID, Drained: true}); err != nil {
			writeHTTPError(w, http.StatusInternalServerError, "failed to drain node: %v", err)
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
		nodeID, ok := parseRequiredPathIDOrWriteError[domaintypes.NodeID](w, r, "id")
		if !ok {
			return
		}

		node, ok := getNodeOrFail(w, r, st, nodeID, "undrain node")
		if !ok {
			return
		}

		if !node.Drained {
			writeHTTPError(w, http.StatusConflict, "node is not drained")
			return
		}

		if err := st.UpdateNodeDrained(r.Context(), store.UpdateNodeDrainedParams{ID: nodeID, Drained: false}); err != nil {
			writeHTTPError(w, http.StatusInternalServerError, "failed to undrain node: %v", err)
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
			writeHTTPError(w, http.StatusInternalServerError, "failed to list nodes: %v", err)
			slog.Error("list nodes: query failed", "err", err)
			return
		}

		resp := make([]nodeResponse, 0, len(nodes))
		for _, node := range nodes {
			resp = append(resp, nodeToResponse(node))
		}

		writeJSON(w, http.StatusOK, resp)
	}
}
