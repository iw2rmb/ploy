package httpserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/iw2rmb/ploy/internal/config/gitlab"
	"github.com/iw2rmb/ploy/internal/controlplane/config"
	"net/http"
	"strconv"
	"strings"
	"time"
)

func (s *controlPlaneServer) handleClusterConfig(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.handleConfigGet(w, r)
	case http.MethodPut:
		s.handleConfigPut(w, r)
	default:
		w.Header().Set("Allow", "GET, PUT")
		writeErrorMessage(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (s *controlPlaneServer) handleConfigGet(w http.ResponseWriter, r *http.Request) {
	if !s.ensureEtcd(w) {
		return
	}
	status := http.StatusOK
	method := http.MethodGet
	defer func() {
		recordConfigRequest(method, status)
	}()

	clusterID := strings.TrimSpace(r.URL.Query().Get("cluster_id"))
	if clusterID == "" {
		status = http.StatusBadRequest
		writeErrorMessage(w, status, "cluster_id query parameter required")
		return
	}

	store, err := s.configStore()
	if err != nil {
		status = http.StatusInternalServerError
		writeError(w, status, err)
		return
	}

	doc, revision, err := store.Load(r.Context(), clusterID)
	if err != nil {
		switch {
		case errors.Is(err, config.ErrNotFound):
			status = http.StatusNotFound
			w.Header().Set("Cache-Control", "no-store")
			writeErrorMessage(w, status, "configuration not found")
		default:
			status = http.StatusInternalServerError
			writeError(w, status, err)
		}
		return
	}

	response := map[string]any{
		"cluster_id": clusterID,
		"config":     cloneAnyMap(doc.Data),
		"revision":   revision,
	}
	if strings.TrimSpace(doc.VersionTag) != "" {
		response["version_tag"] = doc.VersionTag
	}
	if !doc.UpdatedAt.IsZero() {
		response["updated_at"] = doc.UpdatedAt.UTC().Format(time.RFC3339Nano)
	}
	if strings.TrimSpace(doc.UpdatedBy) != "" {
		response["updated_by"] = doc.UpdatedBy
	}

	w.Header().Set("ETag", strconv.FormatInt(revision, 10))
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, status, response)
}

func (s *controlPlaneServer) handleConfigPut(w http.ResponseWriter, r *http.Request) {
	if !s.ensureEtcd(w) {
		return
	}
	status := http.StatusOK
	method := http.MethodPut
	defer func() {
		recordConfigRequest(method, status)
	}()

	var req struct {
		ClusterID  string         `json:"cluster_id"`
		Config     map[string]any `json:"config"`
		VersionTag string         `json:"version_tag"`
		UpdatedBy  string         `json:"updated_by"`
	}
	if err := decodeJSON(r, &req); err != nil {
		status = http.StatusBadRequest
		writeError(w, status, err)
		return
	}

	clusterID := strings.TrimSpace(req.ClusterID)
	if clusterID == "" {
		status = http.StatusBadRequest
		writeErrorMessage(w, status, "cluster_id is required")
		return
	}

	rawMatch := strings.TrimSpace(r.Header.Get("If-Match"))
	if rawMatch == "" {
		status = http.StatusPreconditionRequired
		writeErrorMessage(w, status, "If-Match header required")
		return
	}
	expectedRevision, err := parseRevisionHeader(rawMatch)
	if err != nil {
		status = http.StatusBadRequest
		writeError(w, status, err)
		return
	}

	store, err := s.configStore()
	if err != nil {
		status = http.StatusInternalServerError
		writeError(w, status, err)
		return
	}

	doc := config.Document{
		Data:       cloneAnyMap(req.Config),
		VersionTag: strings.TrimSpace(req.VersionTag),
		UpdatedBy:  strings.TrimSpace(req.UpdatedBy),
		UpdatedAt:  time.Now().UTC(),
	}

	saved, revision, err := store.Save(r.Context(), clusterID, expectedRevision, doc)
	if err != nil {
		switch {
		case errors.Is(err, config.ErrConflict):
			status = http.StatusPreconditionFailed
			writeErrorMessage(w, status, "revision mismatch")
		case errors.Is(err, config.ErrNotFound):
			status = http.StatusNotFound
			writeErrorMessage(w, status, "configuration not found")
		default:
			status = http.StatusInternalServerError
			writeError(w, status, err)
		}
		return
	}

	response := map[string]any{
		"cluster_id": clusterID,
		"config":     cloneAnyMap(saved.Data),
		"revision":   revision,
	}
	if strings.TrimSpace(saved.VersionTag) != "" {
		response["version_tag"] = saved.VersionTag
	}
	if !saved.UpdatedAt.IsZero() {
		response["updated_at"] = saved.UpdatedAt.UTC().Format(time.RFC3339Nano)
	}
	if strings.TrimSpace(saved.UpdatedBy) != "" {
		response["updated_by"] = saved.UpdatedBy
	}

	w.Header().Set("ETag", strconv.FormatInt(revision, 10))
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, status, response)
	configUpdatesTotal.WithLabelValues(clusterID).Inc()
}

