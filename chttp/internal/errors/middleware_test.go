package errors

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestErrorMiddleware_Integration(t *testing.T) {
	app := fiber.New()
	
	// Add our error middleware
	middleware := NewErrorMiddleware(ErrorMiddlewareConfig{
		EnableStackTrace: false,
		EnableLogging:    false,
		LogLevel:        "error",
		IncludeDetails:   true,
	})
	app.Use(middleware)
	
	// Test route that returns a CHTTP error
	app.Get("/chttp-error", func(c *fiber.Ctx) error {
		return NewError(ErrorTypeValidation, "test validation error", nil).
			WithField("field", "email").
			WithField("value", "invalid")
	})
	
	// Test route that returns a regular error  
	app.Get("/regular-error", func(c *fiber.Ctx) error {
		return fiber.NewError(400, "regular fiber error")
	})
	
	// Test route that panics
	app.Get("/panic", func(c *fiber.Ctx) error {
		panic("test panic")
	})
	
	// Test CHTTP error
	req, _ := http.NewRequest(http.MethodGet, "/chttp-error", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	
	assert.Equal(t, 400, resp.StatusCode)
	
	var result map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&result)
	require.NoError(t, err)
	
	assert.Equal(t, "error", result["status"])
	assert.Equal(t, "validation", result["type"])
	assert.Equal(t, "test validation error", result["message"])
	
	// Test regular error
	req, _ = http.NewRequest(http.MethodGet, "/regular-error", nil)
	resp, err = app.Test(req)
	require.NoError(t, err)
	
	assert.Equal(t, 400, resp.StatusCode)
	
	result = make(map[string]interface{})
	err = json.NewDecoder(resp.Body).Decode(&result)
	require.NoError(t, err)
	
	assert.Equal(t, "error", result["status"])
	assert.Equal(t, "validation", result["type"])
	assert.Equal(t, "regular fiber error", result["message"])
	
	// Test panic recovery
	req, _ = http.NewRequest(http.MethodGet, "/panic", nil)
	resp, err = app.Test(req)
	require.NoError(t, err)
	
	assert.Equal(t, 500, resp.StatusCode)
	
	result = make(map[string]interface{})
	err = json.NewDecoder(resp.Body).Decode(&result)
	require.NoError(t, err)
	
	assert.Equal(t, "error", result["status"])
	assert.Equal(t, "internal", result["type"])
	assert.Equal(t, "test panic", result["message"])
}