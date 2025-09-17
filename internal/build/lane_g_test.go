package build

import (
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestBuildLaneG_UploadsWasm(t *testing.T) {
	dir := t.TempDir()
	wasm := filepath.Join(dir, "module.wasm")
	require.NoError(t, os.WriteFile(wasm, []byte("wasm"), 0644))

	mockStorage := new(MockUnifiedStorage)
	// Expect a Put with the computed key
	mockStorage.On("Put", mock.Anything, mock.MatchedBy(func(key string) bool {
		return key == "builds/app/sha/module.wasm"
	}), mock.Anything, mock.Anything).Return(nil)

	deps := &BuildDependencies{Storage: mockStorage}

	app := fiber.New()
	app.Post("/g", func(c *fiber.Ctx) error {
		p, err := buildLaneG(c, deps, "app", dir, "sha")
		if err != nil {
			return err
		}
		return c.JSON(fiber.Map{"path": p})
	})
	req := httptest.NewRequest("POST", "/g", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	t.Cleanup(func() {
		if resp != nil && resp.Body != nil {
			_ = resp.Body.Close()
		}
	})
	require.Equal(t, 200, resp.StatusCode)
	mockStorage.AssertExpectations(t)
}
