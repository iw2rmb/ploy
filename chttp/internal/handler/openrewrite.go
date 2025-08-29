package handler

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/iw2rmb/ploy/chttp/internal/config"
	"github.com/iw2rmb/ploy/chttp/internal/logging"
)

// OpenRewriteHandler handles Java code transformation requests
type OpenRewriteHandler struct {
	config *config.Config
	logger *logging.Logger
}

// NewOpenRewriteHandler creates a new OpenRewrite handler
func NewOpenRewriteHandler(cfg *config.Config, logger *logging.Logger) *OpenRewriteHandler {
	return &OpenRewriteHandler{
		config: cfg,
		logger: logger,
	}
}

// Transform handles OpenRewrite transformation requests
func (h *OpenRewriteHandler) Transform(c *fiber.Ctx) error {
	var request struct {
		TarArchive string `json:"tar_archive"`
		Recipe     string `json:"recipe"`
		Options    map[string]interface{} `json:"options,omitempty"`
	}

	if err := c.BodyParser(&request); err != nil {
		return c.Status(400).JSON(fiber.Map{
			"error": "Invalid request: " + err.Error(),
		})
	}

	// Validate inputs
	if request.TarArchive == "" {
		return c.Status(400).JSON(fiber.Map{
			"error": "tar_archive is required",
		})
	}
	if request.Recipe == "" {
		request.Recipe = "org.openrewrite.java.migrate.java17.UpgradeToJava17"
	}

	// Generate job ID
	jobID := uuid.New().String()
	workspace := filepath.Join("/tmp", "openrewrite-"+jobID)

	// Create workspace
	if err := os.MkdirAll(workspace, 0755); err != nil {
		h.logger.Error("Failed to create workspace", "error", err)
		return c.Status(500).JSON(fiber.Map{
			"error": "Failed to create workspace",
		})
	}
	defer os.RemoveAll(workspace) // Cleanup

	// Decode and extract archive
	archiveData, err := base64.StdEncoding.DecodeString(request.TarArchive)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{
			"error": "Invalid base64 archive",
		})
	}

	// Write archive to temp file
	archivePath := filepath.Join(workspace, "source.tar.gz")
	if err := ioutil.WriteFile(archivePath, archiveData, 0644); err != nil {
		h.logger.Error("Failed to write archive", "error", err)
		return c.Status(500).JSON(fiber.Map{
			"error": "Failed to write archive",
		})
	}

	// Extract archive
	extractCmd := exec.Command("tar", "xzf", archivePath, "-C", workspace)
	if err := extractCmd.Run(); err != nil {
		h.logger.Error("Failed to extract archive", "error", err)
		return c.Status(400).JSON(fiber.Map{
			"error": "Failed to extract archive",
		})
	}

	// Run OpenRewrite via Docker
	h.logger.Info("Starting OpenRewrite transformation", 
		"job_id", jobID,
		"recipe", request.Recipe)

	dockerCmd := exec.Command("docker", "run", "--rm",
		"-v", workspace+":/workspace",
		"-w", "/workspace",
		"maven:3.9-eclipse-temurin-17",
		"bash", "-c",
		fmt.Sprintf(`
			# Ensure pom.xml exists
			if [ ! -f pom.xml ]; then
				cat > pom.xml <<'EOF'
<?xml version="1.0" encoding="UTF-8"?>
<project xmlns="http://maven.apache.org/POM/4.0.0">
    <modelVersion>4.0.0</modelVersion>
    <groupId>transform</groupId>
    <artifactId>job</artifactId>
    <version>1.0</version>
    <properties>
        <maven.compiler.source>11</maven.compiler.source>
        <maven.compiler.target>17</maven.compiler.target>
    </properties>
</project>
EOF
			fi
			
			# Run transformation
			mvn -B org.openrewrite.maven:rewrite-maven-plugin:5.42.0:run \
				-Drewrite.recipeArtifactCoordinates=org.openrewrite.recipe:rewrite-migrate-java:LATEST \
				-Drewrite.activeRecipes=%s \
				-Dskip.tests=true 2>&1
			
			# Count modified files
			find . -name "*.java" -newer /workspace/source.tar.gz | wc -l
		`, request.Recipe))

	output, err := dockerCmd.CombinedOutput()
	if err != nil {
		h.logger.Error("OpenRewrite transformation failed", 
			"error", err,
			"output", string(output))
		return c.Status(500).JSON(fiber.Map{
			"error": "Transformation failed",
			"details": string(output),
		})
	}

	h.logger.Info("OpenRewrite transformation completed",
		"job_id", jobID,
		"recipe", request.Recipe)

	// Return success response
	return c.JSON(fiber.Map{
		"job_id": jobID,
		"recipe": request.Recipe,
		"success": true,
		"message": "Transformation completed successfully",
		"output": string(output),
	})
}

// RegisterRoutes registers OpenRewrite routes
func (h *OpenRewriteHandler) RegisterRoutes(app *fiber.App) {
	api := app.Group("/v1/openrewrite")
	api.Post("/transform", h.Transform)
	api.Get("/health", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{"status": "healthy"})
	})
}