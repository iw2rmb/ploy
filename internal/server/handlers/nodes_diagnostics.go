package handlers

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
)

const (
	nodeDaemonLogDefaultLimit = 200
	nodeDaemonLogMaxLimit     = 1000
	nodeDaemonLogKeepCount    = 1000
	nodeDaemonLogMaxLineBytes = 16 << 10
	nodeDaemonLogMaxLines     = 200
)

var validNodeDaemonComponents = map[string]bool{
	"node":         true,
	"node-updater": true,
}

var validNodeDaemonStreams = map[string]bool{
	"stdout": true,
	"stderr": true,
	"system": true,
}

type nodeDiagnosticResponse struct {
	NodeID        string          `json:"node_id"`
	Component     string          `json:"component"`
	Status        string          `json:"status"`
	LastError     *string         `json:"last_error,omitempty"`
	Version       *string         `json:"version,omitempty"`
	ImageRef      *string         `json:"image_ref,omitempty"`
	LocalImageID  *string         `json:"local_image_id,omitempty"`
	RemoteImageID *string         `json:"remote_image_id,omitempty"`
	Details       json.RawMessage `json:"details"`
	LastCheckedAt *string         `json:"last_checked_at,omitempty"`
	LastSuccessAt *string         `json:"last_success_at,omitempty"`
	UpdatedAt     string          `json:"updated_at"`
}

type nodeDaemonLogResponse struct {
	ID        int64   `json:"id"`
	NodeID    string  `json:"node_id"`
	Component string  `json:"component"`
	Stream    string  `json:"stream"`
	Message   string  `json:"message"`
	CreatedAt *string `json:"created_at,omitempty"`
}

type nodeDaemonLogsCreateResponse struct {
	Count int `json:"count"`
}

func upsertNodeDiagnosticHandler(st store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		nodeID, ok := parseRequiredPathIDOrWriteError[domaintypes.NodeID](w, r, "id")
		if !ok {
			return
		}

		var req struct {
			Component     string          `json:"component"`
			Status        string          `json:"status"`
			LastError     *string         `json:"last_error,omitempty"`
			Version       *string         `json:"version,omitempty"`
			ImageRef      *string         `json:"image_ref,omitempty"`
			LocalImageID  *string         `json:"local_image_id,omitempty"`
			RemoteImageID *string         `json:"remote_image_id,omitempty"`
			Details       json.RawMessage `json:"details,omitempty"`
			LastCheckedAt *string         `json:"last_checked_at,omitempty"`
			LastSuccessAt *string         `json:"last_success_at,omitempty"`
		}
		if err := decodeRequestJSON(w, r, &req, DefaultMaxBodySize); err != nil {
			return
		}

		component, status, ok := validateNodeDiagnosticRequest(w, req.Component, req.Status)
		if !ok {
			return
		}
		if _, ok := getNodeOrFail(w, r, st, nodeID, "node diagnostics"); !ok {
			return
		}
		checkedAt, ok := parseOptionalNodeTime(w, "last_checked_at", req.LastCheckedAt)
		if !ok {
			return
		}
		successAt, ok := parseOptionalNodeTime(w, "last_success_at", req.LastSuccessAt)
		if !ok {
			return
		}
		details := req.Details
		if len(details) == 0 {
			details = json.RawMessage(`{}`)
		}
		if !json.Valid(details) {
			writeHTTPError(w, http.StatusBadRequest, "details must be valid JSON")
			return
		}

		diag, err := st.UpsertNodeDiagnostic(r.Context(), store.UpsertNodeDiagnosticParams{
			NodeID:        nodeID,
			Component:     component,
			Status:        status,
			LastError:     trimOptionalString(req.LastError),
			Version:       trimOptionalString(req.Version),
			ImageRef:      trimOptionalString(req.ImageRef),
			LocalImageID:  trimOptionalString(req.LocalImageID),
			RemoteImageID: trimOptionalString(req.RemoteImageID),
			Details:       []byte(details),
			LastCheckedAt: checkedAt,
			LastSuccessAt: successAt,
		})
		if err != nil {
			writeHTTPError(w, http.StatusInternalServerError, "failed to upsert node diagnostic: %v", err)
			slog.Error("node diagnostics: upsert failed", "node_id", nodeID.String(), "component", component, "err", err)
			return
		}

		writeJSON(w, http.StatusOK, nodeDiagnosticToResponse(diag))
	}
}

