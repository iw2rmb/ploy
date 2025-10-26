package httpserver

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	httpsecurity "github.com/iw2rmb/ploy/internal/api/httpserver/security"
	"github.com/iw2rmb/ploy/internal/controlplane/registry"
	"github.com/iw2rmb/ploy/internal/controlplane/transfers"
	workflowartifacts "github.com/iw2rmb/ploy/internal/workflow/artifacts"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

func (s *controlPlaneServer) handleRegistry(w http.ResponseWriter, r *http.Request) {
	trimmed := strings.TrimPrefix(r.URL.Path, "/v1/registry/")
	trimmed = strings.Trim(trimmed, "/")
	repo, resource, parts, ok := parseRegistryPath(trimmed)
	if !ok {
		recordRegistryRequest("root", r.Method, http.StatusNotFound)
		http.NotFound(w, r)
		return
	}
	switch resource {
	case "manifests":
		s.handleRegistryManifest(w, r, repo, parts)
	case "blobs":
		s.handleRegistryBlobs(w, r, repo, parts)
	case "tags":
		s.handleRegistryTags(w, r, repo, parts)
	default:
		recordRegistryRequest("unknown", r.Method, http.StatusNotFound)
		http.NotFound(w, r)
	}
}

func (s *controlPlaneServer) handleRegistryManifest(w http.ResponseWriter, r *http.Request, repo string, parts []string) {
	status := http.StatusOK
	defer func() { recordRegistryRequest("manifests", r.Method, status) }()
	if len(parts) == 0 {
		status = http.StatusNotFound
		http.NotFound(w, r)
		return
	}
	reference := strings.Trim(strings.Join(parts, "/"), "/")
	if reference == "" {
		status = http.StatusNotFound
		http.NotFound(w, r)
		return
	}
	store := s.registryStore
	if store == nil {
		status = http.StatusServiceUnavailable
		writeErrorMessage(w, status, "registry store unavailable")
		return
	}
	scopePush := httpsecurity.ScopeRegistryPush
	scopePull := httpsecurity.ScopeRegistryPull
	switch r.Method {
	case http.MethodGet:
		if !s.requireScope(w, r, scopePull) {
			status = http.StatusForbidden
			return
		}
		manifest, err := store.ResolveManifest(r.Context(), repo, reference)
		if err != nil {
			if errors.Is(err, registry.ErrManifestNotFound) || errors.Is(err, registry.ErrTagNotFound) {
				status = http.StatusNotFound
				http.NotFound(w, r)
				return
			}
			status = http.StatusInternalServerError
			writeError(w, status, err)
			return
		}
		mediaType := strings.TrimSpace(manifest.MediaType)
		if mediaType == "" {
			mediaType = "application/vnd.oci.image.manifest.v1+json"
		}
		w.Header().Set("Content-Type", mediaType)
		w.Header().Set("Docker-Content-Digest", manifest.Digest)
		w.WriteHeader(http.StatusOK)
		if _, err := w.Write(manifest.Payload); err != nil {
			log.Printf("registry: write manifest response: %v", err)
		}
	case http.MethodPut:
		if !s.requireScope(w, r, scopePush) {
			status = http.StatusForbidden
			return
		}
		payload, err := io.ReadAll(r.Body)
		if err != nil {
			status = http.StatusInternalServerError
			writeError(w, status, fmt.Errorf("read manifest payload: %w", err))
			return
		}
		if len(payload) == 0 {
			status = http.StatusBadRequest
			writeErrorMessage(w, status, "manifest payload required")
			return
		}
		recordRegistryPayload("manifests", "write", int64(len(payload)))
		parsed, err := parseOCIManifest(payload)
		if err != nil {
			status = http.StatusBadRequest
			writeError(w, status, err)
			return
		}
		digest := sha256.Sum256(payload)
		manifestDigest := "sha256:" + hex.EncodeToString(digest[:])
		layerDigests := collectDescriptorDigests(parsed.Layers)
		manifestDoc := registry.ManifestDocument{
			Repo:         repo,
			Digest:       manifestDigest,
			MediaType:    strings.TrimSpace(parsed.MediaType),
			Size:         int64(len(payload)),
			Payload:      payload,
			ConfigDigest: strings.TrimSpace(parsed.Config.Digest),
			LayerDigests: layerDigests,
		}
		if manifestDoc.MediaType == "" {
			manifestDoc.MediaType = "application/vnd.oci.image.manifest.v1+json"
		}
		tag := ""
		if !isDigest(reference) {
			tag = reference
		}
		stored, err := store.PutManifest(r.Context(), manifestDoc, tag)
		if err != nil {
			if errors.Is(err, registry.ErrBlobNotFound) {
				status = http.StatusBadRequest
				writeErrorMessage(w, status, "manifest references missing blobs")
				return
			}
			status = http.StatusInternalServerError
			writeError(w, status, err)
			return
		}
		location := fmt.Sprintf("/v1/registry/%s/manifests/%s", repo, stored.Digest)
		w.Header().Set("Location", location)
		w.Header().Set("Docker-Content-Digest", stored.Digest)
		status = http.StatusCreated
		writeJSON(w, status, map[string]any{"digest": stored.Digest})
	case http.MethodDelete:
		if !s.requireScope(w, r, scopePush) {
			status = http.StatusForbidden
			return
		}
		if isDigest(reference) {
			if err := store.DeleteManifest(r.Context(), repo, reference); err != nil {
				if errors.Is(err, registry.ErrManifestNotFound) {
					status = http.StatusNotFound
					http.NotFound(w, r)
					return
				}
				status = http.StatusInternalServerError
				writeError(w, status, err)
				return
			}
		} else {
			if err := store.DeleteTag(r.Context(), repo, reference); err != nil {
				if errors.Is(err, registry.ErrTagNotFound) {
					status = http.StatusNotFound
					http.NotFound(w, r)
					return
				}
				status = http.StatusInternalServerError
				writeError(w, status, err)
				return
			}
		}
		status = http.StatusAccepted
		writeJSON(w, status, map[string]any{"reference": reference, "state": "deleted"})
	default:
		status = http.StatusMethodNotAllowed
		w.Header().Set("Allow", "GET, PUT, DELETE")
		writeErrorMessage(w, status, "method not allowed")
	}
}

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
	if nodeID == "" {
		nodeID = "registry"
	}
	jobID := fmt.Sprintf("registry:%s", repo)
	slot, err := manager.CreateUploadSlot(transfers.KindRegistryBlob, jobID, repo, nodeID, payload.Size)
	if err != nil {
		status = http.StatusBadRequest
		writeError(w, status, err)
		return
	}
	location := fmt.Sprintf("/v1/registry/%s/blobs/uploads/%s", repo, slot.ID)
	w.Header().Set("Location", location)
	status = http.StatusAccepted
	writeJSON(w, status, map[string]any{
		"upload_id":   slot.ID,
		"slot_id":     slot.ID,
		"remote_path": slot.RemotePath,
		"node_id":     slot.NodeID,
		"location":    location,
	})
}

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

