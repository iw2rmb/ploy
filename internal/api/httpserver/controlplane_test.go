package httpserver_test

import (
	"bufio"
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"
	"go.etcd.io/etcd/server/v3/embed"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/iw2rmb/ploy/internal/api/httpserver"
	"github.com/iw2rmb/ploy/internal/api/httpserver/security"
	"github.com/iw2rmb/ploy/internal/config/gitlab"
	controlplanemods "github.com/iw2rmb/ploy/internal/controlplane/mods"
	"github.com/iw2rmb/ploy/internal/controlplane/registry"
	"github.com/iw2rmb/ploy/internal/controlplane/scheduler"
	"github.com/iw2rmb/ploy/internal/deploy"
	"github.com/iw2rmb/ploy/internal/metrics"
	"github.com/iw2rmb/ploy/internal/node/logstream"
	"github.com/iw2rmb/ploy/internal/version"
)

func TestServerJobLifecycle(t *testing.T) {
	t.Parallel()

	etcd, client := startTestEtcd(t)
	defer etcd.Close()
	defer func() {
		_ = client.Close()
	}()

	sched, err := scheduler.New(client, scheduler.Options{LeaseTTL: 3 * time.Second})
	if err != nil {
		t.Fatalf("new scheduler: %v", err)
	}
	defer func() {
		_ = sched.Close()
	}()

	server := httptest.NewServer(httpserver.NewControlPlaneHandler(httpserver.ControlPlaneOptions{
		Scheduler: sched,
		Etcd:      client,
	}))
	defer server.Close()

	submit := map[string]any{
		"ticket":       "mod-900",
		"step_id":      "plan",
		"priority":     "default",
		"max_attempts": 2,
	}
	job := postJSON(t, server.URL+"/v1/jobs", submit)

	if job["state"].(string) != "queued" {
		t.Fatalf("expected queued state, got %v", job["state"])
	}

	claim := postJSON(t, server.URL+"/v1/jobs/claim", map[string]any{"node_id": "node-http"})
	if claim["status"].(string) != "claimed" {
		t.Fatalf("claim status: %v", claim)
	}
	claimedJob := claim["job"].(map[string]any)
	jobID := claimedJob["id"].(string)

	postJSON(t, server.URL+"/v1/jobs/"+jobID+"/heartbeat", map[string]any{
		"ticket":  "mod-900",
		"node_id": "node-http",
	})

	complete := postJSON(t, server.URL+"/v1/jobs/"+jobID+"/complete", map[string]any{
		"ticket":  "mod-900",
		"node_id": "node-http",
		"state":   "succeeded",
	})
	if complete["state"].(string) != "succeeded" {
		t.Fatalf("completion state: %v", complete["state"])
	}

	listURL := fmt.Sprintf("%s/v1/jobs?ticket=%s", server.URL, url.QueryEscape("mod-900"))
	resp := getJSON(t, listURL)
	jobs := resp["jobs"].([]any)
	if len(jobs) != 1 {
		t.Fatalf("expected one job, got %d", len(jobs))
	}
}

func TestJobRetention(t *testing.T) {
	t.Parallel()

	etcd, client := startTestEtcd(t)
	defer etcd.Close()
	defer func() {
		_ = client.Close()
	}()

	completedAt := time.Date(2025, 10, 22, 17, 0, 0, 0, time.UTC)
	sched, err := scheduler.New(client, scheduler.Options{
		LeaseTTL: 3 * time.Second,
		Now:      func() time.Time { return completedAt },
	})
	if err != nil {
		t.Fatalf("new scheduler: %v", err)
	}
	defer func() {
		_ = sched.Close()
	}()

	server := httptest.NewServer(httpserver.NewControlPlaneHandler(httpserver.ControlPlaneOptions{
		Scheduler: sched,
		Etcd:      client,
	}))
	defer server.Close()

	submit := map[string]any{
		"ticket":       "mod-retention",
		"step_id":      "logs",
		"priority":     "default",
		"max_attempts": 1,
	}
	job := postJSON(t, server.URL+"/v1/jobs", submit)
	jobID := job["id"].(string)

	claim := postJSON(t, server.URL+"/v1/jobs/claim", map[string]any{"node_id": "node-retention"})
	if claim["status"].(string) != "claimed" {
		t.Fatalf("claim status: %v", claim)
	}

	complete := postJSON(t, server.URL+"/v1/jobs/"+jobID+"/complete", map[string]any{
		"ticket":     "mod-retention",
		"node_id":    "node-retention",
		"state":      "failed",
		"inspection": true,
		"bundles": map[string]any{
			"logs": map[string]any{
				"cid":      "bafy-observed",
				"digest":   "sha256:bundle",
				"size":     8192,
				"retained": true,
				"ttl":      "96h",
			},
		},
	})
	if complete["state"].(string) != "inspection_ready" {
		t.Fatalf("expected inspection_ready state, got %v", complete["state"])
	}

	getURL := fmt.Sprintf("%s/v1/jobs/%s?ticket=%s", server.URL, jobID, url.QueryEscape("mod-retention"))
	jobResp := getJSON(t, getURL)
	retention, ok := jobResp["retention"].(map[string]any)
	if !ok {
		t.Fatalf("expected retention block in job response")
	}
	wantExpires := completedAt.Add(96 * time.Hour).UTC().Format(time.RFC3339Nano)
	if retained, _ := retention["retained"].(bool); !retained {
		t.Fatalf("expected retained flag in job response")
	}
	if bundle, _ := retention["bundle"].(string); bundle != "logs" {
		t.Fatalf("unexpected retention bundle: %v", bundle)
	}
	if cid, _ := retention["bundle_cid"].(string); cid != "bafy-observed" {
		t.Fatalf("unexpected retention cid: %v", cid)
	}
	if ttl, _ := retention["ttl"].(string); ttl != "96h" {
		t.Fatalf("unexpected retention ttl: %v", ttl)
	}
	if expires, _ := retention["expires_at"].(string); expires != wantExpires {
		t.Fatalf("unexpected retention expires_at: %v want %s", expires, wantExpires)
	}
	if inspect, _ := retention["inspection"].(bool); !inspect {
		t.Fatalf("expected inspection hint true")
	}

	bundles, ok := jobResp["bundles"].(map[string]any)
	if !ok {
		t.Fatalf("expected bundles map in job response")
	}
	logBundle, ok := bundles["logs"].(map[string]any)
	if !ok {
		t.Fatalf("expected logs bundle in response")
	}
	if expires, _ := logBundle["expires_at"].(string); expires != wantExpires {
		t.Fatalf("unexpected bundle expires_at: %v want %s", expires, wantExpires)
	}

	listURL := fmt.Sprintf("%s/v1/jobs?ticket=%s", server.URL, url.QueryEscape("mod-retention"))
	listResp := getJSON(t, listURL)
	items, ok := listResp["jobs"].([]any)
	if !ok || len(items) != 1 {
		t.Fatalf("expected job listing")
	}
	item := items[0].(map[string]any)
	retList, ok := item["retention"].(map[string]any)
	if !ok {
		t.Fatalf("expected retention in listing entry")
	}
	if expires, _ := retList["expires_at"].(string); expires != wantExpires {
		t.Fatalf("unexpected list retention expires_at: %v want %s", expires, wantExpires)
	}
}

func TestArtifactsListRequiresReadScope(t *testing.T) {
	t.Parallel()

	principal := newTestPrincipal([]string{"artifact.write"})
	handler := httpserver.NewControlPlaneHandler(httpserver.ControlPlaneOptions{
		Auth: security.NewManager(&testTokenVerifier{principal: principal}),
	})

	req := newMTLSRequest(t, http.MethodGet, "/v1/artifacts", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 when read scope missing, got %d", rec.Code)
	}
}

