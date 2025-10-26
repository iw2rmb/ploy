package httpserver

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	httpsecurity "github.com/iw2rmb/ploy/internal/api/httpserver/security"
	controlplaneartifacts "github.com/iw2rmb/ploy/internal/controlplane/artifacts"
	"github.com/iw2rmb/ploy/internal/controlplane/transfers"
	workflowartifacts "github.com/iw2rmb/ploy/internal/workflow/artifacts"
	gonanoid "github.com/matoous/go-nanoid/v2"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

func (s *controlPlaneServer) handleArtifactsList(w http.ResponseWriter, r *http.Request) {
	status := http.StatusOK
	defer func() {
		recordArtifactRequest(r.Method, status)
	}()
	if !s.requireScope(w, r, httpsecurity.ScopeArtifactsRead) {
		status = http.StatusForbidden
		return
	}
	store := s.artifacts
	if store == nil {
		status = http.StatusServiceUnavailable
		writeErrorMessage(w, status, "artifact store unavailable")
		return
	}
	query := r.URL.Query()
	limit, err := parseArtifactLimit(query.Get("limit"))
	if err != nil {
		status = http.StatusBadRequest
		writeErrorMessage(w, status, err.Error())
		return
	}
	jobID := strings.TrimSpace(query.Get("job_id"))
	stage := strings.TrimSpace(query.Get("stage"))
	if stage != "" && jobID == "" {
		status = http.StatusBadRequest
		writeErrorMessage(w, status, "stage filter requires job_id")
		return
	}
	list, err := store.List(r.Context(), controlplaneartifacts.ListOptions{
		JobID:  jobID,
		Stage:  stage,
		Cursor: strings.TrimSpace(query.Get("cursor")),
		Limit:  limit,
	})
	if err != nil {
		status = http.StatusInternalServerError
		writeError(w, status, err)
		return
	}
	kindFilter := strings.TrimSpace(query.Get("kind"))
	artifacts := make([]artifactDTO, 0, len(list.Artifacts))
	for _, meta := range list.Artifacts {
		if kindFilter != "" && !strings.EqualFold(kindFilter, strings.TrimSpace(meta.Kind)) {
			continue
		}
		artifacts = append(artifacts, artifactDTOFrom(meta))
	}
	response := map[string]any{"artifacts": artifacts}
	if strings.TrimSpace(list.NextCursor) != "" {
		response["next_cursor"] = list.NextCursor
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, status, response)
}

