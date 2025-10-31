// registry_blobs.go houses blob upload/download handlers for the registry surface.
package httpserver

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	httpsecurity "github.com/iw2rmb/ploy/internal/api/httpserver/security"
	"github.com/iw2rmb/ploy/internal/controlplane/registry"
	"github.com/iw2rmb/ploy/internal/controlplane/transfers"
	workflowartifacts "github.com/iw2rmb/ploy/internal/workflow/artifacts"
)

// handleRegistryBlobs routes blob requests between uploads, sessions, and direct blob fetch/delete.
func (s *controlPlaneServer) handleRegistryBlobs(w http.ResponseWriter, r *http.Request, repo string, parts []string) {
	if len(parts) == 0 {
		recordRegistryRequest("blobs", r.Method, http.StatusNotFound)
		http.NotFound(w, r)
		return
	}
	resource := strings.TrimSpace(parts[0])
	if resource == "uploads" {
		if len(parts) == 1 {
			s.handleRegistryUploadStart(w, r, repo)
			return
		}
		sessionID := strings.TrimSpace(parts[1])
		if sessionID == "" {
			recordRegistryRequest("uploads", r.Method, http.StatusNotFound)
			http.NotFound(w, r)
			return
		}
		s.handleRegistryUploadSession(w, r, repo, sessionID)
		return
	}
	digest := resource
	if digest == "" {
		recordRegistryRequest("blobs", r.Method, http.StatusNotFound)
		http.NotFound(w, r)
		return
	}
	s.handleRegistryBlob(w, r, repo, digest)
}

// handleRegistryUploadStart initializes a new upload slot for streaming a blob payload.
func (s *controlPlaneServer) handleRegistryUploadStart(w http.ResponseWriter, r *http.Request, repo string) {
	status := http.StatusAccepted
	defer func() { recordRegistryRequest("uploads", r.Method, status) }()
	if r.Method != http.MethodPost {
		status = http.StatusMethodNotAllowed
		w.Header().Set("Allow", http.MethodPost)
		writeErrorMessage(w, status, "method not allowed")
		return
	}
	if !s.requireScope(w, r, httpsecurity.ScopeRegistryPush) {
		status = http.StatusForbidden
		return
	}

    // Fast-path: accept entire blob in one call when digest query is provided or octet-stream payload is present.
    digest := strings.TrimSpace(r.URL.Query().Get("digest"))
    if ct := strings.ToLower(strings.TrimSpace(r.Header.Get("Content-Type"))); ct == "application/octet-stream" || isDigest(digest) {
        // Read the payload exactly once; avoid converting to string to preserve binary content.
        data, err := io.ReadAll(r.Body)
        if err != nil { status = http.StatusInternalServerError; writeError(w, status, err); return }
        if len(data) == 0 { status = http.StatusBadRequest; writeErrorMessage(w, status, "empty blob payload"); return }
        if !isDigest(digest) {
            sum := workflowartifacts.SHA256Bytes(data)
            digest = "sha256:" + sum
        }
        // Publish as artifact then register blob
        publisher := s.artifactPublisher
        store := s.registryStore
        if publisher == nil || store == nil { status = http.StatusServiceUnavailable; writeErrorMessage(w, status, "registry unavailable"); return }
        addResp, err := publisher.Add(r.Context(), workflowartifacts.AddRequest{Name: fmt.Sprintf("blob-%s", strings.ReplaceAll(digest, ":", "-")), Payload: data})
        if err != nil { status = http.StatusBadGateway; writeError(w, status, err); return }
        media := strings.TrimSpace(r.Header.Get("Docker-Upload-Media-Type"))
        if media == "" { media = strings.TrimSpace(r.Header.Get("Content-Type")) }
        if media == "" { media = "application/octet-stream" }
        doc := registry.BlobDocument{Repo: repo, Digest: digest, MediaType: media, Size: int64(len(data)), CID: addResp.CID, Status: registry.BlobStatusAvailable}
        stored, err := store.PutBlob(r.Context(), doc)
        if err != nil { status = http.StatusInternalServerError; writeError(w, status, err); return }
        location := fmt.Sprintf("/v1/registry/%s/blobs/%s", repo, stored.Digest)
        w.Header().Set("Location", location)
        w.Header().Set("Docker-Content-Digest", stored.Digest)
        status = http.StatusCreated
        writeJSON(w, status, map[string]any{"digest": stored.Digest, "cid": stored.CID, "location": location})
        return
    }

    // Legacy: allocate SSH transfer slot for staging
    manager := s.transfers
    if manager == nil {
        status = http.StatusServiceUnavailable
        writeErrorMessage(w, status, "transfer manager unavailable")
        return
    }
    var payload registryUploadRequest
    if err := json.NewDecoder(r.Body).Decode(&payload); err != nil && err != io.EOF {
        status = http.StatusBadRequest
        writeErrorMessage(w, status, "invalid upload payload")
        return
    }
    nodeID := strings.TrimSpace(payload.NodeID)
    if nodeID == "" { nodeID = "registry" }
    jobID := fmt.Sprintf("registry:%s", repo)
    slot, err := manager.CreateUploadSlot(transfers.KindRegistryBlob, jobID, repo, nodeID, payload.Size)
    if err != nil { status = http.StatusBadRequest; writeError(w, status, err); return }
    location := fmt.Sprintf("/v1/registry/%s/blobs/uploads/%s", repo, slot.ID)
    w.Header().Set("Location", location)
    status = http.StatusAccepted
    writeJSON(w, status, map[string]any{"upload_id": slot.ID, "slot_id": slot.ID, "remote_path": slot.RemotePath, "node_id": slot.NodeID, "location": location})
}

