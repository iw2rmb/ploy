package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
	modsapi "github.com/iw2rmb/ploy/api/mods"
	cfgsvc "github.com/iw2rmb/ploy/internal/config"
	"github.com/iw2rmb/ploy/internal/envstore"
	"github.com/iw2rmb/ploy/internal/git/provider"
	"github.com/iw2rmb/ploy/internal/orchestration"
	"github.com/iw2rmb/ploy/internal/storage"
)

// fakeGitProvider satisfies provider.GitProvider for route wiring tests.
type fakeGitProvider struct{}

func (fakeGitProvider) CreateOrUpdateMR(ctx context.Context, config provider.MRConfig) (*provider.MRResult, error) {
	return nil, errors.New("not implemented")
}

func (fakeGitProvider) ValidateConfiguration() error { return nil }

// noopStorage implements storage.Storage with no-ops for handler wiring tests.
type noopStorage struct{}

func (noopStorage) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	return io.NopCloser(strings.NewReader("")), nil
}

func (noopStorage) Put(ctx context.Context, key string, reader io.Reader, opts ...storage.PutOption) error {
	return nil
}

func (noopStorage) Delete(ctx context.Context, key string) error { return nil }

func (noopStorage) Exists(ctx context.Context, key string) (bool, error) { return false, nil }

func (noopStorage) List(ctx context.Context, opts storage.ListOptions) ([]storage.Object, error) {
	return nil, nil
}

func (noopStorage) DeleteBatch(ctx context.Context, keys []string) error { return nil }

func (noopStorage) Head(ctx context.Context, key string) (*storage.Object, error) { return nil, nil }

func (noopStorage) UpdateMetadata(ctx context.Context, key string, metadata map[string]string) error {
	return nil
}

func (noopStorage) Copy(ctx context.Context, src, dst string) error { return nil }

func (noopStorage) Move(ctx context.Context, src, dst string) error { return nil }

func (noopStorage) Health(ctx context.Context) error { return nil }

func (noopStorage) Metrics() *storage.StorageMetrics { return &storage.StorageMetrics{} }

// inMemoryKV is a lightweight orchestration.KV implementation for tests.
type inMemoryKV struct{ data map[string][]byte }

var _ orchestration.KV = (*inMemoryKV)(nil)

func newInMemoryKV() *inMemoryKV { return &inMemoryKV{data: make(map[string][]byte)} }

func (kv *inMemoryKV) Put(key string, value []byte) error {
	kv.data[key] = append([]byte(nil), value...)
	return nil
}

func (kv *inMemoryKV) Get(key string) ([]byte, error) {
	v, ok := kv.data[key]
	if !ok {
		return nil, nil
	}
	return append([]byte(nil), v...), nil
}

func (kv *inMemoryKV) Keys(prefix, separator string) ([]string, error) {
	keys := make([]string, 0, len(kv.data))
	for k := range kv.data {
		if prefix == "" || strings.HasPrefix(k, prefix) {
			keys = append(keys, k)
		}
	}
	return keys, nil
}

func (kv *inMemoryKV) Delete(key string) error {
	delete(kv.data, key)
	return nil
}

func TestSetupRoutes_RegistersModsEndpoints(t *testing.T) {
	s := createMockServer()
	s.dependencies.ModsHandler = modsapi.NewHandler(fakeGitProvider{}, noopStorage{}, newInMemoryKV())

	s.setupRoutes()

	routes := s.app.GetRoutes(false)
	found := false
	for _, r := range routes {
		if r.Method == fiber.MethodPost && r.Path == "/v1/mods" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected POST /v1/mods route to be registered")
	}
}

func TestHandleTriggerAppBuild_StorageResolutionFailure(t *testing.T) {
	original := resolveStorageFromConfigService
	resolveStorageFromConfigService = func(_ *cfgsvc.Service) (storage.Storage, error) {
		return nil, errors.New("boom")
	}
	t.Cleanup(func() { resolveStorageFromConfigService = original })

	s := createMockServer()
	s.app = fiber.New()
	s.app.Post("/v1/apps/:app/builds", s.handleTriggerAppBuild)

	req := httptest.NewRequest("POST", "/v1/apps/demo/builds", bytes.NewBufferString("payload"))
	req.Header.Set("Content-Type", "application/x-tar")

	resp, err := s.app.Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != fiber.StatusServiceUnavailable {
		t.Fatalf("expected status %d, got %d", fiber.StatusServiceUnavailable, resp.StatusCode)
	}

	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if errMsg, ok := body["error"].(string); !ok || errMsg == "" {
		t.Fatalf("expected error field in response body: %#v", body)
	}
}

