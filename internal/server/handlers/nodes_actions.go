package handlers

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
)

type createNodeActionRequest struct {
	ActionType string          `json:"action_type"`
	Meta       json.RawMessage `json:"meta,omitempty"`
}

type nodeActionResponse struct {
	ID         string          `json:"id"`
	NodeID     string          `json:"node_id"`
	ActionType string          `json:"action_type"`
	Status     string          `json:"status"`
	StartedAt  *string         `json:"started_at,omitempty"`
	FinishedAt *string         `json:"finished_at,omitempty"`
	DurationMs int64           `json:"duration_ms"`
	Meta       json.RawMessage `json:"meta,omitempty"`
	Result     json.RawMessage `json:"result,omitempty"`
	CreatedAt  string          `json:"created_at,omitempty"`
}

func createNodeActionHandler(st store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		nodeID, ok := parseRequiredPathIDOrWriteError[domaintypes.NodeID](w, r, "id")
		if !ok {
			return
		}
		if _, ok := getNodeOrFail(w, r, st, nodeID, "create node action"); !ok {
			return
		}
		req, err := decodeCreateNodeActionRequest(r)
		if err != nil {
			writeHTTPError(w, http.StatusBadRequest, "%v", err)
			return
		}
		if !domaintypes.IsNodeActionType(req.ActionType) {
			writeHTTPError(w, http.StatusBadRequest, "unsupported node action_type %q", req.ActionType)
			return
		}
		meta := req.Meta
		if len(meta) == 0 {
			meta = []byte(`{}`)
		}
		action, err := st.CreateNodeAction(r.Context(), store.CreateNodeActionParams{
			ID:         domaintypes.NewJobID(),
			NodeID:     nodeID,
			ActionType: req.ActionType,
			Status:     domaintypes.JobStatusQueued,
			Meta:       meta,
		})
		if err != nil {
			writeHTTPError(w, http.StatusInternalServerError, "failed to create node action: %v", err)
			return
		}
		writeJSON(w, http.StatusCreated, nodeActionToResponse(action))
	}
}

func listNodeActionsHandler(st store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		nodeID, ok := parseRequiredPathIDOrWriteError[domaintypes.NodeID](w, r, "id")
		if !ok {
			return
		}
		limit, err := parseNodeActionLimit(r)
		if err != nil {
			writeHTTPError(w, http.StatusBadRequest, "%v", err)
			return
		}
		actions, err := st.ListNodeActions(r.Context(), store.ListNodeActionsParams{NodeID: nodeID, Limit: limit})
		if err != nil {
			writeHTTPError(w, http.StatusInternalServerError, "failed to list node actions: %v", err)
			return
		}
		resp := make([]nodeActionResponse, 0, len(actions))
		for _, action := range actions {
			resp = append(resp, nodeActionToResponse(action))
		}
		writeJSON(w, http.StatusOK, resp)
	}
}

func decodeCreateNodeActionRequest(r *http.Request) (createNodeActionRequest, error) {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	var req createNodeActionRequest
	if err := dec.Decode(&req); err != nil {
		return createNodeActionRequest{}, err
	}
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			return createNodeActionRequest{}, errors.New("request body must contain exactly one JSON value")
		}
		return createNodeActionRequest{}, err
	}
	req.ActionType = strings.TrimSpace(req.ActionType)
	if req.ActionType == "" {
		return createNodeActionRequest{}, errors.New("action_type is required")
	}
	return req, nil
}

func parseNodeActionLimit(r *http.Request) (int32, error) {
	raw := strings.TrimSpace(r.URL.Query().Get("limit"))
	if raw == "" {
		return 20, nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 1 || n > 100 {
		return 0, errors.New("limit must be an integer between 1 and 100")
	}
	return int32(n), nil
}

func nodeActionToResponse(action store.NodeAction) nodeActionResponse {
	resp := nodeActionResponse{
		ID:         action.ID.String(),
		NodeID:     action.NodeID.String(),
		ActionType: action.ActionType,
		Status:     action.Status.String(),
		DurationMs: action.DurationMs,
		Meta:       action.Meta,
		Result:     action.Result,
	}
	if action.StartedAt.Valid {
		resp.StartedAt = formatTimePtr(action.StartedAt.Time)
	}
	if action.FinishedAt.Valid {
		resp.FinishedAt = formatTimePtr(action.FinishedAt.Time)
	}
	if action.CreatedAt.Valid {
		resp.CreatedAt = action.CreatedAt.Time.Format(time.RFC3339)
	}
	return resp
}

func formatTimePtr(t time.Time) *string {
	s := t.Format(time.RFC3339)
	return &s
}