// handleRegistryUploadSession updates upload progress or finalizes a blob upload session.
func (s *controlPlaneServer) handleRegistryUploadSession(w http.ResponseWriter, r *http.Request, repo, sessionID string) {
	status := http.StatusAccepted
	defer func() { recordRegistryRequest("uploads", r.Method, status) }()
	switch r.Method {
	case http.MethodPatch:
		if !s.requireScope(w, r, httpsecurity.ScopeRegistryPush) {
			status = http.StatusForbidden
			return
		}
		var payload registryUploadProgressRequest
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil && err != io.EOF {
			status = http.StatusBadRequest
			writeErrorMessage(w, status, "invalid upload progress payload")
			return
		}
		if payload.Size < 0 {
			status = http.StatusBadRequest
			writeErrorMessage(w, status, "size must be non-negative")
			return
		}
		recordRegistryPayload("uploads", "patch", payload.Size)
		status = http.StatusAccepted
		writeJSON(w, status, map[string]any{"upload_id": sessionID, "uploaded": payload.Size})
	case http.MethodPut:
		if !s.requireScope(w, r, httpsecurity.ScopeRegistryPush) {
			status = http.StatusForbidden
			return
		}
		s.finalizeRegistryUpload(w, r, repo, sessionID, &status)
	default:
		status = http.StatusMethodNotAllowed
		w.Header().Set("Allow", "PATCH, PUT")
		writeErrorMessage(w, status, "method not allowed")
	}
}

