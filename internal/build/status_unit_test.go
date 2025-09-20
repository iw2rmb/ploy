package build

import (
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStatus_InvalidAppName(t *testing.T) {
	app := fiber.New()
	app.Get("/apps/:app/status", Status)

	req := httptest.NewRequest("GET", "/apps/"+url.PathEscape("invalid app")+"/status", nil)
	resp, err := app.Test(req, 5000)
	require.NoError(t, err)
	assert.Equal(t, 400, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
}
