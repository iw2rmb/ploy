package httpserver

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	httpsecurity "github.com/iw2rmb/ploy/internal/api/httpserver/security"
	controlplaneartifacts "github.com/iw2rmb/ploy/internal/controlplane/artifacts"
	workflowartifacts "github.com/iw2rmb/ploy/internal/workflow/artifacts"
)

func (s *controlPlaneServer) handleArtifactsList(w http.ResponseWriter, r *http.Request) {
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
	cid := strings.TrimSpace(query.Get("cid"))
	list, err := store.List(r.Context(), controlplaneartifacts.ListOptions{
		JobID:  jobID,
		Stage:  stage,
		CID:    cid,
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
		if wantsDownload(r.URL.Query().Get("download")) {
			if s.artifactPublisher == nil {
				status = http.StatusServiceUnavailable
				writeErrorMessage(w, status, "artifact download unavailable")
				return
			}
			result, err := s.artifactPublisher.Fetch(r.Context(), meta.CID)
			if err != nil {
				status = http.StatusBadGateway
				writeError(w, status, fmt.Errorf("fetch artifact: %w", err))
				return
			}
			w.Header().Set("Cache-Control", "no-store")
			w.Header().Set("Content-Type", "application/octet-stream")
			if result.Size > 0 {
				w.Header().Set("Content-Length", strconv.FormatInt(result.Size, 10))
			}
			if _, err := w.Write(result.Data); err != nil {
				status = http.StatusInternalServerError
			}
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