// finalizeRegistryUpload completes the blob upload by storing it and updating the transfer slot.
func (s *controlPlaneServer) finalizeRegistryUpload(w http.ResponseWriter, r *http.Request, repo, sessionID string, status *int) {
	store := s.registryStore
	if store == nil {
		*status = http.StatusServiceUnavailable
		writeErrorMessage(w, *status, "registry store unavailable")
		return
	}
	publisher := s.artifactPublisher
	if publisher == nil {
		*status = http.StatusServiceUnavailable
		writeErrorMessage(w, *status, "artifact publisher unavailable")
		return
	}
	manager := s.transfers
	if manager == nil {
		*status = http.StatusServiceUnavailable
		writeErrorMessage(w, *status, "transfer manager unavailable")
		return
	}
	digest := strings.TrimSpace(r.URL.Query().Get("digest"))
	if !isDigest(digest) {
		*status = http.StatusBadRequest
		writeErrorMessage(w, *status, "digest query parameter required")
		return
	}
	var payload registryCommitRequest
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil && err != io.EOF {
		*status = http.StatusBadRequest
		writeErrorMessage(w, *status, "invalid commit payload")
		return
	}
	slot, data, _, err := manager.LoadSlotPayload(sessionID, payload.Size, digest)
	if err != nil {
		*status = http.StatusBadRequest
		writeError(w, *status, err)
		return
	}
	recordRegistryPayload("uploads", "commit", int64(len(data)))
	name := fmt.Sprintf("blob-%s", strings.ReplaceAll(digest, ":", "-"))
	addResp, err := publisher.Add(r.Context(), workflowartifacts.AddRequest{Name: name, Payload: data})
	if err != nil {
		*status = http.StatusBadGateway
		writeError(w, *status, err)
		return
	}
	blobSize := firstNonZero64(payload.Size, addResp.Size, int64(len(data)))
	blobDoc := registry.BlobDocument{
		Repo:      repo,
		Digest:    digest,
		MediaType: strings.TrimSpace(payload.MediaType),
		Size:      blobSize,
		CID:       strings.TrimSpace(addResp.CID),
		Status:    registry.BlobStatusAvailable,
	}
	if blobDoc.MediaType == "" {
		blobDoc.MediaType = "application/octet-stream"
	}
	stored, err := store.PutBlob(r.Context(), blobDoc)
	if err != nil {
		*status = http.StatusInternalServerError
		writeError(w, *status, err)
		return
	}
	_ = os.RemoveAll(filepath.Dir(slot.RemotePath))
	if _, err := manager.Commit(r.Context(), sessionID, blobSize, digest); err != nil {
		*status = http.StatusInternalServerError
		writeError(w, *status, err)
		return
	}
	location := fmt.Sprintf("/v1/registry/%s/blobs/%s", repo, stored.Digest)
	w.Header().Set("Location", location)
	w.Header().Set("Docker-Content-Digest", stored.Digest)
	*status = http.StatusCreated
	writeJSON(w, *status, map[string]any{
		"digest":   stored.Digest,
		"cid":      stored.CID,
		"location": location,
	})
}

// handleRegistryBlob supports direct blob fetch and delete operations once uploaded.
func (s *controlPlaneServer) handleRegistryBlob(w http.ResponseWriter, r *http.Request, repo, digest string) {
	status := http.StatusOK
	defer func() { recordRegistryRequest("blobs", r.Method, status) }()
	store := s.registryStore
	if store == nil {
		status = http.StatusServiceUnavailable
		writeErrorMessage(w, status, "registry store unavailable")
		return
	}
	switch r.Method {
	case http.MethodGet:
		if !s.requireScope(w, r, httpsecurity.ScopeRegistryPull) {
			status = http.StatusForbidden
			return
		}
		blob, err := store.GetBlob(r.Context(), repo, digest)
		if err != nil {
			if errors.Is(err, registry.ErrBlobNotFound) {
				status = http.StatusNotFound
				http.NotFound(w, r)
				return
			}
			status = http.StatusInternalServerError
			writeError(w, status, err)
			return
		}
		publisher := s.artifactPublisher
		if publisher == nil {
			status = http.StatusServiceUnavailable
			writeErrorMessage(w, status, "artifact publisher unavailable")
			return
		}
		result, err := publisher.Fetch(r.Context(), blob.CID)
		if err != nil {
			status = http.StatusBadGateway
			writeError(w, status, err)
			return
		}
		mediaType := strings.TrimSpace(blob.MediaType)
		if mediaType == "" {
			mediaType = "application/octet-stream"
		}
		w.Header().Set("Content-Type", mediaType)
		w.Header().Set("Docker-Content-Digest", blob.Digest)
		w.WriteHeader(http.StatusOK)
		if _, err := w.Write(result.Data); err != nil {
			log.Printf("registry: write blob response: %v", err)
		}
	case http.MethodDelete:
		if !s.requireScope(w, r, httpsecurity.ScopeRegistryPush) {
			status = http.StatusForbidden
			return
		}
		if _, err := store.DeleteBlob(r.Context(), repo, digest); err != nil {
			if errors.Is(err, registry.ErrBlobNotFound) {
				status = http.StatusNotFound
				http.NotFound(w, r)
				return
			}
			status = http.StatusInternalServerError
			writeError(w, status, err)
			return
		}
		status = http.StatusAccepted
		writeJSON(w, status, map[string]any{"digest": digest, "state": "deleted"})
	default:
		status = http.StatusMethodNotAllowed
		w.Header().Set("Allow", "GET, DELETE")
		writeErrorMessage(w, status, "method not allowed")
	}
}
