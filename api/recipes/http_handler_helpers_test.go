package recipes

import (
    "bytes"
    "context"
    "io"
    "testing"

    "github.com/gofiber/fiber/v2"
    "github.com/iw2rmb/ploy/internal/storage"
)

func newAppWithHandler(h *HTTPHandler) *fiber.App {
    app := fiber.New()
    h.RegisterRoutes(app)
    return app
}

// deletableProvider implements minimal StorageProvider with Delete support for tests
type deletableProvider struct{ data map[string][]byte }

func newDeletableProvider() *deletableProvider { return &deletableProvider{data: map[string][]byte{}} }

func (p *deletableProvider) PutObject(_ string, key string, body io.ReadSeeker, _ string) (*storage.PutObjectResult, error) {
    b, _ := io.ReadAll(body)
    p.data[key] = b
    return &storage.PutObjectResult{Location: key, Size: int64(len(b))}, nil
}
func (p *deletableProvider) GetObject(_ string, key string) (io.ReadCloser, error) {
    if b, ok := p.data[key]; ok { return io.NopCloser(bytes.NewReader(b)), nil }
    return nil, io.EOF
}
func (p *deletableProvider) VerifyUpload(string) error { return nil }
func (p *deletableProvider) ListObjects(_ string, prefix string) ([]storage.ObjectInfo, error) {
    out := make([]storage.ObjectInfo, 0, len(p.data))
    for k, v := range p.data { if prefix == "" || (len(k) >= len(prefix) && k[:len(prefix)] == prefix) { out = append(out, storage.ObjectInfo{Key:k, Size:int64(len(v))}) } }
    return out, nil
}
func (p *deletableProvider) GetProviderType() string    { return "seaweedfs" }
func (p *deletableProvider) GetArtifactsBucket() string { return "ploy-recipes" }
func (p *deletableProvider) UploadArtifactBundle(keyPrefix, artifactPath string) error { return nil }
func (p *deletableProvider) UploadArtifactBundleWithVerification(keyPrefix, artifactPath string) (*storage.BundleIntegrityResult, error) { return &storage.BundleIntegrityResult{Verified: true}, nil }
func (p *deletableProvider) Delete(_ context.Context, key string) error { delete(p.data, key); return nil }

func setupRecipesHTTPHandlerWithDeletable(t *testing.T) *HTTPHandler {
    t.Helper()
    prov := newDeletableProvider()
    registry := NewRecipeRegistry(prov)
    storageAdapter := NewRegistryStorageAdapter(registry)
    return NewHTTPHandlerWithStorage(storageAdapter, nil, nil, prov, registry)
}

