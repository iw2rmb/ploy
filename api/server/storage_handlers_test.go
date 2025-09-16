package server

import (
	"context"
	"encoding/json"
	"io"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
	cfgsvc "github.com/iw2rmb/ploy/internal/config"
	istorage "github.com/iw2rmb/ploy/internal/storage"
)

// fakeStorage implements internal/storage.Storage for handler tests
type fakeStorage struct {
	healthy bool
	metrics *istorage.StorageMetrics
}

func (f *fakeStorage) Get(ctx context.Context, key string) (io.ReadCloser, error) { return nil, nil }
func (f *fakeStorage) Put(ctx context.Context, key string, reader io.Reader, opts ...istorage.PutOption) error {
	return nil
}
func (f *fakeStorage) Delete(ctx context.Context, key string) error         { return nil }
func (f *fakeStorage) Exists(ctx context.Context, key string) (bool, error) { return false, nil }
func (f *fakeStorage) List(ctx context.Context, opts istorage.ListOptions) ([]istorage.Object, error) {
	return nil, nil
}
func (f *fakeStorage) DeleteBatch(ctx context.Context, keys []string) error { return nil }
func (f *fakeStorage) Head(ctx context.Context, key string) (*istorage.Object, error) {
	return nil, nil
}
func (f *fakeStorage) UpdateMetadata(ctx context.Context, key string, metadata map[string]string) error {
	return nil
}
func (f *fakeStorage) Copy(ctx context.Context, src, dst string) error { return nil }
func (f *fakeStorage) Move(ctx context.Context, src, dst string) error { return nil }
func (f *fakeStorage) Health(ctx context.Context) error {
	if f.healthy {
		return nil
	}
	return io.EOF
}
func (f *fakeStorage) Metrics() *istorage.StorageMetrics { return f.metrics }

// override resolveStorageFromConfigService via a test helper variable if available
// Otherwise, inject through Server.getStorageClient by replacing method on receiver via embedding not feasible.

func TestHandleStorageHealth_Healthy(t *testing.T) {
	s := &Server{dependencies: &ServiceDependencies{}}
	// Monkey patch resolveStorageFromConfigService via package-level var by wrapping helper
	old := resolveStorageFromConfigService
	resolveStorageFromConfigService = func(_ *cfgsvc.Service) (istorage.Storage, error) {
		return &fakeStorage{healthy: true, metrics: istorage.NewStorageMetrics()}, nil
	}
	defer func() { resolveStorageFromConfigService = old }()

	app := fiber.New()
	app.Get("/v1/storage/health", s.handleStorageHealth)
	req := httptest.NewRequest("GET", "/v1/storage/health", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	var body map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["status"] != "healthy" {
		t.Fatalf("expected healthy, got %#v", body["status"])
	}
}

func TestHandleStorageHealth_Unhealthy(t *testing.T) {
	s := &Server{dependencies: &ServiceDependencies{}}
	old := resolveStorageFromConfigService
	resolveStorageFromConfigService = func(_ *cfgsvc.Service) (istorage.Storage, error) {
		return &fakeStorage{healthy: false, metrics: istorage.NewStorageMetrics()}, nil
	}
	defer func() { resolveStorageFromConfigService = old }()

	app := fiber.New()
	app.Get("/v1/storage/health", s.handleStorageHealth)
	req := httptest.NewRequest("GET", "/v1/storage/health", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode != 503 {
		t.Fatalf("expected 503, got %d", resp.StatusCode)
	}
}

func TestHandleStorageMetrics(t *testing.T) {
	s := &Server{dependencies: &ServiceDependencies{}}
	metrics := istorage.NewStorageMetrics()
	metrics.RecordUpload(true, 0, 123, istorage.ErrorType(""))
	old := resolveStorageFromConfigService
	resolveStorageFromConfigService = func(_ *cfgsvc.Service) (istorage.Storage, error) {
		return &fakeStorage{healthy: true, metrics: metrics}, nil
	}
	defer func() { resolveStorageFromConfigService = old }()

	app := fiber.New()
	app.Get("/v1/storage/metrics", s.handleStorageMetrics)
	req := httptest.NewRequest("GET", "/v1/storage/metrics", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	var snap istorage.StorageMetrics
	if err := json.NewDecoder(resp.Body).Decode(&snap); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if snap.SuccessfulUploads != 1 {
		t.Fatalf("expected SuccessfulUploads=1, got %d", snap.SuccessfulUploads)
	}
}