func TestArtifactsListEmptyResponse(t *testing.T) {
	t.Parallel()

	principal := newTestPrincipal([]string{"artifact.read"})
	handler := httpserver.NewControlPlaneHandler(httpserver.ControlPlaneOptions{
		Auth: security.NewManager(&testTokenVerifier{principal: principal}),
	})

	req := newMTLSRequest(t, http.MethodGet, "/v1/artifacts", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var body map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	items, ok := body["artifacts"].([]any)
	if !ok {
		t.Fatalf("expected artifacts array, got %#v", body["artifacts"])
	}
	if len(items) != 0 {
		t.Fatalf("expected empty artifacts list, got %d", len(items))
	}
}

func TestArtifactsUploadNotImplemented(t *testing.T) {
	t.Parallel()

	principal := newTestPrincipal([]string{"artifact.write"})
	handler := httpserver.NewControlPlaneHandler(httpserver.ControlPlaneOptions{
		Auth: security.NewManager(&testTokenVerifier{principal: principal}),
	})

	req := newMTLSRequest(t, http.MethodPost, "/v1/artifacts/upload", strings.NewReader("payload"))
	req.Header.Set("Content-Type", "application/octet-stream")

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("expected 501, got %d", rec.Code)
	}

	var body map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if code, _ := body["error_code"].(string); code != "ARTIFACT_UPLOAD_UNIMPLEMENTED" {
		t.Fatalf("unexpected error code: %#v", body["error_code"])
	}
}

func TestArtifactsDeleteRequiresWriteScope(t *testing.T) {
	t.Parallel()

	principal := newTestPrincipal([]string{"artifact.read"})
	handler := httpserver.NewControlPlaneHandler(httpserver.ControlPlaneOptions{
		Auth: security.NewManager(&testTokenVerifier{principal: principal}),
	})

	req := newMTLSRequest(t, http.MethodDelete, "/v1/artifacts/bafytest", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 without write scope, got %d", rec.Code)
	}
}

