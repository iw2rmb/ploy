package build

import (
	"bytes"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/iw2rmb/ploy/internal/config"
	envstore "github.com/iw2rmb/ploy/internal/envstore"
	"github.com/iw2rmb/ploy/internal/testing/mocks"
	"github.com/stretchr/testify/require"
)

// Test that lane C surfaces storage-unavailable error path without calling external builders.
func TestLaneC_NoStorageReturns500(t *testing.T) {
	mockEnvStore := mocks.NewEnvStore()
	mockEnvStore.On("GetAll", "javasvc").Return(envstore.AppEnvVars{}, nil)

	deps := &BuildDependencies{EnvStore: mockEnvStore}
	buildCtx := &BuildContext{APIContext: "apps", AppType: config.UserApp}

	app := fiber.New()
	app.Post("/build/:app", func(c *fiber.Ctx) error { return triggerBuildWithDependencies(c, deps, buildCtx) })

	// Provide a simple tarball to unpack as source
	tarBody := createTestTarball(t, map[string]string{"Main.java": "class Main {}"})
	req := httptest.NewRequest("POST", "/build/javasvc?lane=C&sha=dev", bytes.NewReader(tarBody))
	req.Header.Set("Content-Type", "application/x-tar")
	resp, err := app.Test(req, 30000)
	require.NoError(t, err)
	require.Equal(t, 500, resp.StatusCode)
}
