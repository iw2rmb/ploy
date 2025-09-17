package build

import (
    "encoding/json"
    "net/http/httptest"
    "testing"

    "github.com/gofiber/fiber/v2"
    "github.com/stretchr/testify/require"
)

func TestRenderAndDeployJob_SkipViaEnv(t *testing.T) {
    t.Setenv("MODS_SKIP_DEPLOY_LANES", "e")
    app := fiber.New()
    buildCtx := &BuildContext{APIContext: "apps"}
    app.Get("/test", func(c *fiber.Ctx) error {
        job, err := renderAndDeployJob(c, buildCtx, "E", "app", "/path/img", "", "sha", "", "", "", map[string]string{}, false)
        if err != nil { return err }
        return c.JSON(fiber.Map{"job": job})
    })

    req := httptest.NewRequest("GET", "/test", nil)
    resp, err := app.Test(req, 10000)
    require.NoError(t, err)
    require.Equal(t, 200, resp.StatusCode)
    var body map[string]string
    require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
    require.Equal(t, "", body["job"])
}