func (s *controlPlaneServer) handleArtifactsUpload(w http.ResponseWriter, r *http.Request) {
	status := http.StatusCreated
	defer func() { recordArtifactRequest(r.Method, status) }()
	if !s.requireScope(w, r, httpsecurity.ScopeArtifactsWrite) {
		status = http.StatusForbidden
		return
	}
	store := s.artifacts
	publisher := s.artifactPublisher
	if store == nil || publisher == nil {
		status = http.StatusServiceUnavailable
		writeErrorMessage(w, status, "artifact upload unavailable")
		return
	}
	query := r.URL.Query()
	jobID := strings.TrimSpace(query.Get("job_id"))
	if jobID == "" {
		status = http.StatusBadRequest
		writeErrorMessage(w, status, "job_id query parameter required")
		return
	}
	stage := strings.TrimSpace(query.Get("stage"))
	nodeID := strings.TrimSpace(query.Get("node_id"))
	kind := strings.TrimSpace(query.Get("kind"))
	ttl := strings.TrimSpace(query.Get("ttl"))
	expectedDigest := strings.TrimSpace(query.Get("digest"))
	name := strings.TrimSpace(query.Get("name"))
	if name == "" {
		name = fmt.Sprintf("artifact-%s", jobID)
	}
	replMin, err := parseOptionalIntParam(query.Get("replication_min"))
	if err != nil {
		status = http.StatusBadRequest
		writeErrorMessage(w, status, "invalid replication_min")
		return
	}
	replMax, err := parseOptionalIntParam(query.Get("replication_max"))
	if err != nil {
		status = http.StatusBadRequest
		writeErrorMessage(w, status, "invalid replication_max")
		return
	}
	if r.Body == nil {
		status = http.StatusBadRequest
		writeErrorMessage(w, status, "request body required")
		return
	}
	defer func() { _ = r.Body.Close() }()
	payload, err := io.ReadAll(r.Body)
	if err != nil {
		status = http.StatusInternalServerError
		writeError(w, status, fmt.Errorf("read upload payload: %w", err))
		return
	}
	if len(payload) == 0 {
		status = http.StatusBadRequest
		writeErrorMessage(w, status, "payload required")
		return
	}
	recordArtifactPayload("upload", int64(len(payload)))
	digest := sha256.Sum256(payload)
	computedDigest := "sha256:" + hex.EncodeToString(digest[:])
	if expectedDigest != "" && !strings.EqualFold(expectedDigest, computedDigest) {
		status = http.StatusBadRequest
		writeErrorMessage(w, status, "digest mismatch")
		return
	}
	addReq := workflowartifacts.AddRequest{
		Name:                 name,
		Payload:              payload,
		ReplicationFactorMin: replMin,
		ReplicationFactorMax: replMax,
	}
	result, err := publisher.Add(r.Context(), addReq)
	if err != nil {
		status = http.StatusBadGateway
		writeError(w, status, fmt.Errorf("publish artifact: %w", err))
		return
	}
	meta := controlplaneartifacts.Metadata{
		ID:                   generateArtifactID(),
		JobID:                jobID,
		Stage:                stage,
		Kind:                 kind,
		NodeID:               nodeID,
		CID:                  strings.TrimSpace(result.CID),
		Digest:               strings.TrimSpace(result.Digest),
		Size:                 result.Size,
		Name:                 strings.TrimSpace(result.Name),
		Source:               "http-upload",
		TTL:                  ttl,
		ReplicationFactorMin: firstNonZero(result.ReplicationFactorMin, replMin),
		ReplicationFactorMax: firstNonZero(result.ReplicationFactorMax, replMax),
	}
	if meta.CID == "" {
		meta.CID = "pending"
	}
	if meta.Digest == "" {
		meta.Digest = computedDigest
	}
	if meta.Size == 0 {
		meta.Size = int64(len(payload))
	}
	if meta.Name == "" {
		meta.Name = name
	}
	created, err := store.Create(r.Context(), meta)
	if err != nil {
		status = http.StatusInternalServerError
		writeError(w, status, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, status, map[string]any{"artifact": artifactDTOFrom(created)})
}

func (s *controlPlaneServer) handleArtifactsSubpath(w http.ResponseWriter, r *http.Request) {
	trimmed := strings.TrimPrefix(r.URL.Path, "/v1/artifacts/")
	trimmed = strings.Trim(trimmed, "/")
	if trimmed == "" {
		recordArtifactRequest(r.Method, http.StatusNotFound)
		http.NotFound(w, r)
		return
	}
	switch r.Method {
	case http.MethodGet:
		status := http.StatusOK
		defer func() { recordArtifactRequest(r.Method, status) }()
		if !s.requireScope(w, r, httpsecurity.ScopeArtifactsRead) {
			status = http.StatusForbidden
			return
		}
		store := s.artifacts
		if store == nil {
			status = http.StatusServiceUnavailable
			writeErrorMessage(w, status, "artifact store unavailable")
			return
		}
		meta, err := store.Get(r.Context(), trimmed)
		if err != nil {
			if errors.Is(err, controlplaneartifacts.ErrNotFound) {
				status = http.StatusNotFound
				writeErrorMessage(w, status, "artifact not found")
				return
			}
			status = http.StatusInternalServerError
			writeError(w, status, err)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		writeJSON(w, status, map[string]any{"artifact": artifactDTOFrom(meta)})
	case http.MethodDelete:
		status := http.StatusOK
		defer func() { recordArtifactRequest(r.Method, status) }()
		if !s.requireScope(w, r, httpsecurity.ScopeArtifactsWrite) {
			status = http.StatusForbidden
			return
		}
		store := s.artifacts
		if store == nil {
			status = http.StatusServiceUnavailable
			writeErrorMessage(w, status, "artifact store unavailable")
			return
		}
		meta, err := store.Delete(r.Context(), trimmed)
		if err != nil {
			if errors.Is(err, controlplaneartifacts.ErrNotFound) {
				status = http.StatusNotFound
				writeErrorMessage(w, status, "artifact not found")
				return
			}
			status = http.StatusInternalServerError
			writeError(w, status, err)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		writeJSON(w, status, map[string]any{"artifact": artifactDTOFrom(meta)})
	default:
		recordArtifactRequest(r.Method, http.StatusMethodNotAllowed)
		w.Header().Set("Allow", "GET, DELETE")
		writeErrorMessage(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

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

func generateArtifactID() string {
	id, err := gonanoid.Generate("abcdefghijklmnopqrstuvwxyz0123456789", 16)
	if err != nil {
		return fmt.Sprintf("artifact-%d", time.Now().UnixNano())
	}
	return "artifact-" + id
}

const (
	defaultArtifactLimit = 50
	maxArtifactLimit     = 200
)

func parseArtifactLimit(raw string) (int, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return defaultArtifactLimit, nil
	}
	limit, err := strconv.Atoi(trimmed)
	if err != nil || limit <= 0 {
		return 0, fmt.Errorf("invalid limit")
	}
	if limit > maxArtifactLimit {
		limit = maxArtifactLimit
	}
	return limit, nil
}

func parseOptionalIntParam(raw string) (int, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return 0, nil
	}
	value, err := strconv.Atoi(trimmed)
	if err != nil || value < 0 {
		return 0, fmt.Errorf("invalid integer")
	}
	return value, nil
}

func firstNonZero(values ...int) int {
	for _, v := range values {
		if v > 0 {
			return v
		}
	}
	return 0
}

func firstNonZero64(values ...int64) int64 {
	for _, v := range values {
		if v > 0 {
			return v
		}
	}
	return 0
}

type artifactDTO struct {
	ID                   string `json:"id"`
	JobID                string `json:"job_id"`
	Stage                string `json:"stage,omitempty"`
	Kind                 string `json:"kind,omitempty"`
	NodeID               string `json:"node_id,omitempty"`
	CID                  string `json:"cid"`
	Digest               string `json:"digest"`
	Size                 int64  `json:"size"`
	Name                 string `json:"name,omitempty"`
	Source               string `json:"source,omitempty"`
	TTL                  string `json:"ttl,omitempty"`
	ExpiresAt            string `json:"expires_at,omitempty"`
	ReplicationFactorMin int    `json:"replication_factor_min,omitempty"`
	ReplicationFactorMax int    `json:"replication_factor_max,omitempty"`
	CreatedAt            string `json:"created_at"`
	UpdatedAt            string `json:"updated_at"`
	DeletedAt            string `json:"deleted_at,omitempty"`
}

func artifactDTOFrom(meta controlplaneartifacts.Metadata) artifactDTO {
	dto := artifactDTO{
		ID:                   meta.ID,
		JobID:                meta.JobID,
		Stage:                meta.Stage,
		Kind:                 meta.Kind,
		NodeID:               meta.NodeID,
		CID:                  meta.CID,
		Digest:               meta.Digest,
		Size:                 meta.Size,
		Name:                 meta.Name,
		Source:               meta.Source,
		TTL:                  meta.TTL,
		ReplicationFactorMin: meta.ReplicationFactorMin,
		ReplicationFactorMax: meta.ReplicationFactorMax,
		CreatedAt:            formatTime(meta.CreatedAt),
		UpdatedAt:            formatTime(meta.UpdatedAt),
	}
	if !meta.ExpiresAt.IsZero() {
		dto.ExpiresAt = formatTime(meta.ExpiresAt)
	}
	if !meta.DeletedAt.IsZero() {
		dto.DeletedAt = formatTime(meta.DeletedAt)
	}
	return dto
}

func recordArtifactRequest(method string, status int) {
	method = strings.ToUpper(strings.TrimSpace(method))
	if method == "" {
		method = "UNKNOWN"
	}
	artifactRequestsTotal.WithLabelValues(method, strconv.Itoa(status)).Inc()
}

func recordArtifactPayload(operation string, bytesCopied int64) {
	if bytesCopied <= 0 {
		return
	}
	operation = strings.TrimSpace(operation)
	if operation == "" {
		operation = "unknown"
	}
	artifactPayloadBytes.WithLabelValues(operation).Add(float64(bytesCopied))
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
