// registry_manifests.go contains HTTP handlers focused on OCI manifest operations.
package httpserver

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"sort"
	"strings"

	httpsecurity "github.com/iw2rmb/ploy/internal/api/httpserver/security"
	"github.com/iw2rmb/ploy/internal/controlplane/registry"
)

// handleRegistryManifest manages manifest CRUD operations for the registry API.
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

// parseOCIManifest validates the manifest payload and ensures required digests are present.
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

// collectDescriptorDigests returns a sorted, unique slice of descriptor digests.
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
