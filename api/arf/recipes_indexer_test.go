package arf

import (
	"archive/zip"
	"bytes"
	"net/http/httptest"
	"testing"
	"time"

	"context"

	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

type mockFetcher struct {
	jars map[string][]byte
}

func (m *mockFetcher) Fetch(pack, version string) ([]byte, error) {
	return m.jars[pack+":"+version], nil
}

type memStorage struct {
	put map[string][]byte
}

func (m *memStorage) Put(_ context.Context, key string, data []byte) error {
	if m.put == nil {
		m.put = make(map[string][]byte)
	}
	m.put[key] = append([]byte(nil), data...)
	return nil
}
func (m *memStorage) Get(_ context.Context, key string) ([]byte, error) { return m.put[key], nil }
func (m *memStorage) Delete(_ context.Context, key string) error        { delete(m.put, key); return nil }
func (m *memStorage) Exists(_ context.Context, key string) (bool, error) {
	_, ok := m.put[key]
	return ok, nil
}

func TestRecipesIndexer_RefreshAndPersist(t *testing.T) {
	// Prepare a fake jar for rewrite-java:2.20.0
	jar := buildTestJar(map[string][]byte{
		"META-INF/rewrite/java.yml":    sampleRecipeYAML1,
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
	if err != nil {
		t.Fatalf("refresh request failed: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	// List should now show 2 recipes
	req2 := httptest.NewRequest("GET", "/v1/arf/recipes", nil)
	resp2, err := app.Test(req2, -1)
	if err != nil {
		t.Fatalf("list request failed: %v", err)
	}
	if resp2.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp2.StatusCode)
	}

	// Persistence should have written snapshot
	if ok, _ := storage.Exists(nil, "artifacts/openrewrite/catalog.json"); !ok {
		t.Fatalf("expected catalog snapshot to be persisted")
	}
}

// Test that indexer logs catalog size and index time
func TestRecipesIndexer_LogsCatalogMetrics(t *testing.T) {
	// Capture logs
	var logBuffer bytes.Buffer

	// Prepare test data
	jar := buildTestJar(map[string][]byte{
		"META-INF/rewrite/java.yml":    sampleRecipeYAML1,
		"META-INF/rewrite/migrate.yml": sampleRecipeYAML2,
	})
	fetcher := &mockFetcher{jars: map[string][]byte{"rewrite-java:2.20.0": jar}}
	storage := &memStorage{}

	// Create indexer with logging
	idx := NewRecipesIndexer(fetcher, storage)
	idx.SetLogger(&logBuffer) // Assuming we'll add this method

	// Run refresh
	ctx := context.Background()
	startTime := time.Now()
	catalog, err := idx.Refresh(ctx, []PackSpec{{Name: "rewrite-java", Version: "2.20.0"}})
	duration := time.Since(startTime)

	require.NoError(t, err)
	require.NotNil(t, catalog)

	// Check logs contain expected information
	logs := logBuffer.String()
	assert.Contains(t, logs, "catalog_size", "Should log catalog size")
	assert.Contains(t, logs, "index_time", "Should log indexing time")
	assert.Contains(t, logs, "2", "Should log correct number of recipes")

	// Verify timing is reasonable
	assert.Less(t, duration, 5*time.Second, "Indexing should complete quickly")
}

// Test metrics collection for catalog operations
func TestRecipesIndexer_CollectsMetrics(t *testing.T) {
	// Mock metrics collector
	metrics := &MockMetricsCollector{}

	// Prepare test data
	jar := buildTestJar(map[string][]byte{
		"META-INF/rewrite/java.yml": sampleRecipeYAML1,
	})
	fetcher := &mockFetcher{jars: map[string][]byte{"rewrite-java:2.20.0": jar}}
	storage := &memStorage{}

	// Create indexer with metrics
	idx := NewRecipesIndexer(fetcher, storage)
	idx.SetMetricsCollector(metrics) // Assuming we'll add this method

	// Run refresh
	ctx := context.Background()
	_, err := idx.Refresh(ctx, []PackSpec{{Name: "rewrite-java", Version: "2.20.0"}})
	require.NoError(t, err)

	// Verify metrics were collected
	assert.True(t, metrics.HasMetric("catalog.refresh.duration"), "Should record refresh duration")
	assert.True(t, metrics.HasMetric("catalog.recipe.count"), "Should record recipe count")
	assert.True(t, metrics.HasMetric("catalog.pack.count"), "Should record pack count")
}

// MockMetricsCollector for testing
type MockMetricsCollector struct {
	metrics map[string]interface{}
}

func (m *MockMetricsCollector) RecordDuration(name string, duration time.Duration) {
	if m.metrics == nil {
		m.metrics = make(map[string]interface{})
	}
	m.metrics[name] = duration
}

func (m *MockMetricsCollector) RecordCount(name string, count int) {
	if m.metrics == nil {
		m.metrics = make(map[string]interface{})
	}
	m.metrics[name] = count
}

func (m *MockMetricsCollector) HasMetric(name string) bool {
	_, ok := m.metrics[name]
	return ok
}
