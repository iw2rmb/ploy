package arf

import (
    "archive/zip"
    "bytes"
    "net/http/httptest"
    "testing"

    "github.com/gofiber/fiber/v2"
    "context"
)

// helper to build an in-memory JAR with META-INF/rewrite/*.yml entries
func buildTestJar(entries map[string][]byte) []byte {
    var buf bytes.Buffer
    zw := zip.NewWriter(&buf)
    for name, data := range entries {
        f, _ := zw.Create(name)
        f.Write(data)
    }
    zw.Close()
    return buf.Bytes()
}

type mockFetcher struct{
    jars map[string][]byte
}

func (m *mockFetcher) Fetch(pack, version string) ([]byte, error) {
    return m.jars[pack+":"+version], nil
}

type memStorage struct{
    put map[string][]byte
}

func (m *memStorage) Put(_ context.Context, key string, data []byte) error {
    if m.put == nil { m.put = make(map[string][]byte) }
    m.put[key] = append([]byte(nil), data...)
    return nil
}
func (m *memStorage) Get(_ context.Context, key string) ([]byte, error) { return m.put[key], nil }
func (m *memStorage) Delete(_ context.Context, key string) error { delete(m.put, key); return nil }
func (m *memStorage) Exists(_ context.Context, key string) (bool, error) { _, ok := m.put[key]; return ok, nil }

func TestRecipesIndexer_RefreshAndPersist(t *testing.T) {
    // Prepare a fake jar for rewrite-java:2.20.0
    jar := buildTestJar(map[string][]byte{
        "META-INF/rewrite/java.yml": sampleRecipeYAML1,
        "META-INF/rewrite/migrate.yml": sampleRecipeYAML2,
    })
    fetcher := &mockFetcher{jars: map[string][]byte{"rewrite-java:2.20.0": jar}}
    storage := &memStorage{}

    // Build handler with empty catalog and indexer wired
    cat := NewRecipesCatalog()
    idx := NewRecipesIndexer(fetcher, storage)
    app := fiber.New()
    rh := NewRecipesHandler(cat)
    rh.SetIndexer(idx)
    app.Post("/v1/arf/recipes/refresh", rh.RefreshRecipes)
    app.Get("/v1/arf/recipes", rh.ListRecipes)

    // Call refresh
    req := httptest.NewRequest("POST", "/v1/arf/recipes/refresh", nil)
    resp, err := app.Test(req, -1)
    if err != nil { t.Fatalf("refresh request failed: %v", err) }
    if resp.StatusCode != 200 { t.Fatalf("expected 200, got %d", resp.StatusCode) }

    // List should now show 2 recipes
    req2 := httptest.NewRequest("GET", "/v1/arf/recipes", nil)
    resp2, err := app.Test(req2, -1)
    if err != nil { t.Fatalf("list request failed: %v", err) }
    if resp2.StatusCode != 200 { t.Fatalf("expected 200, got %d", resp2.StatusCode) }

    // Persistence should have written snapshot
    if ok, _ := storage.Exists(nil, "artifacts/openrewrite/catalog.json"); !ok {
        t.Fatalf("expected catalog snapshot to be persisted")
    }
}
