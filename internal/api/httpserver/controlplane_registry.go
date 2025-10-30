// controlplane_registry.go wires the registry router to the specific handler groups.
package httpserver

import (
    "net/http"
    "strings"
)

// handleRegistry routes registry API calls to manifests, blobs, or tag handlers.
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

// handleRegistryV2 adapts /v2/ requests into the internal /v1/registry/ handler.
// - GET /v2/ returns 200 for Docker version checks.
// - All other /v2/<repo>/manifests|blobs|tags paths are re-routed to /v1/registry/...
func (s *controlPlaneServer) handleRegistryV2(w http.ResponseWriter, r *http.Request) {
    trimmed := strings.TrimPrefix(r.URL.Path, "/v2")
    if trimmed == "" || trimmed == "/" {
        w.WriteHeader(http.StatusOK)
        return
    }
    // Rewrite /v2/<rest> -> /v1/registry/<rest>
    r2 := *r
    u := *r.URL
    u.Path = "/v1/registry" + trimmed
    r2.URL = &u
    s.handleRegistry(w, &r2)
}

// parseRegistryPath extracts the repo, resource, and extra parts from a registry URL path.
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

// isDigest reports whether the provided string looks like a digest (sha* prefix plus value).
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
