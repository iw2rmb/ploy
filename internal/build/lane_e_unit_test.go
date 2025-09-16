package build

import (
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/iw2rmb/ploy/internal/config"
	project "github.com/iw2rmb/ploy/internal/detect/project"
	"github.com/stretchr/testify/require"
)

// Ensure buildLaneE returns 400 when Dockerfile is missing and autogen is not enabled.
func TestBuildLaneE_MissingDockerfile_NoAutogen_400(t *testing.T) {
	tmp := t.TempDir()
	deps := &BuildDependencies{}
	buildCtx := &BuildContext{APIContext: "apps", AppType: config.UserApp}
	facts := project.BuildFacts{HasJib: false}

	app := fiber.New()
	app.Post("/e", func(c *fiber.Ctx) error {
		_, _, _, _ = buildLaneE(c, deps, buildCtx, "app", tmp, "sha", tmp, "", facts, map[string]string{})
		// Let buildLaneE write the response status/body
		return nil
	})
	req := httptest.NewRequest("POST", "/e?autogen_dockerfile=false", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	require.Equal(t, 400, resp.StatusCode)
}
