package build

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestListApps(t *testing.T) {
	app := fiber.New()
	app.Get("/apps", ListApps)

	req := httptest.NewRequest("GET", "/apps", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, 200, resp.StatusCode)

	var response map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&response)
	require.NoError(t, err)

	apps, ok := response["apps"]
	assert.True(t, ok)
	
	// Should return empty array
	appsArray, ok := apps.([]interface{})
	assert.True(t, ok)
	assert.Empty(t, appsArray)
}