package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
)

type completeActionRequest struct {
	Status string          `json:"status"`
	Stats  json.RawMessage `json:"stats,omitempty"`
}

func completeActionHandler(st store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		actionID, err := parseRequiredPathID[domaintypes.JobID](r, "action_id")
		if err != nil {
			writeHTTPError(w, http.StatusBadRequest, "%s", err)
			return
		}
		nodeID, status, stats, err := validateCompleteActionRequest(r)
		if err != nil {
			writeHTTPError(w, http.StatusBadRequest, "%v", err)
			return
		}
		if err := completeAction(r.Context(), st, actionID, nodeID, status, stats); err != nil {
			switch {
			case errors.Is(err, pgx.ErrNoRows):
				writeHTTPError(w, http.StatusNotFound, "action not found")
			case errors.Is(err, errForbiddenActionOwner):
				writeHTTPError(w, http.StatusForbidden, "action not assigned to this node")
			case errors.Is(err, errActionNotRunning):
				writeHTTPError(w, http.StatusConflict, "action status is not Running")
			default:
				writeHTTPError(w, http.StatusInternalServerError, "complete action failed: %v", err)
			}
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

var (
	errForbiddenActionOwner = errors.New("action not assigned to node")
	errActionNotRunning     = errors.New("action is not running")
)

func completeAction(
	ctx context.Context,
	st store.Store,
	actionID domaintypes.JobID,
	nodeID domaintypes.NodeID,
	status domaintypes.JobStatus,
	stats JobStatsPayload,
) error {
	action, err := st.GetRunRepoAction(ctx, actionID)
	if err != nil {
		return err
	}
	if action.NodeID == nil || *action.NodeID != nodeID {
		return errForbiddenActionOwner
	}
	if action.Status != domaintypes.JobStatusRunning {
		return errActionNotRunning
	}

	meta := map[string]any{}
	if errText := stats.ErrorMessage(); errText != "" {
		meta["error"] = errText
	}
	if mrURL := stats.MRURL(); mrURL != "" {
		meta["mr_url"] = mrURL
	}
	metaBytes, err := json.Marshal(meta)
	if err != nil {
		return err
	}
	if err := st.UpdateRunRepoActionCompletion(ctx, store.UpdateRunRepoActionCompletionParams{
		ID:     actionID,
		Status: status,
		Meta:   metaBytes,
	}); err != nil {
		return err
	}
	if status == domaintypes.JobStatusSuccess {
		if mrURL := stats.MRURL(); mrURL != "" {
			if err := st.UpdateRunStatsMRURL(ctx, store.UpdateRunStatsMRURLParams{ID: action.RunID, MrUrl: mrURL}); err != nil {
				slog.Error("complete action: failed to merge MR URL into run stats", "action_id", actionID, "run_id", action.RunID, "err", err)
			}
		}
	}
	return nil
}

func validateCompleteActionRequest(r *http.Request) (domaintypes.NodeID, domaintypes.JobStatus, JobStatsPayload, error) {
	nodeIDHeaderStr := strings.TrimSpace(r.Header.Get(nodeUUIDHeader))
	if nodeIDHeaderStr == "" {
		return "", "", JobStatsPayload{}, errors.New("PLOY_NODE_UUID header is required")
	}
	var nodeID domaintypes.NodeID
	if err := nodeID.UnmarshalText([]byte(nodeIDHeaderStr)); err != nil {
		return "", "", JobStatsPayload{}, errors.New("invalid PLOY_NODE_UUID header")
	}

	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	var req completeActionRequest
	if err := dec.Decode(&req); err != nil {
		return "", "", JobStatsPayload{}, err
	}
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			return "", "", JobStatsPayload{}, errors.New("request body must contain exactly one JSON value")
		}
		return "", "", JobStatsPayload{}, err
	}
	normalizedStatus, err := domaintypes.ParseJobStatus(strings.TrimSpace(req.Status))
	if err != nil {
		return "", "", JobStatsPayload{}, err
	}
	switch normalizedStatus {
	case domaintypes.JobStatusSuccess, domaintypes.JobStatusFail, domaintypes.JobStatusError, domaintypes.JobStatusCancelled:
	default:
		return "", "", JobStatsPayload{}, errors.New("status must be Success, Fail, Error, or Cancelled")
	}

	var stats JobStatsPayload
	if len(req.Stats) > 0 {
		if err := json.Unmarshal(req.Stats, &stats); err != nil {
			return "", "", JobStatsPayload{}, err
		}
	}
	return nodeID, normalizedStatus, stats, nil
}
