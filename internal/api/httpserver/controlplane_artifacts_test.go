package httpserver_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/iw2rmb/ploy/internal/api/httpserver"
	"github.com/iw2rmb/ploy/internal/api/httpserver/security"
	controlplaneartifacts "github.com/iw2rmb/ploy/internal/controlplane/artifacts"
	workflowartifacts "github.com/iw2rmb/ploy/internal/workflow/artifacts"
)

func TestArtifactsListRequiresReadScope(t *testing.T) {
	t.Parallel()

	principal := newTestPrincipal([]string{"artifact.write"})
	handler := newTestControlPlaneHandler(t, httpserver.ControlPlaneOptions{
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

	etcd, client := startTestEtcd(t)
	defer etcd.Close()
	defer func() { _ = client.Close() }()

	store, err := controlplaneartifacts.NewStore(client, controlplaneartifacts.StoreOptions{})
	if err != nil {
		t.Fatalf("new artifact store: %v", err)
	}

	principal := newTestPrincipal([]string{security.ScopeArtifactsRead})
	handler := newTestControlPlaneHandler(t, httpserver.ControlPlaneOptions{
		Etcd:              client,
		Auth:              security.NewManager(&testTokenVerifier{principal: principal}),
		ArtifactStore:     store,
		ArtifactPublisher: &stubArtifactPublisher{},
	})

	req := newMTLSRequest(t, http.MethodGet, "/v1/artifacts", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if cache := rec.Header().Get("Cache-Control"); cache != "no-store" {
		t.Fatalf("expected Cache-Control no-store, got %q", cache)
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
	if cursor, _ := body["next_cursor"].(string); cursor != "" {
		t.Fatalf("expected empty cursor, got %q", cursor)
	}
}

func TestArtifactsListFiltersByJob(t *testing.T) {
	t.Parallel()

	etcd, client := startTestEtcd(t)
	defer etcd.Close()
	defer func() { _ = client.Close() }()

	store, err := controlplaneartifacts.NewStore(client, controlplaneartifacts.StoreOptions{})
	if err != nil {
		t.Fatalf("new artifact store: %v", err)
	}
	ctx := context.Background()
	if _, err := store.Create(ctx, controlplaneartifacts.Metadata{
		ID:     "artifact-alpha",
		JobID:  "job-artifacts",
		Stage:  "plan",
		CID:    "bafyplan",
		Digest: "sha256:plan",
		Size:   1024,
	}); err != nil {
		t.Fatalf("seed alpha: %v", err)
	}
	if _, err := store.Create(ctx, controlplaneartifacts.Metadata{
		ID:     "artifact-beta",
		JobID:  "job-other",
		Stage:  "plan",
		CID:    "bafyother",
		Digest: "sha256:other",
		Size:   2048,
	}); err != nil {
		t.Fatalf("seed beta: %v", err)
	}

	principal := newTestPrincipal([]string{security.ScopeArtifactsRead})
	handler := newTestControlPlaneHandler(t, httpserver.ControlPlaneOptions{
		Etcd:              client,
		Auth:              security.NewManager(&testTokenVerifier{principal: principal}),
		ArtifactStore:     store,
		ArtifactPublisher: &stubArtifactPublisher{},
	})

	req := newMTLSRequest(t, http.MethodGet, "/v1/artifacts?job_id=job-artifacts&limit=1", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var body map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	items := body["artifacts"].([]any)
	if len(items) != 1 {
		t.Fatalf("expected single artifact, got %d", len(items))
	}
	first := items[0].(map[string]any)
	if first["id"].(string) != "artifact-alpha" {
		t.Fatalf("unexpected artifact id %q", first["id"])
	}
	cursor, _ := body["next_cursor"].(string)
	if cursor == "" {
		t.Fatalf("expected cursor for pagination")
	}

	req = newMTLSRequest(t, http.MethodGet, "/v1/artifacts?job_id=job-artifacts&cursor="+url.QueryEscape(cursor), nil)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 on second page, got %d", rec.Code)
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode second response: %v", err)
	}
	items = body["artifacts"].([]any)
	if len(items) != 0 {
		t.Fatalf("expected no further artifacts, got %d", len(items))
	}
}

func TestArtifactsListFiltersByCID(t *testing.T) {
	t.Parallel()

	etcd, client := startTestEtcd(t)
	defer etcd.Close()
	defer func() { _ = client.Close() }()

	store, err := controlplaneartifacts.NewStore(client, controlplaneartifacts.StoreOptions{})
	if err != nil {
		t.Fatalf("new artifact store: %v", err)
	}
	ctx := context.Background()
	if _, err := store.Create(ctx, controlplaneartifacts.Metadata{
		ID:     "artifact-match",
		JobID:  "job-cid",
		CID:    "bafy-match",
		Digest: "sha256:match",
		Size:   128,
	}); err != nil {
		t.Fatalf("seed match: %v", err)
	}
	if _, err := store.Create(ctx, controlplaneartifacts.Metadata{
		ID:     "artifact-other",
		JobID:  "job-cid",
		CID:    "bafy-other",
		Digest: "sha256:other",
		Size:   256,
	}); err != nil {
		t.Fatalf("seed other: %v", err)
	}

	principal := newTestPrincipal([]string{security.ScopeArtifactsRead})
	handler := newTestControlPlaneHandler(t, httpserver.ControlPlaneOptions{
		Etcd:              client,
		Auth:              security.NewManager(&testTokenVerifier{principal: principal}),
		ArtifactStore:     store,
		ArtifactPublisher: &stubArtifactPublisher{},
	})

	req := newMTLSRequest(t, http.MethodGet, "/v1/artifacts?cid=bafy-match", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var body map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	items, _ := body["artifacts"].([]any)
	if len(items) != 1 {
		t.Fatalf("expected single artifact, got %d", len(items))
	}
	first := items[0].(map[string]any)
	if first["id"].(string) != "artifact-match" {
		t.Fatalf("unexpected artifact id %q", first["id"])
	}
	if first["cid"].(string) != "bafy-match" {
		t.Fatalf("unexpected cid %q", first["cid"])
	}
}

func TestArtifactsUploadPersistsMetadata(t *testing.T) {
	t.Parallel()

	etcd, client := startTestEtcd(t)
	defer etcd.Close()
	defer func() { _ = client.Close() }()

	store, err := controlplaneartifacts.NewStore(client, controlplaneartifacts.StoreOptions{})
	if err != nil {
		t.Fatalf("new artifact store: %v", err)
	}
	publisher := &stubArtifactPublisher{
		response: workflowartifacts.AddResponse{CID: "bafy-upload", Digest: "sha256:payload", Size: 7},
	}
	principal := newTestPrincipal([]string{security.ScopeArtifactsWrite})
	handler := newTestControlPlaneHandler(t, httpserver.ControlPlaneOptions{
		Etcd:              client,
		Auth:              security.NewManager(&testTokenVerifier{principal: principal}),
		ArtifactStore:     store,
		ArtifactPublisher: publisher,
	})

	req := newMTLSRequest(t, http.MethodPost, "/v1/artifacts/upload?job_id=job-upload&stage=plan&node_id=node-a&kind=repo&ttl=24h", strings.NewReader("payload"))
	req.Header.Set("Content-Type", "application/octet-stream")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", rec.Code)
	}
	var body map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode upload response: %v", err)
	}
	artifact := body["artifact"].(map[string]any)
	id := artifact["id"].(string)
	if id == "" {
		t.Fatalf("expected artifact id")
	}
	if artifact["cid"].(string) != "bafy-upload" {
		t.Fatalf("unexpected cid %q", artifact["cid"])
	}
	if artifact["pin_state"].(string) == "" {
		t.Fatalf("expected pin_state in response")
	}
	if _, err := store.Get(context.Background(), id); err != nil {
		t.Fatalf("missing metadata: %v", err)
	}
}

func TestArtifactsGetReturnsMetadata(t *testing.T) {
	t.Parallel()

	etcd, client := startTestEtcd(t)
	defer etcd.Close()
	defer func() { _ = client.Close() }()

	store, err := controlplaneartifacts.NewStore(client, controlplaneartifacts.StoreOptions{})
	if err != nil {
		t.Fatalf("new artifact store: %v", err)
	}
	if _, err := store.Create(context.Background(), controlplaneartifacts.Metadata{
		ID:     "artifact-inspect",
		JobID:  "job-inspect",
		Stage:  "plan",
		CID:    "bafyinspect",
		Digest: "sha256:inspect",
		Size:   512,
	}); err != nil {
		t.Fatalf("seed artifact: %v", err)
	}

	principal := newTestPrincipal([]string{security.ScopeArtifactsRead})
	handler := newTestControlPlaneHandler(t, httpserver.ControlPlaneOptions{
		Etcd:              client,
		Auth:              security.NewManager(&testTokenVerifier{principal: principal}),
		ArtifactStore:     store,
		ArtifactPublisher: &stubArtifactPublisher{},
	})
	req := newMTLSRequest(t, http.MethodGet, "/v1/artifacts/artifact-inspect", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var body map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	artifact := body["artifact"].(map[string]any)
	if artifact["cid"].(string) != "bafyinspect" {
		t.Fatalf("unexpected cid %q", artifact["cid"])
	}
	if artifact["pin_state"].(string) == "" {
		t.Fatalf("expected pin state in response: %#v", artifact)
	}
}

func TestArtifactsDeleteRequiresWriteScope(t *testing.T) {
	t.Parallel()

	principal := newTestPrincipal([]string{"artifact.read"})
	handler := newTestControlPlaneHandler(t, httpserver.ControlPlaneOptions{
		Auth: security.NewManager(&testTokenVerifier{principal: principal}),
	})

	req := newMTLSRequest(t, http.MethodDelete, "/v1/artifacts/bafytest", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 without write scope, got %d", rec.Code)
	}
}

func TestArtifactsDeleteRemovesMetadata(t *testing.T) {
	t.Parallel()

	etcd, client := startTestEtcd(t)
	defer etcd.Close()
	defer func() { _ = client.Close() }()

	store, err := controlplaneartifacts.NewStore(client, controlplaneartifacts.StoreOptions{})
	if err != nil {
		t.Fatalf("new artifact store: %v", err)
	}
	if _, err := store.Create(context.Background(), controlplaneartifacts.Metadata{
		ID:     "artifact-delete",
		JobID:  "job-delete",
		Stage:  "plan",
		CID:    "bafydelete",
		Digest: "sha256:delete",
		Size:   256,
	}); err != nil {
		t.Fatalf("seed artifact: %v", err)
	}

	principal := newTestPrincipal([]string{security.ScopeArtifactsWrite})
	handler := newTestControlPlaneHandler(t, httpserver.ControlPlaneOptions{
		Etcd:              client,
		Auth:              security.NewManager(&testTokenVerifier{principal: principal}),
		ArtifactStore:     store,
		ArtifactPublisher: &stubArtifactPublisher{},
	})
	req := newMTLSRequest(t, http.MethodDelete, "/v1/artifacts/artifact-delete", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var body map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode delete response: %v", err)
	}
	artifact := body["artifact"].(map[string]any)
	if artifact["id"].(string) != "artifact-delete" {
		t.Fatalf("unexpected artifact id: %q", artifact["id"])
	}
	if artifact["deleted_at"].(string) == "" {
		t.Fatalf("expected deleted_at timestamp")
	}
	if _, err := store.Get(context.Background(), "artifact-delete"); !errors.Is(err, controlplaneartifacts.ErrNotFound) {
		t.Fatalf("expected ErrNotFound after delete, got %v", err)
	}
}

func TestArtifactsDownloadStreamsPayload(t *testing.T) {
	t.Parallel()

	etcd, client := startTestEtcd(t)
	defer etcd.Close()
	defer func() { _ = client.Close() }()

	store, err := controlplaneartifacts.NewStore(client, controlplaneartifacts.StoreOptions{})
	if err != nil {
		t.Fatalf("new artifact store: %v", err)
	}
	publisher := &stubArtifactPublisher{
		payloads: map[string]storedPayload{
			"bafydata": {data: []byte("artifact-bytes"), digest: "sha256:data"},
		},
	}
	if _, err := store.Create(context.Background(), controlplaneartifacts.Metadata{
		ID:     "artifact-download",
		JobID:  "job-download",
		CID:    "bafydata",
		Digest: "sha256:data",
		Size:   14,
	}); err != nil {
		t.Fatalf("seed artifact: %v", err)
	}

	principal := newTestPrincipal([]string{security.ScopeArtifactsRead})
	handler := newTestControlPlaneHandler(t, httpserver.ControlPlaneOptions{
		Etcd:              client,
		Auth:              security.NewManager(&testTokenVerifier{principal: principal}),
		ArtifactStore:     store,
		ArtifactPublisher: publisher,
	})

	req := newMTLSRequest(t, http.MethodGet, "/v1/artifacts/artifact-download?download=1", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct == "" {
		t.Fatalf("expected content type, got empty")
	}
	if rec.Body.String() != "artifact-bytes" {
		t.Fatalf("unexpected payload: %q", rec.Body.String())
	}
}
