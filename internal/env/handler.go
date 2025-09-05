package env

import (
	"fmt"
	"log"

	"github.com/gofiber/fiber/v2"
	envstore "github.com/iw2rmb/ploy/internal/envstore"
	"github.com/iw2rmb/ploy/internal/utils"
	"github.com/iw2rmb/ploy/internal/validation"
)

func SetEnvVars(c *fiber.Ctx, envStore envstore.EnvStoreInterface) error {
	app := c.Params("app")

	var req map[string]string
	if err := c.BodyParser(&req); err != nil {
		return utils.ErrJSON(c, 400, fmt.Errorf("invalid request body"))
	}

	// Validate environment variables
	if err := validation.ValidateEnvVars(req); err != nil {
		return utils.ErrJSON(c, 400, fmt.Errorf("validation failed: %w", err))
	}

	log.Printf("Setting environment variables for app %s", app)

	if err := envStore.SetAll(app, req); err != nil {
		return utils.ErrJSON(c, 500, fmt.Errorf("failed to store environment variables: %w", err))
	}

	return c.JSON(fiber.Map{
		"status":  "updated",
		"app":     app,
		"count":   len(req),
		"message": "Environment variables updated successfully",
	})
}

func GetEnvVars(c *fiber.Ctx, envStore envstore.EnvStoreInterface) error {
	app := c.Params("app")

	log.Printf("Getting environment variables for app %s", app)

	envVars, err := envStore.GetAll(app)
	if err != nil {
		return utils.ErrJSON(c, 500, fmt.Errorf("failed to retrieve environment variables: %w", err))
	}

	return c.JSON(fiber.Map{
		"app": app,
		"env": envVars,
	})
}

func SetEnvVar(c *fiber.Ctx, envStore envstore.EnvStoreInterface) error {
	app := c.Params("app")
	key := c.Params("key")

	var req struct {
		Value string `json:"value"`
	}
	if err := c.BodyParser(&req); err != nil {
		return utils.ErrJSON(c, 400, fmt.Errorf("invalid request body"))
	}

	// Validate environment variable name
	if err := validation.ValidateEnvVarName(key); err != nil {
		return utils.ErrJSON(c, 400, fmt.Errorf("invalid environment variable name: %w", err))
	}

	// Validate environment variable value
	if err := validation.ValidateEnvVarValue(req.Value); err != nil {
		return utils.ErrJSON(c, 400, fmt.Errorf("invalid environment variable value: %w", err))
	}

	log.Printf("Setting environment variable %s for app %s", key, app)

	if err := envStore.Set(app, key, req.Value); err != nil {
		return utils.ErrJSON(c, 500, fmt.Errorf("failed to store environment variable: %w", err))
	}

	return c.JSON(fiber.Map{
		"status":  "updated",
		"app":     app,
		"key":     key,
		"message": "Environment variable updated successfully",
	})
}

func DeleteEnvVar(c *fiber.Ctx, envStore envstore.EnvStoreInterface) error {
	app := c.Params("app")
	key := c.Params("key")

	log.Printf("Deleting environment variable %s for app %s", key, app)

	if err := envStore.Delete(app, key); err != nil {
		return utils.ErrJSON(c, 500, fmt.Errorf("failed to delete environment variable: %w", err))
	}

	return c.JSON(fiber.Map{
		"status":  "deleted",
		"app":     app,
		"key":     key,
		"message": "Environment variable deleted successfully",
	})
}
