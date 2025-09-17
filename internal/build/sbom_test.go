package build

import (
	"net/http/httptest"
	"os"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/require"
)

func TestSBOMFeatureEnabled_QueryOverride(t *testing.T) {
	app := fiber.New()
	t.Cleanup(func() { _ = app.Shutdown() })
	// default env allows SBOM
	t.Setenv("PLOY_SBOM_ENABLED", "true")

	var got bool
	app.Get("/", func(c *fiber.Ctx) error {
		got = sbomFeatureEnabled(c)
		return nil
	})
	req := httptest.NewRequest("GET", "/?sbom=false", nil)
	resp1, err := app.Test(req)
	require.NoError(t, err)
	t.Cleanup(func() {
		if resp1 != nil && resp1.Body != nil {
			_ = resp1.Body.Close()
		}
	})
	require.False(t, got)
}

func TestSBOMFeatureEnabled_EnvToggle(t *testing.T) {
	app := fiber.New()
	t.Cleanup(func() { _ = app.Shutdown() })
	require.NoError(t, os.Unsetenv("PLOY_SBOM_ENABLED"))

	var got bool
	app.Get("/", func(c *fiber.Ctx) error {
		got = sbomFeatureEnabled(c)
		return nil
	})
	req := httptest.NewRequest("GET", "/", nil)
	resp2, err := app.Test(req)
	require.NoError(t, err)
	t.Cleanup(func() {
		if resp2 != nil && resp2.Body != nil {
			_ = resp2.Body.Close()
		}
	})
	require.True(t, got, "default should be enabled when env not set")

	t.Setenv("PLOY_SBOM_ENABLED", "off")
	req = httptest.NewRequest("GET", "/", nil)
	resp3, err := app.Test(req)
	require.NoError(t, err)
	t.Cleanup(func() {
		if resp3 != nil && resp3.Body != nil {
			_ = resp3.Body.Close()
		}
	})
	require.False(t, got)
}

func TestSBOMFailOnError_Toggles(t *testing.T) {
	require.NoError(t, os.Unsetenv("PLOY_SBOM_FAIL_ON_ERROR"))
	require.False(t, sbomFailOnError())
	t.Setenv("PLOY_SBOM_FAIL_ON_ERROR", "1")
	require.True(t, sbomFailOnError())
}
