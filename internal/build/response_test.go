package build

import (
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/iw2rmb/ploy/internal/config"
	"github.com/stretchr/testify/require"
)

func TestMakeBuildResponse_BuilderFieldsLaneE(t *testing.T) {
	start := time.Now().Add(-2 * time.Second)
	resp := makeBuildResponse(
		"E",
		"/opt/ploy/artifacts/app.img",
		"",
		"apps",
		config.UserApp,
		start,
		nil,
		42.5,
		"app-e-build-123",
		"app",
		"123",
		"registry.dev",
		"project",
		nil,
		nil,
	)
	require.Contains(t, resp, "builder")
	b := resp["builder"].(fiber.Map)
	require.Equal(t, "app-e-build-123", b["job"])
	// imageSize always present
	require.Contains(t, resp, "imageSize")
}

func TestMakeBuildResponse_BuilderFieldsLaneC(t *testing.T) {
	start := time.Now().Add(-1 * time.Second)
	resp := makeBuildResponse(
		"C",
		"/opt/ploy/artifacts/app-osv.qemu",
		"",
		"platform",
		config.PlatformApp,
		start,
		nil,
		10,
		"ignored",
		"ordersvc",
		"deadbeef",
		"registry.dev",
		"project",
		nil,
		nil,
	)
	require.Contains(t, resp, "builder")
	b := resp["builder"].(fiber.Map)
	require.Equal(t, "ordersvc-c-build-deadbeef", b["job"])
}
