package handlers

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"

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
		DrainRevision:   node.DrainRevision,
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

// Conditional maintenance releases cannot override a later operator decision.
func setNodeDrainHandler(st store.Store, drain bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		nodeID, ok := parseRequiredPathIDOrWriteError[domaintypes.NodeID](w, r, "id")
		if !ok {
			return
		}
		var revision *int64
		if raw, present := r.URL.Query()["expected_revision"]; present {
			value, err := strconv.ParseInt(raw[0], 10, 64)
			if err != nil || value < 0 {
				writeHTTPError(w, http.StatusBadRequest, "invalid expected_revision")
				return
			}
			revision = &value
		}
		node, err := st.UpdateNodeDrained(r.Context(), store.UpdateNodeDrainedParams{ID: nodeID, Drained: drain, ExpectedRevision: revision})
		if errors.Is(err, pgx.ErrNoRows) {
			if _, ok := getNodeOrFail(w, r, st, nodeID, "set drain"); !ok {
				return
			}
			writeHTTPError(w, http.StatusConflict, "node drain revision or state changed")
			return
		}
		if err != nil {
			writeHTTPError(w, http.StatusInternalServerError, "update node drain: %v", err)
			return
		}
		w.Header().Set("X-Ploy-Drain-Revision", strconv.FormatInt(node.DrainRevision, 10))
		w.WriteHeader(http.StatusNoContent)
		slog.Info("node drain updated", "node_id", nodeID, "drained", drain, "drain_revision", node.DrainRevision)
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
