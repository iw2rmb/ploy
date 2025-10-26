package httpserver

import (
	"encoding/json"
	"net/http"
	"strings"

	httpsecurity "github.com/iw2rmb/ploy/internal/api/httpserver/security"
	"github.com/iw2rmb/ploy/internal/controlplane/transfers"
)

func (s *controlPlaneServer) handleTransfersUpload(w http.ResponseWriter, r *http.Request) {
	if !s.requireScope(w, r, httpsecurity.ScopeArtifactsWrite) {
		return
	}
	var payload transferUploadRequest
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeErrorMessage(w, http.StatusBadRequest, "invalid upload payload")
		return
	}
	slot, err := s.transfers.CreateUploadSlot(
		parseTransferKind(payload.Kind),
		strings.TrimSpace(payload.JobID),
		strings.TrimSpace(payload.Stage),
		strings.TrimSpace(payload.NodeID),
		payload.Size,
	)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	slot.Digest = strings.TrimSpace(payload.Digest)
	writeJSON(w, http.StatusOK, slot)
}

func (s *controlPlaneServer) handleTransfersDownload(w http.ResponseWriter, r *http.Request) {
	if !s.requireScope(w, r, httpsecurity.ScopeArtifactsRead) {
		return
	}
	var payload transferDownloadRequest
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeErrorMessage(w, http.StatusBadRequest, "invalid download payload")
		return
	}
	slot, artifact, err := s.transfers.CreateDownloadSlot(strings.TrimSpace(payload.JobID), strings.TrimSpace(payload.ArtifactID), parseTransferKind(payload.Kind))
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	slot.Digest = artifact.Digest
	writeJSON(w, http.StatusOK, slot)
}

func (s *controlPlaneServer) handleTransfersSlotAction(w http.ResponseWriter, r *http.Request) {
	trimmed := strings.TrimPrefix(r.URL.Path, "/v1/transfers/")
	trimmed = strings.Trim(trimmed, "/")
	parts := strings.Split(trimmed, "/")
	if len(parts) != 2 {
		writeErrorMessage(w, http.StatusNotFound, "slot not found")
		return
	}
	slotID := strings.TrimSpace(parts[0])
	action := strings.TrimSpace(parts[1])
	switch action {
	case "commit":
		var payload transferCommitRequest
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			writeErrorMessage(w, http.StatusBadRequest, "invalid commit payload")
			return
		}
		slot, err := s.transfers.Commit(r.Context(), slotID, payload.Size, strings.TrimSpace(payload.Digest))
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, slot)
	case "abort":
		s.transfers.Abort(slotID)
		writeJSON(w, http.StatusOK, map[string]any{"slot_id": slotID, "state": "aborted"})
	default:
		writeErrorMessage(w, http.StatusNotFound, "unknown slot action")
	}
}

type transferUploadRequest struct {
	JobID  string `json:"job_id"`
	Stage  string `json:"stage,omitempty"`
	Kind   string `json:"kind"`
	NodeID string `json:"node_id"`
	Size   int64  `json:"size"`
	Digest string `json:"digest"`
}

type transferDownloadRequest struct {
	JobID      string `json:"job_id"`
	Kind       string `json:"kind"`
	ArtifactID string `json:"artifact_id"`
}

type transferCommitRequest struct {
	Size   int64  `json:"size"`
	Digest string `json:"digest"`
}

func parseTransferKind(value string) transfers.Kind {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "logs":
		return transfers.KindLogs
	case "report":
		return transfers.KindReport
	default:
		return transfers.KindRepo
	}
}