func (s *controlPlaneServer) handleGitLabConfig(w http.ResponseWriter, r *http.Request) {
	if !s.ensureEtcd(w) {
		return
	}
	switch r.Method {
	case http.MethodGet:
		s.handleGitLabConfigGet(w, r)
	case http.MethodPut:
		s.handleGitLabConfigPut(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *controlPlaneServer) handleGitLabConfigGet(w http.ResponseWriter, r *http.Request) {
	store := gitlab.NewStore(gitlab.NewEtcdKV(s.etcd))
	cfg, revision, err := store.Load(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if revision == 0 {
		writeErrorMessage(w, http.StatusNotFound, "gitlab configuration not set")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"config":   cfg.Sanitize(),
		"revision": revision,
	})
}

func (s *controlPlaneServer) handleGitLabConfigPut(w http.ResponseWriter, r *http.Request) {
	var req gitlabConfigRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if req.Revision < 0 {
		writeErrorMessage(w, http.StatusBadRequest, "revision must be non-negative")
		return
	}

	store := gitlab.NewStore(gitlab.NewEtcdKV(s.etcd))
	_, currentRevision, err := store.Load(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	revisionMismatch := func() {
		writeErrorMessage(w, http.StatusConflict, "revision mismatch")
	}

	if currentRevision == 0 {
		if req.Revision != 0 {
			revisionMismatch()
			return
		}
	} else if req.Revision != currentRevision {
		revisionMismatch()
		return
	}

	normalized, err := gitlab.Normalize(req.Config)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	newRevision, err := store.Save(r.Context(), normalized)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"revision": newRevision,
		"config":   normalized.Sanitize(),
	})
}

type gitlabConfigRequest struct {
	Revision int64         `json:"revision"`
	Config   gitlab.Config `json:"config"`
}

func recordConfigRequest(method string, status int) {
	method = strings.ToUpper(strings.TrimSpace(method))
	if method == "" {
		method = "UNKNOWN"
	}
	configRequestsTotal.WithLabelValues(method, strconv.Itoa(status)).Inc()
}

func recordBeaconRequest(resource, method string, status int) {
	resource = strings.TrimSpace(resource)
	if resource == "" {
		resource = "unknown"
	}
	method = strings.ToUpper(strings.TrimSpace(method))
	if method == "" {
		method = "UNKNOWN"
	}
	beaconRequestsTotal.WithLabelValues(resource, method, strconv.Itoa(status)).Inc()
}

func cloneAnyMap(src map[string]any) map[string]any {
	if len(src) == 0 {
		return map[string]any{}
	}
	out := make(map[string]any, len(src))
	for k, v := range src {
		out[k] = v
	}
	return out
}

func parseRevisionHeader(raw string) (int64, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return 0, errors.New("config: If-Match header required")
	}
	if trimmed == "*" {
		return -1, nil
	}
	if strings.HasPrefix(trimmed, "W/") {
		trimmed = strings.TrimSpace(strings.TrimPrefix(trimmed, "W/"))
	}
	trimmed = strings.Trim(trimmed, `"`)
	if trimmed == "" {
		return 0, errors.New("config: invalid If-Match header")
	}
	revision, err := strconv.ParseInt(trimmed, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("config: parse If-Match revision: %w", err)
	}
	if revision < 0 {
		return 0, errors.New("config: revision must be non-negative")
	}
	return revision, nil
}

func (s *controlPlaneServer) persistBeaconCanonical(ctx context.Context, clusterID string, record map[string]any) error {
	if s.etcd == nil {
		return errors.New("beacon: etcd unavailable")
	}
	data, err := json.Marshal(record)
	if err != nil {
		return fmt.Errorf("beacon: encode canonical record: %w", err)
	}
	if _, err := s.etcd.Put(ctx, beaconCanonicalKey(clusterID), string(data)); err != nil {
		return fmt.Errorf("beacon: persist canonical record: %w", err)
	}
	return nil
}

func beaconCanonicalKey(clusterID string) string {
	return fmt.Sprintf("/ploy/clusters/%s/beacon/canonical", strings.TrimSpace(clusterID))
}
