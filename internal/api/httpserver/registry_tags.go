// registry_tags.go isolates APIs for listing registry tags.
package httpserver

import (
	"net/http"
	"strings"

	httpsecurity "github.com/iw2rmb/ploy/internal/api/httpserver/security"
)

// handleRegistryTags returns the tags associated with a repository.
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