func (s *controlPlaneServer) handleRegistryTags(w http.ResponseWriter, r *http.Request, repo string, parts []string) {
	status := http.StatusOK
	defer func() { recordRegistryRequest("tags", r.Method, status) }()
	if len(parts) == 0 || strings.TrimSpace(parts[0]) != "list" {
		status = http.StatusNotFound
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodGet {
		status = http.StatusMethodNotAllowed
		w.Header().Set("Allow", http.MethodGet)
		writeErrorMessage(w, status, "method not allowed")
		return
	}
	if !s.requireScope(w, r, httpsecurity.ScopeRegistryPull) {
		status = http.StatusForbidden
		return
	}
	store := s.registryStore
	if store == nil {
		status = http.StatusServiceUnavailable
		writeErrorMessage(w, status, "registry store unavailable")
		return
	}
	tags, err := store.ListTags(r.Context(), repo)
	if err != nil {
		status = http.StatusInternalServerError
		writeError(w, status, err)
		return
	}
	names := make([]string, 0, len(tags))
	for _, tag := range tags {
		names = append(names, tag.Name)
	}
	status = http.StatusOK
	writeJSON(w, status, map[string]any{
		"name": repo,
		"tags": names,
	})
}

func recordRegistryRequest(resource, method string, status int) {
	resource = strings.TrimSpace(resource)
	if resource == "" {
		resource = "unknown"
	}
	method = strings.ToUpper(strings.TrimSpace(method))
	if method == "" {
		method = "UNKNOWN"
	}
	registryRequestsTotal.WithLabelValues(resource, method, strconv.Itoa(status)).Inc()
}

func recordRegistryPayload(resource, operation string, bytesCopied int64) {
	if bytesCopied <= 0 {
		return
	}
	resource = strings.TrimSpace(resource)
	if resource == "" {
		resource = "unknown"
	}
	operation = strings.TrimSpace(operation)
	if operation == "" {
		operation = "unknown"
	}
	registryPayloadBytes.WithLabelValues(resource, operation).Add(float64(bytesCopied))
}

func parseRegistryPath(path string) (string, string, []string, bool) {
	trimmed := strings.Trim(path, "/")
	if trimmed == "" {
		return "", "", nil, false
	}
	parts := strings.Split(trimmed, "/")
	for i := 1; i < len(parts); i++ {
		section := strings.TrimSpace(parts[i])
		switch section {
		case "manifests", "blobs", "tags":
			repo := strings.Trim(strings.Join(parts[:i], "/"), "/")
			if repo == "" {
				return "", "", nil, false
			}
			return repo, section, parts[i+1:], true
		}
	}
	return "", "", nil, false
}

func isDigest(value string) bool {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return false
	}
	parts := strings.SplitN(trimmed, ":", 2)
	if len(parts) != 2 {
		return false
	}
	if parts[1] == "" {
		return false
	}
	algorithm := strings.ToLower(strings.TrimSpace(parts[0]))
	return strings.HasPrefix(algorithm, "sha")
}

