package api

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	
	"github.com/iw2rmb/ploy/services/cllm/internal/providers"
	"github.com/iw2rmb/ploy/services/cllm/internal/sandbox"
)

func createTestHandlers(t *testing.T) *Handlers {
	// Create mock provider manager with mock provider
	providerConfigs := []providers.ProviderConfig{
		{
			Type:    "mock",
			BaseURL: "http://localhost",
			Model:   "test-model",
		},
	}
	
	providerManager, err := providers.NewProviderManager(providerConfigs)
	require.NoError(t, err)
	
	// Create mock sandbox manager with minimal config
	sandboxManager, err := sandbox.NewManager(sandbox.ManagerConfig{
		WorkDir:        "/tmp",
		MaxMemory:      "100MB",
		MaxCPUTime:     "30s",
		MaxProcesses:   10,
		CleanupTimeout: "5s",
	})
	require.NoError(t, err)
	
	return NewHandlers(providerManager, sandboxManager)
}

func TestHealthHandler(t *testing.T) {
	app := fiber.New()
	
	// This will fail until we implement the handler
	handler := createTestHandlers(t)
	app.Get("/health", handler.Health)
	
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	resp, err := app.Test(req)
	
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	
	// Check response body
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	
	var response map[string]interface{}
	err = json.Unmarshal(body, &response)
	require.NoError(t, err)
	
	assert.Equal(t, "healthy", response["status"])
	assert.Contains(t, response, "timestamp")
}

func TestReadyHandler(t *testing.T) {
	app := fiber.New()
	
	handler := createTestHandlers(t)
	app.Get("/ready", handler.Ready)
	
	req := httptest.NewRequest(http.MethodGet, "/ready", nil)
	resp, err := app.Test(req)
	
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	
	var response map[string]interface{}
	err = json.Unmarshal(body, &response)
	require.NoError(t, err)
	
	assert.Equal(t, "ready", response["status"])
}

func TestVersionHandler(t *testing.T) {
	app := fiber.New()
	
	handler := createTestHandlers(t)
	app.Get("/version", handler.Version)
	
	req := httptest.NewRequest(http.MethodGet, "/version", nil)
	resp, err := app.Test(req)
	
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	
	var response map[string]interface{}
	err = json.Unmarshal(body, &response)
	require.NoError(t, err)
	
	assert.Contains(t, response, "version")
	assert.Contains(t, response, "service")
	assert.Equal(t, "cllm", response["service"])
}

func TestAnalyzeHandler_ValidationError(t *testing.T) {
	app := fiber.New()
	
	handler := createTestHandlers(t)
	app.Post("/v1/analyze", handler.Analyze)
	
	// Send request with no body - should fail validation
	req := httptest.NewRequest(http.MethodPost, "/v1/analyze", nil)
	resp, err := app.Test(req)
	
	require.NoError(t, err)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestTransformHandler_ValidationError(t *testing.T) {
	app := fiber.New()
	
	handler := createTestHandlers(t)
	app.Post("/v1/transform", handler.Transform)
	
	// Send request with no body - should fail validation
	req := httptest.NewRequest(http.MethodPost, "/v1/transform", nil)
	resp, err := app.Test(req)
	
	require.NoError(t, err)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestMiddleware_CORS(t *testing.T) {
	app := fiber.New()
	
	// This will fail until we implement CORS middleware
	SetupMiddleware(app)
	
	app.Get("/test", func(c *fiber.Ctx) error {
		return c.SendString("ok")
	})
	
	req := httptest.NewRequest(http.MethodOptions, "/test", nil)
	req.Header.Set("Origin", "http://localhost:3000")
	req.Header.Set("Access-Control-Request-Method", "GET")
	
	resp, err := app.Test(req)
	require.NoError(t, err)
	
	assert.Equal(t, http.StatusNoContent, resp.StatusCode)
	assert.Contains(t, resp.Header.Get("Access-Control-Allow-Origin"), "*")
}

func TestMiddleware_RequestLogging(t *testing.T) {
	app := fiber.New()
	
	SetupMiddleware(app)
	
	app.Get("/test", func(c *fiber.Ctx) error {
		return c.SendString("ok")
	})
	
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	resp, err := app.Test(req)
	
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	
	// Check that request ID header is set
	assert.NotEmpty(t, resp.Header.Get("X-Request-ID"))
}