func listNodeDiagnosticsHandler(st store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		nodeID, ok := parseRequiredPathIDOrWriteError[domaintypes.NodeID](w, r, "id")
		if !ok {
			return
		}
		if _, ok := getNodeOrFail(w, r, st, nodeID, "list node diagnostics"); !ok {
			return
		}
		diagnostics, err := st.ListNodeDiagnostics(r.Context(), nodeID)
		if err != nil {
			writeHTTPError(w, http.StatusInternalServerError, "failed to list node diagnostics: %v", err)
			slog.Error("node diagnostics: list failed", "node_id", nodeID.String(), "err", err)
			return
		}
		resp := make([]nodeDiagnosticResponse, 0, len(diagnostics))
		for _, diag := range diagnostics {
			resp = append(resp, nodeDiagnosticToResponse(diag))
		}
		writeJSON(w, http.StatusOK, resp)
	}
}

func createNodeDaemonLogsHandler(st store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		nodeID, ok := parseRequiredPathIDOrWriteError[domaintypes.NodeID](w, r, "id")
		if !ok {
			return
		}
		var req struct {
			Component string   `json:"component"`
			Stream    string   `json:"stream"`
			Lines     []string `json:"lines"`
		}
		if err := decodeRequestJSON(w, r, &req, DefaultMaxBodySize); err != nil {
			return
		}
		component := strings.TrimSpace(req.Component)
		stream := strings.TrimSpace(req.Stream)
		if !validNodeDaemonComponents[component] {
			writeHTTPError(w, http.StatusBadRequest, "invalid component")
			return
		}
		if !validNodeDaemonStreams[stream] {
			writeHTTPError(w, http.StatusBadRequest, "invalid stream")
			return
		}
		if len(req.Lines) == 0 {
			writeHTTPError(w, http.StatusBadRequest, "lines is required")
			return
		}
		if len(req.Lines) > nodeDaemonLogMaxLines {
			writeHTTPError(w, http.StatusBadRequest, "too many log lines")
			return
		}
		if _, ok := getNodeOrFail(w, r, st, nodeID, "node daemon logs"); !ok {
			return
		}
		for _, line := range req.Lines {
			message := strings.TrimRight(line, "\r\n")
			if strings.TrimSpace(message) == "" {
				continue
			}
			if len(message) > nodeDaemonLogMaxLineBytes {
				writeHTTPError(w, http.StatusBadRequest, "log line exceeds %d bytes", nodeDaemonLogMaxLineBytes)
				return
			}
			if _, err := st.CreateNodeDaemonLog(r.Context(), store.CreateNodeDaemonLogParams{
				NodeID:    nodeID,
				Component: component,
				Stream:    stream,
				Message:   message,
			}); err != nil {
				writeHTTPError(w, http.StatusInternalServerError, "failed to create node daemon log: %v", err)
				slog.Error("node daemon logs: create failed", "node_id", nodeID.String(), "component", component, "err", err)
				return
			}
		}
		if err := st.TrimNodeDaemonLogs(r.Context(), store.TrimNodeDaemonLogsParams{
			NodeID:    nodeID,
			Component: component,
			KeepCount: nodeDaemonLogKeepCount,
		}); err != nil {
			slog.Warn("node daemon logs: trim failed", "node_id", nodeID.String(), "component", component, "err", err)
		}
		writeJSON(w, http.StatusCreated, nodeDaemonLogsCreateResponse{Count: len(req.Lines)})
	}
}