func TestHandleTriggerAppBuild_InvalidAppName(t *testing.T) {
	original := resolveStorageFromConfigService
	resolveStorageFromConfigService = func(_ *cfgsvc.Service) (storage.Storage, error) {
		return noopStorage{}, nil
	}
	t.Cleanup(func() { resolveStorageFromConfigService = original })

	s := createMockServer()
	s.dependencies.EnvStore = envstore.New(t.TempDir())
	s.app = fiber.New()
	s.app.Post("/v1/apps/:app/builds", s.handleTriggerAppBuild)

	req := httptest.NewRequest("POST", "/v1/apps/INVALID!/builds", bytes.NewReader([]byte("tar")))
	req.Header.Set("Content-Type", "application/x-tar")

	resp, err := s.app.Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != fiber.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", fiber.StatusBadRequest, resp.StatusCode)
	}

	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	errMsg, ok := body["error"].(string)
	if !ok {
		t.Fatalf("expected error string in body: %#v", body)
	}
	if !strings.Contains(strings.ToLower(errMsg), "invalid") {
		t.Fatalf("expected invalid app error, got %#v", body)
	}
}

func TestHandleAppProbe_NoEndpointFound(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	}))
	t.Cleanup(ts.Close)
	t.Setenv("NOMAD_ADDR", ts.URL)
	t.Setenv("NOMAD_HTTP_MAX_RETRIES", "0")
	t.Setenv("NOMAD_HTTP_BASE_DELAY", "0s")
	t.Setenv("NOMAD_HTTP_MAX_DELAY", "0s")

	s := createMockServer()
	s.app = fiber.New()
	s.app.Get("/v1/apps/:app/probe", s.handleAppProbe)

	resp, err := s.app.Test(httptest.NewRequest("GET", "/v1/apps/demo/probe", nil), 3000)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != fiber.StatusNotFound {
		t.Fatalf("expected status %d, got %d", fiber.StatusNotFound, resp.StatusCode)
	}

	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	errMsg, ok := body["error"].(string)
	if !ok {
		t.Fatalf("expected error string in body: %#v", body)
	}
	if !strings.Contains(errMsg, "no running") {
		t.Fatalf("unexpected error message: %#v", body)
	}
}

func TestHandleTriggerPlatformBuild_StorageResolutionFailure(t *testing.T) {
	original := resolveStorageFromConfigService
	resolveStorageFromConfigService = func(_ *cfgsvc.Service) (storage.Storage, error) {
		return nil, errors.New("kaput")
	}
	t.Cleanup(func() { resolveStorageFromConfigService = original })

	s := createMockServer()
	s.app = fiber.New()
	s.app.Post("/v1/platform/:service/builds", s.handleTriggerPlatformBuild)

	req := httptest.NewRequest("POST", "/v1/platform/queue/builds", bytes.NewBufferString("payload"))
	req.Header.Set("Content-Type", "application/x-tar")

	resp, err := s.app.Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != fiber.StatusServiceUnavailable {
		t.Fatalf("expected status %d, got %d", fiber.StatusServiceUnavailable, resp.StatusCode)
	}
}

func TestHandleBuildsOptionsHeaders(t *testing.T) {
	s := createMockServer()
	s.app = fiber.New()
	s.app.Options("/v1/apps/:app/builds", s.handleBuildsOptions)

	resp, err := s.app.Test(httptest.NewRequest("OPTIONS", "/v1/apps/demo/builds", nil))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != fiber.StatusNoContent {
		t.Fatalf("expected status %d, got %d", fiber.StatusNoContent, resp.StatusCode)
	}

	if v := resp.Header.Get("Allow"); v != "POST, OPTIONS" {
		t.Fatalf("unexpected Allow header: %q", v)
	}
	if v := resp.Header.Get("Access-Control-Allow-Methods"); v != "POST, OPTIONS" {
		t.Fatalf("unexpected Access-Control-Allow-Methods header: %q", v)
	}
}