func parseOCIManifest(payload []byte) (ociManifest, error) {
	var manifest ociManifest
	if err := json.Unmarshal(payload, &manifest); err != nil {
		return manifest, fmt.Errorf("decode manifest: %w", err)
	}
	if strings.TrimSpace(manifest.Config.Digest) == "" {
		return manifest, errors.New("manifest config digest required")
	}
	for _, layer := range manifest.Layers {
		if strings.TrimSpace(layer.Digest) == "" {
			return manifest, errors.New("manifest layer digest required")
		}
	}
	return manifest, nil
}

func collectDescriptorDigests(descriptors []ociDescriptor) []string {
	set := make(map[string]struct{})
	for _, desc := range descriptors {
		trimmed := strings.TrimSpace(desc.Digest)
		if trimmed != "" {
			set[trimmed] = struct{}{}
		}
	}
	result := make([]string, 0, len(set))
	for digest := range set {
		result = append(result, digest)
	}
	sort.Strings(result)
	return result
}

type registryUploadRequest struct {
	MediaType string `json:"media_type"`
	Size      int64  `json:"size"`
	NodeID    string `json:"node_id"`
}

type registryUploadProgressRequest struct {
	Size int64 `json:"size"`
}

type registryCommitRequest struct {
	MediaType string `json:"media_type"`
	Size      int64  `json:"size"`
}

type ociManifest struct {
	SchemaVersion int             `json:"schemaVersion"`
	MediaType     string          `json:"mediaType"`
	Config        ociDescriptor   `json:"config"`
	Layers        []ociDescriptor `json:"layers"`
}

type ociDescriptor struct {
	MediaType string `json:"mediaType"`
	Digest    string `json:"digest"`
	Size      int64  `json:"size"`
}
