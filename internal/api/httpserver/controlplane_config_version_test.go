package httpserver_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/iw2rmb/ploy/internal/api/httpserver"
	"github.com/iw2rmb/ploy/internal/api/httpserver/security"
	"github.com/iw2rmb/ploy/internal/version"
)

func TestConfigGetRequiresAdminScope(t *testing.T) {
	t.Parallel()

	principal := newTestPrincipal([]string{"mods"})
	handler := newTestControlPlaneHandler(t, httpserver.ControlPlaneOptions{
		Auth: security.NewManager(&testTokenVerifier{principal: principal}),
	})

	req := newMTLSRequest(t, http.MethodGet, "/v1/config?cluster_id=cluster-alpha", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 without admin scope, got %d", rec.Code)
	}
}

func TestConfigNotFound(t *testing.T) {
	t.Parallel()

	etcd, client := startTestEtcd(t)
	defer etcd.Close()
	defer func() { _ = client.Close() }()

	principal := newTestPrincipal([]string{security.ScopeAdmin})
	handler := newTestControlPlaneHandler(t, httpserver.ControlPlaneOptions{
		Etcd: client,
		Auth: security.NewManager(&testTokenVerifier{principal: principal}),
	})

	req := newMTLSRequest(t, http.MethodGet, "/v1/config?cluster_id=cluster-alpha", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for missing config, got %d", rec.Code)
	}
	if cache := rec.Header().Get("Cache-Control"); cache != "no-store" {
		t.Fatalf("expected Cache-Control no-store, got %q", cache)
	}
}