func listNodeDaemonLogsHandler(st store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		nodeID, ok := parseRequiredPathIDOrWriteError[domaintypes.NodeID](w, r, "id")
		if !ok {
			return
		}
		if _, ok := getNodeOrFail(w, r, st, nodeID, "list node daemon logs"); !ok {
			return
		}
		var component *string
		if raw := strings.TrimSpace(r.URL.Query().Get("component")); raw != "" {
			if !validNodeDaemonComponents[raw] {
				writeHTTPError(w, http.StatusBadRequest, "invalid component")
				return
			}
			component = &raw
		}
		limit := parseNodeDaemonLogLimit(r)
		logs, err := st.ListNodeDaemonLogs(r.Context(), store.ListNodeDaemonLogsParams{
			NodeID:     nodeID,
			Component:  component,
			LimitCount: int32(limit),
		})
		if err != nil {
			writeHTTPError(w, http.StatusInternalServerError, "failed to list node daemon logs: %v", err)
			slog.Error("node daemon logs: list failed", "node_id", nodeID.String(), "err", err)
			return
		}
		resp := make([]nodeDaemonLogResponse, 0, len(logs))
		for _, log := range logs {
			resp = append(resp, nodeDaemonLogToResponse(log))
		}
		writeJSON(w, http.StatusOK, resp)
	}
}

func validateNodeDiagnosticRequest(w http.ResponseWriter, component, status string) (string, string, bool) {
	component = strings.TrimSpace(component)
	status = strings.TrimSpace(status)
	if !validNodeDaemonComponents[component] {
		writeHTTPError(w, http.StatusBadRequest, "invalid component")
		return "", "", false
	}
	if status == "" {
		writeHTTPError(w, http.StatusBadRequest, "status is required")
		return "", "", false
	}
	return component, status, true
}

func parseOptionalNodeTime(w http.ResponseWriter, field string, value *string) (pgtype.Timestamptz, bool) {
	if value == nil || strings.TrimSpace(*value) == "" {
		return pgtype.Timestamptz{}, true
	}
	parsed, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(*value))
	if err != nil {
		writeHTTPError(w, http.StatusBadRequest, "%s must be RFC3339", field)
		return pgtype.Timestamptz{}, false
	}
	return pgtype.Timestamptz{Time: parsed.UTC(), Valid: true}, true
}

func trimOptionalString(value *string) *string {
	if value == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*value)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

func parseNodeDaemonLogLimit(r *http.Request) int {
	raw := strings.TrimSpace(r.URL.Query().Get("limit"))
	if raw == "" {
		return nodeDaemonLogDefaultLimit
	}
	limit, err := strconv.Atoi(raw)
	if err != nil || limit <= 0 {
		return nodeDaemonLogDefaultLimit
	}
	if limit > nodeDaemonLogMaxLimit {
		return nodeDaemonLogMaxLimit
	}
	return limit
}

func nodeDiagnosticToResponse(diag store.NodeDiagnostic) nodeDiagnosticResponse {
	resp := nodeDiagnosticResponse{
		NodeID:        diag.NodeID.String(),
		Component:     diag.Component,
		Status:        diag.Status,
		LastError:     diag.LastError,
		Version:       diag.Version,
		ImageRef:      diag.ImageRef,
		LocalImageID:  diag.LocalImageID,
		RemoteImageID: diag.RemoteImageID,
		Details:       json.RawMessage(diag.Details),
	}
	if len(resp.Details) == 0 {
		resp.Details = json.RawMessage(`{}`)
	}
	if diag.LastCheckedAt.Valid {
		s := diag.LastCheckedAt.Time.Format(time.RFC3339)
		resp.LastCheckedAt = &s
	}
	if diag.LastSuccessAt.Valid {
		s := diag.LastSuccessAt.Time.Format(time.RFC3339)
		resp.LastSuccessAt = &s
	}
	if diag.UpdatedAt.Valid {
		resp.UpdatedAt = diag.UpdatedAt.Time.Format(time.RFC3339)
	}
	return resp
}

func nodeDaemonLogToResponse(log store.NodeDaemonLog) nodeDaemonLogResponse {
	resp := nodeDaemonLogResponse{
		ID:        log.ID,
		NodeID:    log.NodeID.String(),
		Component: log.Component,
		Stream:    log.Stream,
		Message:   log.Message,
	}
	if log.CreatedAt.Valid {
		s := log.CreatedAt.Time.Format(time.RFC3339)
		resp.CreatedAt = &s
	}
	return resp
}
