package build

import (
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/require"
)

func TestBuildLaneAB_CreatesImage(t *testing.T) {
	tmp := t.TempDir()
	app := fiber.New()
	app.Post("/a", func(c *fiber.Ctx) error {
		img, err := buildLaneAB(c, &BuildDependencies{}, "app", "A", tmp, "sha", tmp, map[string]string{})
		if err != nil {
			return err
		}
		return c.JSON(fiber.Map{"path": img})
	})
	req := httptest.NewRequest("POST", "/a", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	t.Cleanup(func() {
		if resp != nil && resp.Body != nil {
			_ = resp.Body.Close()
		}
	})
	require.Equal(t, 200, resp.StatusCode)
	// Verify file exists in tmp (final.img)
	p := filepath.Join(tmp, "final.img")
	_, statErr := os.Stat(p)
	require.NoError(t, statErr)
}
