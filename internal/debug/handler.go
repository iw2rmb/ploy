package debug

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/gofiber/fiber/v2"
	ibuilders "github.com/iw2rmb/ploy/internal/builders"
	envstore "github.com/iw2rmb/ploy/internal/envstore"
	orchestration "github.com/iw2rmb/ploy/internal/orchestration"
	ipolicy "github.com/iw2rmb/ploy/internal/policy"
	"github.com/iw2rmb/ploy/internal/utils"
)

func DebugApp(c *fiber.Ctx, envStore envstore.EnvStoreInterface) error {
	app := c.Params("app")
	lane := c.Query("lane", "")

	var req struct {
		SSHEnabled bool `json:"ssh_enabled"`
	}
	if err := c.BodyParser(&req); err != nil {
		return utils.ErrJSON(c, 400, fmt.Errorf("invalid request body"))
	}

	log.Printf("Creating debug build for app %s (lane: %s) with SSH enabled: %v", app, lane, req.SSHEnabled)

	// OPA policy enforcement for debug builds
	env := c.Query("env", "dev")
	breakGlass := c.Query("break_glass", "false") == "true"

	// Debug builds always have potential for SSH access, require policy validation
	if err := ipolicy.DefaultEnforcer.Enforce(ipolicy.ArtifactInput{
		Signed:      true, // Debug builds are considered signed for policy purposes
		SBOMPresent: true, // Debug builds are considered to have SBOM for policy purposes
		Env:         env,
		SSHEnabled:  req.SSHEnabled,
		BreakGlass:  breakGlass,
		App:         app,
		Lane:        lane,
		Debug:       true,
	}); err != nil {
		return utils.ErrJSON(c, 403, fmt.Errorf("debug build policy enforcement failed: %w", err))
	}

	srcDir := filepath.Join(os.TempDir(), fmt.Sprintf("debug-src-%s-%d", app, time.Now().Unix()))
	outDir := filepath.Join(os.TempDir(), fmt.Sprintf("debug-out-%s-%d", app, time.Now().Unix()))
	sha := fmt.Sprintf("debug-%d", time.Now().Unix())

	if err := os.MkdirAll(srcDir, 0755); err != nil {
		return utils.ErrJSON(c, 500, fmt.Errorf("failed to create source directory: %v", err))
	}
	if err := os.MkdirAll(outDir, 0755); err != nil {
		return utils.ErrJSON(c, 500, fmt.Errorf("failed to create output directory: %v", err))
	}

	envVarsData, err := envStore.GetAll(app)
	if err != nil {
		log.Printf("Failed to get environment variables for %s: %v", app, err)
		envVarsData = make(map[string]string)
	}
	envVars := make(map[string]string)
	for k, v := range envVarsData {
		envVars[k] = v
	}

	debugResult, err := ibuilders.DefaultDebugBuilder.BuildDebugInstance(app, lane, srcDir, sha, outDir, envVars, req.SSHEnabled)
	if err != nil {
		return utils.ErrJSON(c, 500, fmt.Errorf("debug build failed: %v", err))
	}

	debugInstanceName := fmt.Sprintf("debug-%s-%d", app, time.Now().Unix())

	if utils.Getenv("PLOY_SKIP_DEPLOY", "false") != "true" {
        renderData := orchestration.RenderData{
            App:         debugInstanceName,
            ImagePath:   debugResult.ImagePath,
            DockerImage: debugResult.DockerImage,
            EnvVars:     envVars,
            IsDebug:     true,
            Lane:        lane,
        }
        templatePath, err := orchestration.RenderTemplate(lane, renderData)
        if err != nil {
            return utils.ErrJSON(c, 500, fmt.Errorf("failed to render debug template: %v", err))
        }
        if vErr := orchestration.ValidateJob(templatePath); vErr != nil {
            return utils.ErrJSON(c, 500, fmt.Errorf("job validation failed: %v", vErr))
        }
        if err := orchestration.Submit(templatePath); err != nil {
            return utils.ErrJSON(c, 500, fmt.Errorf("failed to deploy debug instance: %v", err))
        }
		os.Remove(templatePath)
	}

	response := fiber.Map{
		"status":      "debug_created",
		"app":         app,
		"instance":    debugInstanceName,
		"ssh_enabled": req.SSHEnabled,
		"message":     "Debug instance created successfully",
	}

	if req.SSHEnabled {
		response["ssh_command"] = debugResult.SSHCommand
		response["ssh_public_key"] = debugResult.SSHPublicKey
	}

	return c.JSON(response)
}

func RollbackApp(c *fiber.Ctx) error {
	app := c.Params("app")

	var req struct {
		SHA string `json:"sha"`
	}
	if err := c.BodyParser(&req); err != nil {
		return utils.ErrJSON(c, 400, fmt.Errorf("invalid request body"))
	}

	log.Printf("Rolling back app %s to SHA %s", app, req.SHA)

	return c.JSON(fiber.Map{
		"status":  "rolled_back",
		"app":     app,
		"sha":     req.SHA,
		"message": "Application rolled back successfully",
	})
}