func TestConfigGetRequiresAdminScope(t *testing.T) {
	t.Parallel()

	principal := newTestPrincipal([]string{"mods"})
	handler := httpserver.NewControlPlaneHandler(httpserver.ControlPlaneOptions{
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
	handler := httpserver.NewControlPlaneHandler(httpserver.ControlPlaneOptions{
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
	handler := httpserver.NewControlPlaneHandler(httpserver.ControlPlaneOptions{
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
	revision = getBody["revision"].(float64)

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

	handler := httpserver.NewControlPlaneHandler(httpserver.ControlPlaneOptions{})
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

func TestRegistryManifestGetNotImplemented(t *testing.T) {
	t.Parallel()

	principal := newTestPrincipal([]string{"registry.pull"})
	handler := httpserver.NewControlPlaneHandler(httpserver.ControlPlaneOptions{
		Auth: security.NewManager(&testTokenVerifier{principal: principal}),
	})

	req := newMTLSRequest(t, http.MethodGet, "/v1/registry/acme/manifests/latest", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("expected 501 for registry manifest GET, got %d", rec.Code)
	}

	var body map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if code, _ := body["error_code"].(string); code != "REGISTRY_MANIFEST_UNIMPLEMENTED" {
		t.Fatalf("unexpected error code: %#v", body["error_code"])
	}
}

func TestModsHTTPSubmitStatusLifecycle(t *testing.T) {
	t.Parallel()

	fixture := newModsServerFixture(t)
	ticketID := "mod-http-1"

	submit := postJSON(t, fixture.server.URL+"/v1/mods", map[string]any{
		"ticket_id": ticketID,
		"submitter": "cli",
		"stages": []any{
			map[string]any{"id": "plan"},
		},
	})
	ticket, ok := submit["ticket"].(map[string]any)
	if !ok {
		t.Fatalf("expected ticket in submit response")
	}
	if got, _ := ticket["ticket_id"].(string); got != ticketID {
		t.Fatalf("unexpected ticket id %q", got)
	}

	statusURL := fmt.Sprintf("%s/v1/mods/%s", fixture.server.URL, ticketID)
	status := getJSON(t, statusURL)
	statusTicket, ok := status["ticket"].(map[string]any)
	if !ok {
		t.Fatalf("expected ticket block in status response")
	}
	stages, ok := statusTicket["stages"].(map[string]any)
	if !ok || len(stages) != 1 {
		t.Fatalf("expected stages map in status response, got %+v", statusTicket["stages"])
	}
	stage, ok := stages["plan"].(map[string]any)
	if !ok {
		t.Fatalf("expected plan stage in status response")
	}
	if state, _ := stage["state"].(string); state != "queued" {
		t.Fatalf("expected stage queued, got %q", state)
	}
	if jobID, _ := stage["current_job_id"].(string); strings.TrimSpace(jobID) == "" {
		t.Fatalf("expected current job id on queued stage")
	}

	cancelStatus, _ := postJSONStatus(t, fmt.Sprintf("%s/v1/mods/%s/cancel", fixture.server.URL, ticketID), map[string]any{})
	if cancelStatus != http.StatusAccepted {
		t.Fatalf("expected cancel status 202, got %d", cancelStatus)
	}

	cancelled := getJSON(t, statusURL)
	cancelStages := cancelled["ticket"].(map[string]any)["stages"].(map[string]any)
	cancelStage := cancelStages["plan"].(map[string]any)
	if state, _ := cancelStage["state"].(string); state != "cancelled" {
		t.Fatalf("expected stage cancelled, got %q", state)
	}

	resume := postJSON(t, fmt.Sprintf("%s/v1/mods/%s/resume", fixture.server.URL, ticketID), map[string]any{})
	resumeTicket := resume["ticket"].(map[string]any)
	resumeStages := resumeTicket["stages"].(map[string]any)
	resumeStage := resumeStages["plan"].(map[string]any)
	if state, _ := resumeStage["state"].(string); state != "queued" {
		t.Fatalf("expected stage queued after resume, got %q", state)
	}
	if jobID, _ := resumeStage["current_job_id"].(string); strings.TrimSpace(jobID) == "" {
		t.Fatalf("expected new job id after resume")
	}

	legacy := getJSON(t, fmt.Sprintf("%s/v1/mods/tickets/%s", fixture.server.URL, ticketID))
	legacyTicket := legacy["ticket"].(map[string]any)
	if got, _ := legacyTicket["ticket_id"].(string); got != ticketID {
		t.Fatalf("legacy endpoint returned wrong ticket id %q", got)
	}
}

func TestModsLogsEndpoints(t *testing.T) {
	t.Parallel()

	fixture := newModsServerFixture(t)
	ticketID := "mod-logs-1"

	fixture.streams.Ensure(ticketID)
	if err := fixture.streams.PublishLog(context.Background(), ticketID, logstream.LogRecord{
		Timestamp: "2025-10-24T10:00:00Z",
		Stream:    "stdout",
		Line:      "starting stage",
	}); err != nil {
		t.Fatalf("publish log: %v", err)
	}

	logs := getJSON(t, fmt.Sprintf("%s/v1/mods/%s/logs", fixture.server.URL, ticketID))
	events, ok := logs["events"].([]any)
	if !ok || len(events) == 0 {
		t.Fatalf("expected snapshot events in logs response")
	}
	first, ok := events[0].(map[string]any)
	if !ok {
		t.Fatalf("expected event map in logs snapshot")
	}
	if evtType, _ := first["type"].(string); evtType != "log" {
		t.Fatalf("expected first snapshot event log, got %q", evtType)
	}
	payload, ok := first["data"].(map[string]any)
	if !ok {
		t.Fatalf("expected event payload map")
	}
	if line, _ := payload["line"].(string); line != "starting stage" {
		t.Fatalf("unexpected log line %q", line)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	eventCh := make(chan sseEvent, 4)
	errCh := make(chan error, 1)
	go func() {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("%s/v1/mods/%s/logs/stream", fixture.server.URL, ticketID), nil)
		if err != nil {
			errCh <- err
			return
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			errCh <- err
			return
		}
		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			errCh <- fmt.Errorf("http %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
			return
		}
		defer func() {
			_ = resp.Body.Close()
		}()
		reader := bufio.NewReader(resp.Body)
		for {
			evt, err := readSSEEvent(reader)
			if err != nil {
				errCh <- err
				return
			}
			eventCh <- evt
			if evt.Type == "done" {
				return
			}
		}
	}()

	time.Sleep(50 * time.Millisecond)

	if err := fixture.streams.PublishLog(context.Background(), ticketID, logstream.LogRecord{
		Timestamp: "2025-10-24T10:00:01Z",
		Stream:    "stderr",
		Line:      "warning: retry",
	}); err != nil {
		t.Fatalf("publish follow-up log: %v", err)
	}
	if err := fixture.streams.PublishRetention(context.Background(), ticketID, logstream.RetentionHint{
		Retained: true,
		TTL:      "72h",
		Expires:  "2025-10-27T10:00:00Z",
		Bundle:   "bafy-log-bundle",
	}); err != nil {
		t.Fatalf("publish retention: %v", err)
	}
	if err := fixture.streams.PublishStatus(context.Background(), ticketID, logstream.Status{Status: "completed"}); err != nil {
		t.Fatalf("publish status: %v", err)
	}

	wantOrder := []string{"log", "log", "retention", "done"}
	for i := 0; i < len(wantOrder); i++ {
		select {
		case evt := <-eventCh:
			if evt.Type != wantOrder[i] {
				t.Fatalf("expected event %s at position %d, got %s", wantOrder[i], i, evt.Type)
			}
		case err := <-errCh:
			t.Fatalf("stream error: %v", err)
		case <-ctx.Done():
			t.Fatalf("timed out waiting for event %d", i)
		}
	}
}

func TestModsEventsStream(t *testing.T) {
	t.Parallel()

	fixture := newModsServerFixture(t)
	ticketID := "mod-events-1"

	postJSON(t, fixture.server.URL+"/v1/mods", map[string]any{
		"ticket_id": ticketID,
		"stages": []any{
			map[string]any{"id": "plan"},
		},
	})

	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()

	eventCh := make(chan sseEvent, 6)
	errCh := make(chan error, 1)

	go func() {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("%s/v1/mods/%s/events", fixture.server.URL, ticketID), nil)
		if err != nil {
			errCh <- err
			return
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			errCh <- err
			return
		}
		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			errCh <- fmt.Errorf("http %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
			return
		}
		defer func() {
			_ = resp.Body.Close()
		}()
		reader := bufio.NewReader(resp.Body)
		for {
			evt, err := readSSEEvent(reader)
			if err != nil {
				errCh <- err
				return
			}
			eventCh <- evt
		}
	}()

	var initial sseEvent
	select {
	case evt := <-eventCh:
		initial = evt
	case err := <-errCh:
		t.Fatalf("initial stream error: %v", err)
	case <-ctx.Done():
		t.Fatalf("timeout waiting for initial mods event")
	}
	if initial.Type != "ticket" {
		t.Fatalf("expected initial ticket event, got %s", initial.Type)
	}
	var initialPayload map[string]any
	if err := json.Unmarshal([]byte(initial.Data), &initialPayload); err != nil {
		t.Fatalf("decode initial payload: %v", err)
	}
	if state, _ := initialPayload["state"].(string); state == "" {
		t.Fatalf("expected ticket state in initial payload")
	}

	cancelStatus, _ := postJSONStatus(t, fmt.Sprintf("%s/v1/mods/%s/cancel", fixture.server.URL, ticketID), map[string]any{})
	if cancelStatus != http.StatusAccepted {
		t.Fatalf("expected cancel status 202, got %d", cancelStatus)
	}

	cancelled := false
	timeout := time.After(4 * time.Second)
	for !cancelled {
		select {
		case evt := <-eventCh:
			if evt.Type != "ticket" {
				continue
			}
			var payload map[string]any
			if err := json.Unmarshal([]byte(evt.Data), &payload); err != nil {
				t.Fatalf("decode ticket payload: %v", err)
			}
			if state, _ := payload["state"].(string); state == "cancelled" {
				cancelled = true
			}
		case err := <-errCh:
			t.Fatalf("stream error: %v", err)
		case <-timeout:
			t.Fatalf("timed out waiting for cancelled ticket event")
		}
	}
}

func TestJobEventsStream(t *testing.T) {
	t.Parallel()

	fixture := newModsServerFixture(t)
	ticket := "mod-job-events"

	jobResp := postJSON(t, fixture.server.URL+"/v1/jobs", map[string]any{
		"ticket":       ticket,
		"step_id":      "plan",
		"priority":     "default",
		"max_attempts": 1,
	})
	jobID, _ := jobResp["id"].(string)
	if strings.TrimSpace(jobID) == "" {
		t.Fatalf("expected job id in response")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	eventCh := make(chan sseEvent, 8)
	errCh := make(chan error, 1)
	go func() {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("%s/v1/jobs/%s/events", fixture.server.URL, jobID), nil)
		if err != nil {
			errCh <- err
			return
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			errCh <- err
			return
		}
		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			errCh <- fmt.Errorf("http %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
			return
		}
		defer func() { _ = resp.Body.Close() }()
		reader := bufio.NewReader(resp.Body)
		for {
			evt, err := readSSEEvent(reader)
			if err != nil {
				errCh <- err
				return
			}
			eventCh <- evt
		}
	}()

	waitForState := func(want string) {
		timeout := time.After(4 * time.Second)
		for {
			select {
			case evt := <-eventCh:
				if evt.Type != "job" {
					continue
				}
				var payload map[string]any
				if err := json.Unmarshal([]byte(evt.Data), &payload); err != nil {
					t.Fatalf("decode job event %s: %v", want, err)
				}
				if state, _ := payload["state"].(string); state == want {
					return
				}
			case err := <-errCh:
				t.Fatalf("job events stream error: %v", err)
			case <-timeout:
				t.Fatalf("timed out waiting for job state %s", want)
			}
		}
	}

	waitForState("queued")

	postJSON(t, fixture.server.URL+"/v1/jobs/claim", map[string]any{"node_id": "node-events"})
	waitForState("running")

	postJSON(t, fmt.Sprintf("%s/v1/jobs/%s/heartbeat", fixture.server.URL, jobID), map[string]any{
		"ticket":  ticket,
		"node_id": "node-events",
	})

	postJSON(t, fmt.Sprintf("%s/v1/jobs/%s/complete", fixture.server.URL, jobID), map[string]any{
		"ticket":  ticket,
		"node_id": "node-events",
		"state":   "succeeded",
	})

	waitForState("succeeded")
}

func TestServerNodesLifecycle(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	etcd, client := startTestEtcd(t)
	defer etcd.Close()
	defer func() { _ = client.Close() }()

	manager := mustBootstrapCluster(t, client, "cluster-alpha")

	sched, err := scheduler.New(client, scheduler.Options{LeaseTTL: 3 * time.Second})
	if err != nil {
		t.Fatalf("new scheduler: %v", err)
	}
	defer func() { _ = sched.Close() }()

	server := httptest.NewServer(httpserver.NewControlPlaneHandler(httpserver.ControlPlaneOptions{
		Scheduler: sched,
		Etcd:      client,
	}))
	defer server.Close()

	status, nodeResp := postJSONStatus(t, server.URL+"/v1/nodes", map[string]any{
		"cluster_id": "cluster-alpha",
		"address":    "10.20.1.50",
		"labels": map[string]any{
			"role": "build",
		},
	})
	if status != http.StatusCreated {
		t.Fatalf("expected status 201, got %d", status)
	}

	workerID, ok := nodeResp["worker_id"].(string)
	if !ok || strings.TrimSpace(workerID) == "" {
		t.Fatalf("expected worker_id in response, got %+v", nodeResp["worker_id"])
	}

	desc, ok := nodeResp["descriptor"].(map[string]any)
	if !ok {
		t.Fatalf("expected descriptor map in response")
	}
	if address, _ := desc["address"].(string); address != "10.20.1.50" {
		t.Fatalf("unexpected descriptor address %q", address)
	}
	statusMap, ok := desc["status"].(map[string]any)
	if !ok {
		t.Fatalf("expected status block in descriptor")
	}
	if phase, _ := statusMap["phase"].(string); phase != registry.WorkerPhaseReady {
		t.Fatalf("expected ready phase, got %q", phase)
	}

	listStatus, listResp := getJSONStatus(t, fmt.Sprintf("%s/v1/nodes?cluster_id=%s", server.URL, url.QueryEscape("cluster-alpha")))
	if listStatus != http.StatusOK {
		t.Fatalf("expected list status 200, got %d", listStatus)
	}
	nodes, ok := listResp["nodes"].([]any)
	if !ok || len(nodes) == 0 {
		t.Fatalf("expected nodes array in list response")
	}
	var entry map[string]any
	for _, item := range nodes {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if id, _ := m["id"].(string); id == workerID {
			entry = m
			break
		}
	}
	if entry == nil {
		t.Fatalf("worker %s missing from listing", workerID)
	}
	if version, _ := entry["certificate_version"].(string); strings.TrimSpace(version) == "" {
		t.Fatalf("expected certificate version recorded in listing")
	}

	jobTicket := "mod-node"
	job, err := sched.SubmitJob(ctx, scheduler.JobSpec{
		Ticket:      jobTicket,
		StepID:      "build",
		Priority:    "default",
		MaxAttempts: 1,
	})
	if err != nil {
		t.Fatalf("submit job: %v", err)
	}

	claim, err := sched.ClaimNext(ctx, scheduler.ClaimRequest{NodeID: workerID})
	if err != nil {
		t.Fatalf("claim job: %v", err)
	}

	completed := make(chan struct{})
	go func() {
		defer close(completed)
		time.Sleep(150 * time.Millisecond)
		_, err := sched.CompleteJob(ctx, scheduler.CompleteRequest{
			JobID:  claim.Job.ID,
			NodeID: workerID,
			Ticket: job.Ticket,
			State:  scheduler.JobStateSucceeded,
		})
		if err != nil {
			t.Errorf("complete job: %v", err)
		}
	}()

	deleteStatus, _ := deleteJSONStatus(t, server.URL+"/v1/nodes", map[string]any{
		"cluster_id":            "cluster-alpha",
		"worker_id":             workerID,
		"confirm":               workerID,
		"drain_timeout_seconds": 5,
	})
	if deleteStatus != http.StatusNoContent {
		t.Fatalf("expected delete status 204, got %d", deleteStatus)
	}

	select {
	case <-completed:
	case <-time.After(2 * time.Second):
		t.Fatalf("job completion goroutine did not finish")
	}

	state, err := manager.State(ctx)
	if err != nil {
		t.Fatalf("manager state: %v", err)
	}
	for _, id := range state.Nodes.Workers {
		if id == workerID {
			t.Fatalf("expected worker removed from CA inventory")
		}
	}
	if _, ok := state.WorkerCertificates[workerID]; ok {
		t.Fatalf("expected worker certificate removed from CA state")
	}

	reg, err := registry.NewWorkerRegistry(client, "cluster-alpha")
	if err != nil {
		t.Fatalf("new worker registry: %v", err)
	}
	if _, err := reg.Get(ctx, workerID); !errors.Is(err, registry.ErrWorkerNotFound) {
		t.Fatalf("expected registry ErrWorkerNotFound, got %v", err)
	}
}

func TestStatusSummaryIncludesQueueAndWorkers(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	etcd, client := startTestEtcd(t)
	defer etcd.Close()
	defer func() { _ = client.Close() }()

	_ = mustBootstrapCluster(t, client, "cluster-alpha")

	registryClient, err := registry.NewWorkerRegistry(client, "cluster-alpha")
	if err != nil {
		t.Fatalf("new worker registry: %v", err)
	}
	now := time.Date(2025, 10, 24, 9, 45, 0, 0, time.UTC)
	descriptor := registry.WorkerDescriptor{
		ID:           "worker-ready",
		Address:      "10.21.0.10",
		RegisteredAt: now,
		Status: registry.WorkerStatus{
			Phase:     registry.WorkerPhaseReady,
			CheckedAt: now,
		},
	}
	if _, err := registryClient.Register(ctx, descriptor); err != nil {
		t.Fatalf("register worker: %v", err)
	}

	promRegistry := prometheus.NewRegistry()
	recorder, err := metrics.NewSchedulerMetrics(promRegistry)
	if err != nil {
		t.Fatalf("new scheduler metrics: %v", err)
	}

	sched, err := scheduler.New(client, scheduler.Options{
		LeaseTTL: 3 * time.Second,
		Metrics:  recorder,
	})
	if err != nil {
		t.Fatalf("new scheduler: %v", err)
	}
	defer func() { _ = sched.Close() }()

	principal := newTestPrincipal([]string{security.ScopeAdmin})
	handler := httpserver.NewControlPlaneHandler(httpserver.ControlPlaneOptions{
		Scheduler: sched,
		Etcd:      client,
		Gatherer:  promRegistry,
		Auth:      security.NewManager(&testTokenVerifier{principal: principal}),
	})

	if _, err := sched.SubmitJob(ctx, scheduler.JobSpec{
		Ticket:      "mod-status",
		StepID:      "plan",
		Priority:    "default",
		MaxAttempts: 1,
	}); err != nil {
		t.Fatalf("submit job: %v", err)
	}

	req := newMTLSRequest(t, http.MethodGet, "/v1/status?cluster_id=cluster-alpha", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 from status endpoint, got %d", rec.Code)
	}
	if cache := rec.Header().Get("Cache-Control"); cache != "no-store" {
		t.Fatalf("expected Cache-Control no-store, got %q", cache)
	}

	var body map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode status response: %v", err)
	}
	if ts, _ := body["timestamp"].(string); strings.TrimSpace(ts) == "" {
		t.Fatalf("expected timestamp in status response")
	}
	queueBlock, ok := body["queue"].(map[string]any)
	if !ok {
		t.Fatalf("expected queue block in status response")
	}
	totalDepth, _ := queueBlock["total_depth"].(float64)
	if totalDepth < 1 {
		t.Fatalf("expected positive total_depth, got %v", totalDepth)
	}
	priorities, ok := queueBlock["priorities"].([]any)
	if !ok || len(priorities) == 0 {
		t.Fatalf("expected queue priorities slice, got %#v", queueBlock["priorities"])
	}
	workersBlock, ok := body["workers"].(map[string]any)
	if !ok {
		t.Fatalf("expected workers block in status response")
	}
	totalWorkers, _ := workersBlock["total"].(float64)
	if totalWorkers != 1 {
		t.Fatalf("expected total workers 1, got %v", totalWorkers)
	}
	phases, _ := workersBlock["phases"].(map[string]any)
	if ready, _ := phases[registry.WorkerPhaseReady].(float64); ready != 1 {
		t.Fatalf("expected ready workers 1, got %v", ready)
	}
}

func TestServerBeaconRotateCA(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	etcd, client := startTestEtcd(t)
	defer etcd.Close()
	defer func() { _ = client.Close() }()

	manager := mustBootstrapCluster(t, client, "cluster-alpha")

	_, err := deploy.RunWorkerJoin(ctx, client, deploy.WorkerJoinOptions{
		ClusterID:    "cluster-alpha",
		WorkerID:     "worker-rotate",
		Address:      "10.20.2.12",
		HealthProbes: nil,
		Clock:        func() time.Time { return time.Date(2025, 10, 22, 12, 0, 0, 0, time.UTC) },
	})
	if err != nil {
		t.Fatalf("RunWorkerJoin: %v", err)
	}

	sched, err := scheduler.New(client, scheduler.Options{LeaseTTL: 3 * time.Second})
	if err != nil {
		t.Fatalf("new scheduler: %v", err)
	}
	defer func() { _ = sched.Close() }()

	server := httptest.NewServer(httpserver.NewControlPlaneHandler(httpserver.ControlPlaneOptions{
		Scheduler: sched,
		Etcd:      client,
	}))
	defer server.Close()

	initial, err := manager.State(ctx)
	if err != nil {
		t.Fatalf("initial state: %v", err)
	}

	status, resp := postJSONStatus(t, server.URL+"/v1/beacon/rotate-ca", map[string]any{
		"cluster_id": "cluster-alpha",
		"operator":   "ci-bot",
		"reason":     "expiry-test",
	})
	if status != http.StatusOK {
		t.Fatalf("expected rotate status 200, got %d", status)
	}

	oldVersion, _ := resp["old_version"].(string)
	newVersion, _ := resp["new_version"].(string)
	if oldVersion != initial.CurrentCA.Version {
		t.Fatalf("expected old version %s, got %s", initial.CurrentCA.Version, oldVersion)
	}
	if oldVersion == "" || newVersion == "" || oldVersion == newVersion {
		t.Fatalf("expected rotation to change CA version, got old=%q new=%q", oldVersion, newVersion)
	}
	if operator, _ := resp["operator"].(string); operator != "ci-bot" {
		t.Fatalf("expected operator ci-bot, got %q", operator)
	}
	if reason, _ := resp["reason"].(string); reason != "expiry-test" {
		t.Fatalf("expected reason expiry-test, got %q", reason)
	}

	updated, err := manager.State(ctx)
	if err != nil {
		t.Fatalf("updated state: %v", err)
	}
	if updated.CurrentCA.Version != newVersion {
		t.Fatalf("expected state current version %s, got %s", newVersion, updated.CurrentCA.Version)
	}
	if _, ok := updated.WorkerCertificates["worker-rotate"]; !ok {
		t.Fatalf("expected worker certificate reissued for worker-rotate")
	}
}

func TestMetricsEndpointExposesPrometheus(t *testing.T) {
	t.Parallel()

	etcd, client := startTestEtcd(t)
	defer etcd.Close()
	defer func() { _ = client.Close() }()

	reg := prometheus.NewRegistry()
	recorder, err := metrics.NewSchedulerMetrics(reg)
	if err != nil {
		t.Fatalf("new scheduler metrics: %v", err)
	}

	sched, err := scheduler.New(client, scheduler.Options{
		LeaseTTL: 3 * time.Second,
		Metrics:  recorder,
	})
	if err != nil {
		t.Fatalf("new scheduler: %v", err)
	}
	defer func() { _ = sched.Close() }()

	server := httptest.NewServer(httpserver.NewControlPlaneHandler(httpserver.ControlPlaneOptions{
		Scheduler: sched,
		Gatherer:  reg,
		Etcd:      client,
	}))
	defer server.Close()

	postJSON(t, server.URL+"/v1/jobs", map[string]any{
		"ticket":       "mod-observe",
		"step_id":      "build",
		"priority":     "default",
		"max_attempts": 1,
	})

	resp, err := http.Get(server.URL + "/metrics")
	if err != nil {
		t.Fatalf("fetch metrics: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("unexpected status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read metrics body: %v", err)
	}
	text := string(body)
	if !strings.Contains(text, "ploy_controlplane_queue_depth") {
		t.Fatalf("expected queue depth metric in scrape output")
	}
	if !strings.Contains(text, `priority="default"`) {
		t.Fatalf("expected queue depth labels recorded")
	}
}

func TestServerGitLabConfig(t *testing.T) {
	t.Parallel()

	etcd, client := startTestEtcd(t)
	defer etcd.Close()
	defer func() { _ = client.Close() }()

	sched, err := scheduler.New(client, scheduler.Options{LeaseTTL: 3 * time.Second})
	if err != nil {
		t.Fatalf("new scheduler: %v", err)
	}
	defer func() { _ = sched.Close() }()

	server := httptest.NewServer(httpserver.NewControlPlaneHandler(httpserver.ControlPlaneOptions{
		Scheduler: sched,
		Etcd:      client,
	}))
	defer server.Close()

	status, _ := getJSONStatus(t, server.URL+"/v1/config/gitlab")
	if status != http.StatusNotFound {
		t.Fatalf("expected 404 for missing config, got %d", status)
	}

	createPayload := map[string]any{
		"revision": 0,
		"config": map[string]any{
			"api_base_url":     "https://gitlab.local/api/v4",
			"allowed_projects": []any{"acme/ploy"},
			"default_token":    map[string]any{"name": "default", "value": "glpat-secret", "scopes": []any{"api"}},
			"deploy_tokens": []any{
				map[string]any{"name": "deploy", "value": "glpat-deploy", "scopes": []any{"read_repository"}},
			},
			"branch_policies": []any{},
			"rbac":            map[string]any{"readers": []any{"ops"}, "updaters": []any{"ops", "release"}},
		},
	}

	putStatus, putResp := putJSONStatus(t, server.URL+"/v1/config/gitlab", createPayload)
	if putStatus != http.StatusOK {
		t.Fatalf("expected put status 200, got %d", putStatus)
	}
	revision := int64(putResp["revision"].(float64))
	if revision == 0 {
		t.Fatalf("expected non-zero revision after create")
	}

	getStatus, getResp := getJSONStatus(t, server.URL+"/v1/config/gitlab")
	if getStatus != http.StatusOK {
		t.Fatalf("expected get status 200, got %d", getStatus)
	}
	cfg, ok := getResp["config"].(map[string]any)
	if !ok {
		t.Fatalf("expected config object in get response")
	}
	defaultToken, _ := cfg["default_token"].(map[string]any)
	if defaultToken == nil {
		t.Fatalf("expected default_token in config response")
	}
	if value, _ := defaultToken["value"].(string); value != "***redacted***" {
		t.Fatalf("expected default token to be masked, got %q", value)
	}

	updatePayload := map[string]any{
		"revision": revision,
		"config": map[string]any{
			"api_base_url":     "https://gitlab.local/api/v4",
			"allowed_projects": []any{"acme/ploy", "acme/api"},
			"default_token":    map[string]any{"name": "default", "value": "glpat-secret", "scopes": []any{"api", "read_repository"}},
			"deploy_tokens": []any{
				map[string]any{"name": "deploy", "value": "glpat-deploy", "scopes": []any{"read_repository"}},
			},
			"branch_policies": []any{
				map[string]any{"pattern": "main", "protected": true, "require_approvals": 1},
			},
			"rbac": map[string]any{
				"readers":  []any{"ops"},
				"updaters": []any{"ops", "release"},
			},
		},
	}

	updateStatus, updateResp := putJSONStatus(t, server.URL+"/v1/config/gitlab", updatePayload)
	if updateStatus != http.StatusOK {
		t.Fatalf("expected update status 200, got %d", updateStatus)
	}
	newRevision := int64(updateResp["revision"].(float64))
	if newRevision == revision || newRevision == 0 {
		t.Fatalf("expected new revision different from previous")
	}

	stalePayload := map[string]any{
		"revision": revision,
		"config":   updatePayload["config"],
	}
	staleStatus, staleResp := putJSONStatus(t, server.URL+"/v1/config/gitlab", stalePayload)
	if staleStatus != http.StatusConflict {
		t.Fatalf("expected conflict status, got %d", staleStatus)
	}
	if message, _ := staleResp["error"].(string); !strings.Contains(message, "revision mismatch") {
		t.Fatalf("expected revision mismatch error, got %q", message)
	}
}

func TestServerGitLabSignerEndpoints(t *testing.T) {
	t.Parallel()

	etcd, client := startTestEtcd(t)
	defer etcd.Close()
	defer func() {
		_ = client.Close()
	}()

	sched, err := scheduler.New(client, scheduler.Options{LeaseTTL: 3 * time.Second})
	if err != nil {
		t.Fatalf("new scheduler: %v", err)
	}
	defer func() {
		_ = sched.Close()
	}()

	key := strings.Repeat("l", 32)
	cipher, err := gitlab.NewAESCipher([]byte(key))
	if err != nil {
		t.Fatalf("new cipher: %v", err)
	}
	signer, err := gitlab.NewSigner(client, cipher)
	if err != nil {
		t.Fatalf("new signer: %v", err)
	}
	defer func() {
		_ = signer.Close()
	}()

	server := httptest.NewServer(httpserver.NewControlPlaneHandler(httpserver.ControlPlaneOptions{
		Scheduler: sched,
		Signer:    signer,
		Etcd:      client,
	}))
	defer server.Close()

	rotateResp := putJSON(t, server.URL+"/v1/gitlab/signer/secrets", map[string]any{
		"secret":  "runner",
		"api_key": "glpat-first",
		"scopes":  []string{"api", "read_repository"},
	})
	initialRevision := int64(rotateResp["revision"].(float64))
	if initialRevision == 0 {
		t.Fatalf("expected initial revision > 0")
	}

	tokenResp := postJSON(t, server.URL+"/v1/gitlab/signer/tokens", map[string]any{
		"secret":      "runner",
		"scopes":      []string{"read_repository"},
		"ttl_seconds": 300,
		"node_id":     "node-http",
	})
	if tokenResp["secret"].(string) != "runner" {
		t.Fatalf("unexpected token secret: %v", tokenResp["secret"])
	}
	if tokenResp["token"].(string) == "" {
		t.Fatalf("expected token value")
	}
	if tokenResp["token_id"].(string) == "" {
		t.Fatalf("expected token_id in response")
	}
	if ttl := int64(tokenResp["ttl_seconds"].(float64)); ttl != 300 {
		t.Fatalf("expected ttl_seconds 300, got %d", ttl)
	}

	eventCh := make(chan map[string]any, 1)
	errCh := make(chan error, 1)

	go func() {
		url := fmt.Sprintf("%s/v1/gitlab/signer/rotations?timeout=5s&since=%d", server.URL, initialRevision)
		resp, err := http.Get(url)
		if err != nil {
			errCh <- err
			return
		}
		defer func() {
			_ = resp.Body.Close()
		}()
		if resp.StatusCode >= 400 {
			body, _ := io.ReadAll(resp.Body)
			errCh <- fmt.Errorf("rotation http %d: %s", resp.StatusCode, string(body))
			return
		}
		if resp.StatusCode == http.StatusNoContent {
			errCh <- fmt.Errorf("rotation returned no content")
			return
		}
		var payload map[string]any
		if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
			errCh <- fmt.Errorf("decode rotation: %w", err)
			return
		}
		eventCh <- payload
	}()

	time.Sleep(150 * time.Millisecond)
	putJSON(t, server.URL+"/v1/gitlab/signer/secrets", map[string]any{
		"secret":  "runner",
		"api_key": "glpat-second",
		"scopes":  []string{"api", "read_repository"},
	})

	select {
	case err := <-errCh:
		t.Fatalf("rotation watcher: %v", err)
	case evt := <-eventCh:
		if evt["secret"].(string) != "runner" {
			t.Fatalf("expected rotation secret runner, got %v", evt["secret"])
		}
		if rev := int64(evt["revision"].(float64)); rev <= initialRevision {
			t.Fatalf("expected revision > %d, got %d", initialRevision, rev)
		}
	case <-time.After(3 * time.Second):
		t.Fatalf("timed out waiting for rotation event")
	}
}

func TestLogsStreamDeliversEvents(t *testing.T) {
	t.Parallel()

	etcd, client := startTestEtcd(t)
	defer etcd.Close()
	defer func() {
		_ = client.Close()
	}()

	sched, err := scheduler.New(client, scheduler.Options{LeaseTTL: 3 * time.Second})
	if err != nil {
		t.Fatalf("new scheduler: %v", err)
	}
	defer func() {
		_ = sched.Close()
	}()

	streams := logstream.NewHub(logstream.Options{BufferSize: 8, HistorySize: 16})
	jobID := "job-stream-1"
	streams.Ensure(jobID)

	server := httptest.NewServer(httpserver.NewControlPlaneHandler(httpserver.ControlPlaneOptions{
		Scheduler: sched,
		Streams:   streams,
		Etcd:      client,
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	events := make(chan sseEvent, 4)
	errCh := make(chan error, 1)
	go func() {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("%s/v1/jobs/%s/logs/stream", server.URL, jobID), nil)
		if err != nil {
			errCh <- err
			return
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			errCh <- err
			return
		}
		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			errCh <- fmt.Errorf("http %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
			return
		}
		defer func() {
			_ = resp.Body.Close()
		}()
		reader := bufio.NewReader(resp.Body)
		for {
			evt, err := readSSEEvent(reader)
			if err != nil {
				errCh <- err
				return
			}
			events <- evt
			if evt.Type == "done" {
				return
			}
		}
	}()

	time.Sleep(50 * time.Millisecond)

	if err := streams.PublishLog(context.Background(), jobID, logstream.LogRecord{
		Timestamp: "2025-10-22T12:00:00Z",
		Stream:    "stdout",
		Line:      "starting job",
	}); err != nil {
		t.Fatalf("publish log: %v", err)
	}
	if err := streams.PublishRetention(context.Background(), jobID, logstream.RetentionHint{
		Retained: true,
		TTL:      "72h",
		Expires:  "2025-10-25T12:00:00Z",
		Bundle:   "bafy-log-bundle",
	}); err != nil {
		t.Fatalf("publish retention: %v", err)
	}
	if err := streams.PublishStatus(context.Background(), jobID, logstream.Status{Status: "completed"}); err != nil {
		t.Fatalf("publish status: %v", err)
	}

	expect := []struct {
		event string
		check func(data string)
	}{
		{
			event: "log",
			check: func(data string) {
				var payload logstream.LogRecord
				if err := json.Unmarshal([]byte(data), &payload); err != nil {
					t.Fatalf("decode log payload: %v", err)
				}
				if payload.Line != "starting job" {
					t.Fatalf("unexpected log line: %q", payload.Line)
				}
			},
		},
		{
			event: "retention",
			check: func(data string) {
				var payload logstream.RetentionHint
				if err := json.Unmarshal([]byte(data), &payload); err != nil {
					t.Fatalf("decode retention payload: %v", err)
				}
				if !payload.Retained || payload.Bundle != "bafy-log-bundle" {
					t.Fatalf("unexpected retention payload: %+v", payload)
				}
			},
		},
		{
			event: "done",
			check: func(data string) {
				var payload logstream.Status
				if err := json.Unmarshal([]byte(data), &payload); err != nil {
					t.Fatalf("decode status payload: %v", err)
				}
				if payload.Status != "completed" {
					t.Fatalf("unexpected status payload: %+v", payload)
				}
			},
		},
	}

	for _, want := range expect {
		select {
		case evt := <-events:
			if evt.Type != want.event {
				t.Fatalf("expected event %q, got %q", want.event, evt.Type)
			}
			want.check(evt.Data)
		case err := <-errCh:
			t.Fatalf("stream error: %v", err)
		case <-time.After(1 * time.Second):
			t.Fatalf("timed out waiting for %s event", want.event)
		}
	}
}

func TestLogsStreamResumesWithLastEventID(t *testing.T) {
	t.Parallel()

	etcd, client := startTestEtcd(t)
	defer etcd.Close()
	defer func() { _ = client.Close() }()

	sched, err := scheduler.New(client, scheduler.Options{LeaseTTL: 3 * time.Second})
	if err != nil {
		t.Fatalf("new scheduler: %v", err)
	}
	defer func() { _ = sched.Close() }()

	streams := logstream.NewHub(logstream.Options{BufferSize: 8, HistorySize: 16})
	jobID := "job-resume-1"
	streams.Ensure(jobID)

	server := httptest.NewServer(httpserver.NewControlPlaneHandler(httpserver.ControlPlaneOptions{
		Scheduler: sched,
		Streams:   streams,
		Etcd:      client,
	}))
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	firstEvents := make(chan sseEvent, 3)
	errCh := make(chan error, 1)

	go func() {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("%s/v1/jobs/%s/logs/stream", server.URL, jobID), nil)
		if err != nil {
			errCh <- err
			return
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			errCh <- err
			return
		}
		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			errCh <- fmt.Errorf("http %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
			return
		}
		defer func() {
			_ = resp.Body.Close()
		}()

		reader := bufio.NewReader(resp.Body)
		for {
			evt, err := readSSEEvent(reader)
			if err != nil {
				errCh <- err
				return
			}
			firstEvents <- evt
			if len(firstEvents) == 2 {
				cancel()
				return
			}
		}
	}()

	go func() {
		_ = streams.PublishLog(context.Background(), jobID, logstream.LogRecord{Timestamp: "2025-10-22T12:10:00Z", Stream: "stdout", Line: "phase one"})
		time.Sleep(50 * time.Millisecond)
		_ = streams.PublishRetention(context.Background(), jobID, logstream.RetentionHint{Retained: false, TTL: "", Bundle: ""})
		time.Sleep(50 * time.Millisecond)
		_ = streams.PublishStatus(context.Background(), jobID, logstream.Status{Status: "completed"})
	}()

	var lastID string
	for i := 0; i < 2; i++ {
		select {
		case evt := <-firstEvents:
			lastID = evt.ID
		case err := <-errCh:
			t.Fatalf("stream error: %v", err)
		case <-time.After(2 * time.Second):
			t.Fatalf("timeout waiting for initial events")
		}
	}

	if lastID == "" {
		t.Fatalf("expected last event id to be captured")
	}

	resumeReq, err := http.NewRequest(http.MethodGet, fmt.Sprintf("%s/v1/jobs/%s/logs/stream", server.URL, jobID), nil)
	if err != nil {
		t.Fatalf("resume request: %v", err)
	}
	resumeReq.Header.Set("Last-Event-ID", lastID)

	resp, err := http.DefaultClient.Do(resumeReq)
	if err != nil {
		t.Fatalf("resume stream: %v", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("resume http %d: %s", resp.StatusCode, string(body))
	}

	reader := bufio.NewReader(resp.Body)
	evt, err := readSSEEvent(reader)
	if err != nil {
		t.Fatalf("resume read: %v", err)
	}
	if evt.Type != "done" {
		t.Fatalf("expected done event on resume, got %s", evt.Type)
	}
}

type modsServerFixture struct {
	server    *httptest.Server
	etcd      *embed.Etcd
	client    *clientv3.Client
	scheduler *scheduler.Scheduler
	mods      *controlplanemods.Service
	streams   *logstream.Hub
}

func newModsServerFixture(t *testing.T) *modsServerFixture {
	t.Helper()
	etcd, client := startTestEtcd(t)

	sched, err := scheduler.New(client, scheduler.Options{LeaseTTL: 3 * time.Second})
	if err != nil {
		t.Fatalf("new scheduler: %v", err)
	}

	service, err := controlplanemods.NewService(client, controlplanemods.Options{
		Scheduler: controlplanemods.NewSchedulerBridge(sched),
		Clock:     func() time.Time { return time.Date(2025, 10, 24, 10, 0, 0, 0, time.UTC) },
	})
	if err != nil {
		t.Fatalf("new mods service: %v", err)
	}

	streams := logstream.NewHub(logstream.Options{BufferSize: 8, HistorySize: 32})

	handler := httpserver.NewControlPlaneHandler(httpserver.ControlPlaneOptions{
		Scheduler: sched,
		Etcd:      client,
		Mods:      service,
		Streams:   streams,
	})
	server := httptest.NewServer(handler)

	fixture := &modsServerFixture{
		server:    server,
		etcd:      etcd,
		client:    client,
		scheduler: sched,
		mods:      service,
		streams:   streams,
	}

	t.Cleanup(func() {
		server.Close()
		_ = service.Close()
		_ = sched.Close()
		_ = client.Close()
		etcd.Close()
	})

	return fixture
}

type sseEvent struct {
	ID   string
	Type string
	Data string
}

func readSSEEvent(r *bufio.Reader) (sseEvent, error) {
	var evt sseEvent
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return evt, err
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			if evt.Type == "" && evt.Data == "" && evt.ID == "" {
				continue
			}
			return evt, nil
		}
		switch {
		case strings.HasPrefix(line, "event:"):
			evt.Type = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
		case strings.HasPrefix(line, "data:"):
			if evt.Data != "" {
				evt.Data += "\n"
			}
			evt.Data += strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		case strings.HasPrefix(line, "id:"):
			evt.ID = strings.TrimSpace(strings.TrimPrefix(line, "id:"))
		default:
			// ignore comments and unknown fields
		}
	}
}

func mustBootstrapCluster(t *testing.T, client *clientv3.Client, clusterID string) *deploy.CARotationManager {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	manager, err := deploy.NewCARotationManager(client, clusterID)
	if err != nil {
		t.Fatalf("new ca rotation manager: %v", err)
	}
	_, err = manager.Bootstrap(ctx, deploy.BootstrapOptions{
		BeaconIDs: []string{"beacon-main"},
	})
	if err != nil && !errors.Is(err, deploy.ErrPKIAlreadyBootstrapped) {
		t.Fatalf("bootstrap ca: %v", err)
	}
	return manager
}

func TestBeaconNodesRequiresClusterID(t *testing.T) {
	t.Parallel()

	etcd, client := startTestEtcd(t)
	defer etcd.Close()
	defer func() { _ = client.Close() }()

	handler := httpserver.NewControlPlaneHandler(httpserver.ControlPlaneOptions{
		Etcd: client,
	})

	req := newMTLSRequest(t, http.MethodGet, "/v1/beacon/nodes", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 without cluster_id, got %d", rec.Code)
	}
}

func TestBeaconNodesReturnsSignedPayload(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	etcd, client := startTestEtcd(t)
	defer etcd.Close()
	defer func() { _ = client.Close() }()

	manager := mustBootstrapCluster(t, client, "cluster-alpha")

	handler := httpserver.NewControlPlaneHandler(httpserver.ControlPlaneOptions{
		Etcd: client,
	})

	req := newMTLSRequest(t, http.MethodGet, "/v1/beacon/nodes?cluster_id=cluster-alpha", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 from beacon nodes, got %d", rec.Code)
	}
	if cache := rec.Header().Get("Cache-Control"); !strings.Contains(cache, "max-age") {
		t.Fatalf("expected cache header with max-age, got %q", cache)
	}

	env := decodeSignedEnvelope(t, rec.Body.Bytes())

	var payload map[string]any
	if err := json.Unmarshal(env.Payload, &payload); err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	if cluster, _ := payload["cluster_id"].(string); cluster != "cluster-alpha" {
		t.Fatalf("unexpected cluster_id %q", cluster)
	}
	beacons, ok := payload["beacons"].([]any)
	if !ok || len(beacons) == 0 {
		t.Fatalf("expected beacons array in payload, got %#v", payload["beacons"])
	}
	certPEM := stateCurrentCACert(t, manager, ctx)
	verifyBeaconSignature(t, certPEM, env.Payload, env.Signature.Value)
}

func TestBeaconCAReturnsSignedBundle(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	etcd, client := startTestEtcd(t)
	defer etcd.Close()
	defer func() { _ = client.Close() }()

	manager := mustBootstrapCluster(t, client, "cluster-alpha")

	handler := httpserver.NewControlPlaneHandler(httpserver.ControlPlaneOptions{
		Etcd: client,
	})

	req := newMTLSRequest(t, http.MethodGet, "/v1/beacon/ca?cluster_id=cluster-alpha", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 from beacon ca, got %d", rec.Code)
	}

	env := decodeSignedEnvelope(t, rec.Body.Bytes())

	var payload map[string]any
	if err := json.Unmarshal(env.Payload, &payload); err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	state, err := manager.State(ctx)
	if err != nil {
		t.Fatalf("manager state: %v", err)
	}
	if versionID, _ := payload["version"].(string); versionID != state.CurrentCA.Version {
		t.Fatalf("unexpected CA version %q", versionID)
	}
	if certPEM, _ := payload["certificate_pem"].(string); strings.TrimSpace(certPEM) == "" {
		t.Fatalf("expected certificate_pem in payload")
	}
	verifyBeaconSignature(t, state.CurrentCA.CertificatePEM, env.Payload, env.Signature.Value)
}

func TestBeaconConfigReturnsSignedDocument(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	etcd, client := startTestEtcd(t)
	defer etcd.Close()
	defer func() { _ = client.Close() }()

	manager := mustBootstrapCluster(t, client, "cluster-alpha")

	principal := newTestPrincipal([]string{security.ScopeAdmin})
	handler := httpserver.NewControlPlaneHandler(httpserver.ControlPlaneOptions{
		Etcd: client,
		Auth: security.NewManager(&testTokenVerifier{principal: principal}),
	})

	body := map[string]any{
		"cluster_id": "cluster-alpha",
		"config": map[string]any{
			"endpoints": map[string]any{"etcd": []any{"https://127.0.0.1:2379"}},
		},
		"version_tag": "2025.10.24",
		"updated_by":  "ops",
	}
	payload, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal config body: %v", err)
	}
	putReq := newMTLSRequest(t, http.MethodPut, "/v1/config", bytes.NewReader(payload))
	putReq.Header.Set("Content-Type", "application/json")
	putReq.Header.Set("If-Match", "0")
	putRec := httptest.NewRecorder()
	handler.ServeHTTP(putRec, putReq)
	if putRec.Code != http.StatusOK {
		t.Fatalf("expected 200 creating config, got %d", putRec.Code)
	}

	req := newMTLSRequest(t, http.MethodGet, "/v1/beacon/config?cluster_id=cluster-alpha", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 from beacon config, got %d", rec.Code)
	}
	env := decodeSignedEnvelope(t, rec.Body.Bytes())

	var doc map[string]any
	if err := json.Unmarshal(env.Payload, &doc); err != nil {
		t.Fatalf("decode beacon config payload: %v", err)
	}
	cfg, ok := doc["config"].(map[string]any)
	if !ok {
		t.Fatalf("expected config block in payload")
	}
	if endpoints, ok := cfg["endpoints"].(map[string]any); !ok || len(endpoints) == 0 {
		t.Fatalf("expected endpoints map in beacon config payload, got %#v", cfg["endpoints"])
	}
	verifyBeaconSignature(t, stateCurrentCACert(t, manager, ctx), env.Payload, env.Signature.Value)
}

func TestBeaconPromoteStoresCanonicalBeacon(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	etcd, client := startTestEtcd(t)
	defer etcd.Close()
	defer func() { _ = client.Close() }()

	manager := mustBootstrapCluster(t, client, "cluster-alpha")

	principal := newTestPrincipal([]string{security.ScopeAdmin})
	handler := httpserver.NewControlPlaneHandler(httpserver.ControlPlaneOptions{
		Etcd: client,
		Auth: security.NewManager(&testTokenVerifier{principal: principal}),
	})

	body := map[string]any{
		"cluster_id": "cluster-alpha",
		"beacon_id":  "beacon-main",
		"operator":   "ops",
	}
	payload, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal promote body: %v", err)
	}

	req := newMTLSRequest(t, http.MethodPost, "/v1/beacon/promote", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 from beacon promote, got %d", rec.Code)
	}

	env := decodeSignedEnvelope(t, rec.Body.Bytes())

	var doc map[string]any
	if err := json.Unmarshal(env.Payload, &doc); err != nil {
		t.Fatalf("decode promotion payload: %v", err)
	}
	if id, _ := doc["beacon_id"].(string); id != "beacon-main" {
		t.Fatalf("unexpected beacon_id %q", id)
	}
	verifyBeaconSignature(t, stateCurrentCACert(t, manager, ctx), env.Payload, env.Signature.Value)

	resp, err := client.Get(ctx, beaconCanonicalKey("cluster-alpha"))
	if err != nil {
		t.Fatalf("read canonical beacon: %v", err)
	}
	if len(resp.Kvs) != 1 {
		t.Fatalf("expected canonical beacon persisted, got %d keys", len(resp.Kvs))
	}
	var stored map[string]any
	if err := json.Unmarshal(resp.Kvs[0].Value, &stored); err != nil {
		t.Fatalf("decode canonical beacon: %v", err)
	}
	if storedID, _ := stored["beacon_id"].(string); storedID != "beacon-main" {
		t.Fatalf("unexpected stored beacon_id %q", storedID)
	}
}

type signedEnvelope struct {
	Payload   json.RawMessage `json:"payload"`
	Signature signatureDTO    `json:"signature"`
}

type signatureDTO struct {
	Algorithm string `json:"algorithm"`
	KeyID     string `json:"key_id"`
	Value     string `json:"value"`
}

func decodeSignedEnvelope(t *testing.T, data []byte) signedEnvelope {
	t.Helper()
	var env signedEnvelope
	if err := json.Unmarshal(data, &env); err != nil {
		t.Fatalf("decode signed envelope: %v", err)
	}
	if len(env.Payload) == 0 {
		t.Fatalf("signed envelope missing payload")
	}
	if env.Signature.Value == "" {
		t.Fatalf("signed envelope missing signature")
	}
	return env
}

func verifyBeaconSignature(t *testing.T, certPEM string, payload []byte, sigBase64 string) {
	t.Helper()
	block, _ := pem.Decode([]byte(certPEM))
	if block == nil {
		t.Fatalf("decode certificate PEM failed")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatalf("parse certificate: %v", err)
	}
	pub, ok := cert.PublicKey.(*ecdsa.PublicKey)
	if !ok {
		t.Fatalf("certificate public key is not ECDSA")
	}
	sig, err := base64.StdEncoding.DecodeString(sigBase64)
	if err != nil {
		t.Fatalf("decode signature: %v", err)
	}
	sum := sha256.Sum256(payload)
	if !ecdsa.VerifyASN1(pub, sum[:], sig) {
		t.Fatalf("signature verification failed")
	}
}

func stateCurrentCACert(t *testing.T, manager *deploy.CARotationManager, ctx context.Context) string {
	t.Helper()
	state, err := manager.State(ctx)
	if err != nil {
		t.Fatalf("manager state: %v", err)
	}
	return state.CurrentCA.CertificatePEM
}

func beaconCanonicalKey(clusterID string) string {
	return fmt.Sprintf("/ploy/clusters/%s/beacon/canonical", clusterID)
}

func startTestEtcd(t *testing.T) (*embed.Etcd, *clientv3.Client) {
	t.Helper()
	dir := t.TempDir()
	cfg := embed.NewConfig()
	cfg.Dir = dir
	clientURL := mustParseURL("http://127.0.0.1:0")
	peerURL := mustParseURL("http://127.0.0.1:0")
	cfg.ListenClientUrls = []url.URL{clientURL}
	cfg.ListenPeerUrls = []url.URL{peerURL}
	cfg.AdvertiseClientUrls = []url.URL{clientURL}
	cfg.AdvertisePeerUrls = []url.URL{peerURL}
	cfg.Name = "default"
	cfg.InitialCluster = fmt.Sprintf("%s=%s", cfg.Name, peerURL.String())
	cfg.ClusterState = embed.ClusterStateFlagNew
	cfg.InitialClusterToken = "httpapi-test"
	cfg.LogLevel = "panic"
	cfg.Logger = "zap"
	cfg.LogOutputs = []string{filepath.Join(dir, "etcd.log")}

	e, err := embed.StartEtcd(cfg)
	if err != nil {
		t.Fatalf("start etcd: %v", err)
	}
	select {
	case <-e.Server.ReadyNotify():
	case <-time.After(10 * time.Second):
		e.Server.Stop()
		t.Fatalf("etcd start timeout")
	}

	client, err := clientv3.New(clientv3.Config{
		Endpoints:   []string{e.Clients[0].Addr().String()},
		DialTimeout: 5 * time.Second,
	})
	if err != nil {
		e.Close()
		t.Fatalf("client: %v", err)
	}

	return e, client
}

func mustParseURL(raw string) url.URL {
	parsed, err := url.Parse(raw)
	if err != nil {
		panic(err)
	}
	return *parsed
}

func postJSON(t *testing.T, endpoint string, payload map[string]any) map[string]any {
	status, out := postJSONStatus(t, endpoint, payload)
	if status >= 400 {
		t.Fatalf("post %s -> http %d: %v", endpoint, status, out)
	}
	return out
}

func putJSON(t *testing.T, endpoint string, payload map[string]any) map[string]any {
	status, out := putJSONStatus(t, endpoint, payload)
	if status >= 400 {
		t.Fatalf("put %s -> http %d: %v", endpoint, status, out)
	}
	return out
}

func getJSON(t *testing.T, endpoint string) map[string]any {
	status, out := getJSONStatus(t, endpoint)
	if status >= 400 {
		t.Fatalf("get %s -> http %d: %v", endpoint, status, out)
	}
	return out
}

func postJSONStatus(t *testing.T, endpoint string, payload map[string]any) (int, map[string]any) {
	return sendJSONStatus(t, http.MethodPost, endpoint, payload)
}

func putJSONStatus(t *testing.T, endpoint string, payload map[string]any) (int, map[string]any) {
	return sendJSONStatus(t, http.MethodPut, endpoint, payload)
}

func deleteJSONStatus(t *testing.T, endpoint string, payload map[string]any) (int, map[string]any) {
	return sendJSONStatus(t, http.MethodDelete, endpoint, payload)
}

func getJSONStatus(t *testing.T, endpoint string) (int, map[string]any) {
	t.Helper()
	resp, err := http.Get(endpoint)
	if err != nil {
		t.Fatalf("get %s: %v", endpoint, err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()
	status := resp.StatusCode
	data, _ := io.ReadAll(resp.Body)
	if len(data) == 0 {
		return status, nil
	}
	var out map[string]any
	if err := json.Unmarshal(data, &out); err != nil {
		out = map[string]any{"error": strings.TrimSpace(string(data))}
	}
	return status, out
}

func sendJSONStatus(t *testing.T, method, endpoint string, payload map[string]any) (int, map[string]any) {
	t.Helper()
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	req, err := http.NewRequest(method, endpoint, bytes.NewReader(body))
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, endpoint, err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()
	status := resp.StatusCode
	data, _ := io.ReadAll(resp.Body)
	if status == http.StatusNoContent {
		return status, nil
	}
	if len(data) == 0 {
		return status, nil
	}
	var out map[string]any
	if err := json.Unmarshal(data, &out); err != nil {
		out = map[string]any{"error": strings.TrimSpace(string(data))}
	}
	return status, out
}

func newTestPrincipal(scopes []string) security.Principal {
	now := time.Now().UTC()
	return security.Principal{
		SecretName: "test-client",
		TokenID:    "token-123",
		Scopes:     scopes,
		IssuedAt:   now,
		ExpiresAt:  now.Add(time.Hour),
	}
}

func newMTLSRequest(t *testing.T, method, target string, body io.Reader) *http.Request {
	t.Helper()
	req := httptest.NewRequest(method, target, body)
	req.TLS = &tls.ConnectionState{
		PeerCertificates: []*x509.Certificate{{}},
	}
	req.Header.Set("Authorization", "Bearer test-token")
	return req
}

type testTokenVerifier struct {
	principal security.Principal
	err       error
}

func (t *testTokenVerifier) Verify(ctx context.Context, token string) (security.Principal, error) {
	if t.err != nil {
		return security.Principal{}, t.err
	}
	return t.principal, nil
}
