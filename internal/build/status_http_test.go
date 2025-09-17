package build

import (
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStatus_ValidNameButNoActiveJob(t *testing.T) {
	t.Skip("Requires Nomad API; skip in unit tests to avoid timeouts")
	app := fiber.New()
	app.Get("/apps/:app/status", Status)

	// valid app name to pass validation; with no Nomad, expect 404 not found
	req := httptest.NewRequest("GET", "/apps/valid-app/status", nil)
	resp, err := app.Test(req, 10000)
	require.NoError(t, err)
	assert.Equal(t, 404, resp.StatusCode)
}
