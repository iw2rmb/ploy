package handlers

import (
	"log/slog"
	"net/http"
	"time"

	domainapi "github.com/iw2rmb/ploy/internal/domain/api"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
)

func nodeToResponse(node store.Node) domainapi.Node {
	nr := domainapi.Node{
		ID:              node.ID,
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
func drainNodeHandler(st store.Store) http.HandlerFunc { return setNodeDrainHandler(st, true) }

// undrainNodeHandler marks a node as undrained (active).
func undrainNodeHandler(st store.Store) http.HandlerFunc { return setNodeDrainHandler(st, false) }

// setNodeDrainHandler returns a handler that toggles a node's drained flag.
// When drain=true it sets drained=true (409 if already drained); when false it clears it.
func setNodeDrainHandler(st store.Store, drain bool) http.HandlerFunc {
	action := "drain"
	conflictMsg := "node is already drained"
	logDone := "node drained"
	if !drain {
		action = "undrain"
		conflictMsg = "node is not drained"
		logDone = "node undrained"
	}
	return func(w http.ResponseWriter, r *http.Request) {
		nodeID, ok := parseRequiredPathIDOrWriteError[domaintypes.NodeID](w, r, "id")
		if !ok {
			return
		}

		node, ok := getNodeOrFail(w, r, st, nodeID, action+" node")
		if !ok {
			return
		}

		if node.Drained == drain {
			writeHTTPError(w, http.StatusConflict, "%s", conflictMsg)
			return
		}

		if err := st.UpdateNodeDrained(r.Context(), store.UpdateNodeDrainedParams{ID: nodeID, Drained: drain}); err != nil {
			writeHTTPError(w, http.StatusInternalServerError, "failed to %s node: %v", action, err)
			slog.Error(action+" node: update failed", "node_id", nodeID, "err", err)
			return
		}

		w.WriteHeader(http.StatusNoContent)
		slog.Info(logDone, "node_id", nodeID, "name", node.Name)
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

		resp := make([]domainapi.Node, 0, len(nodes))
		for _, node := range nodes {
			resp = append(resp, nodeToResponse(node))
		}

		writeJSON(w, http.StatusOK, resp)
	}
}