func TestConfigPutRoundTrip(t *testing.T) {
	t.Parallel()

	etcd, client := startTestEtcd(t)
	defer etcd.Close()
	defer func() { _ = client.Close() }()

	principal := newTestPrincipal([]string{security.ScopeAdmin})
	handler := newTestControlPlaneHandler(t, httpserver.ControlPlaneOptions{
		Etcd: client,
		Auth: security.NewManager(&testTokenVerifier{principal: principal}),
	})

	createBody := map[string]any{
		"cluster_id":  "cluster-alpha",
		"config":      map[string]any{"features": map[string]any{"mods": true}},
		"version_tag": "2025.10.24",
		"updated_by":  "ops-team",
	}

	bodyBytes, err := json.Marshal(createBody)
	if err != nil {
		t.Fatalf("marshal create body: %v", err)
	}

	putReq := newMTLSRequest(t, http.MethodPut, "/v1/config", bytes.NewReader(bodyBytes))
	putReq.Header.Set("Content-Type", "application/json")
	putRec := httptest.NewRecorder()

	handler.ServeHTTP(putRec, putReq)

	if putRec.Code != http.StatusPreconditionRequired {
		t.Fatalf("expected 428 without If-Match, got %d", putRec.Code)
	}

	putReq = newMTLSRequest(t, http.MethodPut, "/v1/config", bytes.NewReader(bodyBytes))
	putReq.Header.Set("Content-Type", "application/json")
	putReq.Header.Set("If-Match", "0")
	putRec = httptest.NewRecorder()

	handler.ServeHTTP(putRec, putReq)

	if putRec.Code != http.StatusOK {
		t.Fatalf("expected 200 creating config, got %d", putRec.Code)
	}
	if cache := putRec.Header().Get("Cache-Control"); cache != "no-store" {
		t.Fatalf("expected Cache-Control no-store on PUT response, got %q", cache)
	}
	etag := strings.TrimSpace(putRec.Header().Get("ETag"))
	if etag == "" {
		t.Fatalf("expected ETag header on config response")
	}

	var putBody map[string]any
	if err := json.NewDecoder(putRec.Body).Decode(&putBody); err != nil {
		t.Fatalf("decode put response: %v", err)
	}
	revision, _ := putBody["revision"].(float64)
	if revision <= 0 {
		t.Fatalf("expected positive revision, got %v", putBody["revision"])
	}
	if gotTag, _ := putBody["version_tag"].(string); gotTag != "2025.10.24" {
		t.Fatalf("unexpected version_tag %q", gotTag)
	}
	configMap, ok := putBody["config"].(map[string]any)
	if !ok {
		t.Fatalf("expected config block in put response")
	}
	if features, ok := configMap["features"].(map[string]any); !ok || len(features) != 1 {
		t.Fatalf("expected features map in config response, got %#v", configMap["features"])
	}

	getReq := newMTLSRequest(t, http.MethodGet, "/v1/config?cluster_id=cluster-alpha", nil)
	getRec := httptest.NewRecorder()
	handler.ServeHTTP(getRec, getReq)

	if getRec.Code != http.StatusOK {
		t.Fatalf("expected 200 fetching config, got %d", getRec.Code)
	}
	if cache := getRec.Header().Get("Cache-Control"); cache != "no-store" {
		t.Fatalf("expected Cache-Control no-store on GET, got %q", cache)
	}
	if got := strings.TrimSpace(getRec.Header().Get("ETag")); got != etag {
		t.Fatalf("expected GET ETag %q, got %q", etag, got)
	}

	var getBody map[string]any
	if err := json.NewDecoder(getRec.Body).Decode(&getBody); err != nil {
		t.Fatalf("decode get response: %v", err)
	}
	if gotCluster, _ := getBody["cluster_id"].(string); gotCluster != "cluster-alpha" {
		t.Fatalf("unexpected cluster_id %q", gotCluster)
	}
	_ = getBody["revision"]

	staleReq := newMTLSRequest(t, http.MethodPut, "/v1/config", bytes.NewReader(bodyBytes))
	staleReq.Header.Set("Content-Type", "application/json")
	staleReq.Header.Set("If-Match", "0")
	staleRec := httptest.NewRecorder()
	handler.ServeHTTP(staleRec, staleReq)
	if staleRec.Code != http.StatusPreconditionFailed {
		t.Fatalf("expected 412 on stale revision, got %d", staleRec.Code)
	}

	updateBody := map[string]any{
		"cluster_id":  "cluster-alpha",
		"config":      map[string]any{"features": map[string]any{"mods": false}},
		"version_tag": "2025.10.25",
		"updated_by":  "ops-team",
	}
	updateBytes, err := json.Marshal(updateBody)
	if err != nil {
		t.Fatalf("marshal update body: %v", err)
	}
	updateReq := newMTLSRequest(t, http.MethodPut, "/v1/config", bytes.NewReader(updateBytes))
	updateReq.Header.Set("Content-Type", "application/json")
	updateReq.Header.Set("If-Match", etag)
	updateRec := httptest.NewRecorder()
	handler.ServeHTTP(updateRec, updateReq)
	if updateRec.Code != http.StatusOK {
		t.Fatalf("expected 200 updating config, got %d", updateRec.Code)
	}
	if cache := updateRec.Header().Get("Cache-Control"); cache != "no-store" {
		t.Fatalf("expected Cache-Control no-store on update, got %q", cache)
	}
	newETag := strings.TrimSpace(updateRec.Header().Get("ETag"))
	if newETag == "" || newETag == etag {
		t.Fatalf("expected new etag after update, old %q new %q", etag, newETag)
	}

	var updateResp map[string]any
	if err := json.NewDecoder(updateRec.Body).Decode(&updateResp); err != nil {
		t.Fatalf("decode update response: %v", err)
	}
	if gotTag, _ := updateResp["version_tag"].(string); gotTag != "2025.10.25" {
		t.Fatalf("unexpected updated version_tag %q", gotTag)
	}
	updatedConfig, ok := updateResp["config"].(map[string]any)
	if !ok {
		t.Fatalf("expected config block in update response")
	}
	features, ok := updatedConfig["features"].(map[string]any)
	if !ok {
		t.Fatalf("expected features map after update")
	}
	if enabled, _ := features["mods"].(bool); enabled {
		t.Fatalf("expected mods feature disabled after update")
	}
}

func TestVersionEndpointReturnsBuildMetadata(t *testing.T) {
	t.Parallel()

	origVersion := version.Version
	origCommit := version.Commit
	origBuilt := version.BuiltAt
	version.Version = "2.3.4"
	version.Commit = "abcdef1"
	version.BuiltAt = "2025-10-24T10:15:00Z"
	t.Cleanup(func() {
		version.Version = origVersion
		version.Commit = origCommit
		version.BuiltAt = origBuilt
	})

	handler := newTestControlPlaneHandler(t, httpserver.ControlPlaneOptions{})
	req := newMTLSRequest(t, http.MethodGet, "/v1/version", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for version endpoint, got %d", rec.Code)
	}
	if cache := rec.Header().Get("Cache-Control"); !strings.Contains(cache, "max-age") {
		t.Fatalf("expected Cache-Control with max-age, got %q", cache)
	}

	var body map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode version response: %v", err)
	}
	if got, _ := body["version"].(string); got != "2.3.4" {
		t.Fatalf("unexpected version %q", got)
	}
	if got, _ := body["commit"].(string); got != "abcdef1" {
		t.Fatalf("unexpected commit %q", got)
	}
	if got, _ := body["built_at"].(string); got != "2025-10-24T10:15:00Z" {
		t.Fatalf("unexpected built_at %q", got)
	}
}
