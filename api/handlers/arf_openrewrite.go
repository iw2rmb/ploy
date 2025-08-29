package handlers

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/iw2rmb/ploy/api/arf"
	"github.com/iw2rmb/ploy/internal/storage"
)

// ARFOpenRewriteHandler handles OpenRewrite-related ARF endpoints
type ARFOpenRewriteHandler struct {
	imageBuilder *arf.OpenRewriteImageBuilder
	dispatcher   *arf.OpenRewriteDispatcher
}

// NewARFOpenRewriteHandler creates a new OpenRewrite handler
func NewARFOpenRewriteHandler(storageClient *storage.StorageClient) (*ARFOpenRewriteHandler, error) {
	// Get Docker and registry configuration from environment
	dockerEndpoint := getEnvOrDefault("DOCKER_HOST", "unix:///var/run/docker.sock")
	registryURL := getEnvOrDefault("OPENREWRITE_REGISTRY", "registry.dev.ployman.app")
	nomadAddr := getEnvOrDefault("NOMAD_ADDR", "http://localhost:4646")
	consulAddr := getEnvOrDefault("CONSUL_HTTP_ADDR", "http://localhost:8500")
	
	// Create image builder with Consul address for recipe lookup
	imageBuilder, err := arf.NewOpenRewriteImageBuilder(dockerEndpoint, registryURL, consulAddr, storageClient)
	if err != nil {
		return nil, fmt.Errorf("failed to create image builder: %w", err)
	}
	
	// Create dispatcher
	dispatcher, err := arf.NewOpenRewriteDispatcher(nomadAddr, consulAddr, storageClient)
	if err != nil {
		return nil, fmt.Errorf("failed to create dispatcher: %w", err)
	}
	
	return &ARFOpenRewriteHandler{
		imageBuilder: imageBuilder,
		dispatcher:   dispatcher,
	}, nil
}

// BuildImageRequest represents the API request for building an image
type BuildImageRequest struct {
	Recipes        []string `json:"recipes" validate:"required,min=1"`
	PackageManager string   `json:"package_manager" validate:"omitempty,oneof=maven gradle"`
	BaseJDK        string   `json:"base_jdk" validate:"omitempty,oneof=11 17 21"`
	Force          bool     `json:"force"`
}

// BuildImage handles POST /v1/arf/openrewrite/build
func (h *ARFOpenRewriteHandler) BuildImage(c *fiber.Ctx) error {
	var req BuildImageRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
			"details": err.Error(),
		})
	}
	
	// Validate request
	if len(req.Recipes) == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "At least one recipe is required",
		})
	}
	
	// Build the image
	buildReq := arf.BuildImageRequest{
		Recipes:        req.Recipes,
		PackageManager: req.PackageManager,
		BaseJDK:        req.BaseJDK,
		Force:          req.Force,
	}
	
	result, err := h.imageBuilder.BuildImage(buildReq)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to build image",
			"details": err.Error(),
		})
	}
	
	return c.JSON(result)
}

// ValidateRecipesRequest represents the API request for validating recipes
type ValidateRecipesRequest struct {
	Recipes []string `json:"recipes" validate:"required,min=1"`
}

// ValidateRecipes handles POST /v1/arf/openrewrite/validate
func (h *ARFOpenRewriteHandler) ValidateRecipes(c *fiber.Ctx) error {
	var req ValidateRecipesRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
			"details": err.Error(),
		})
	}
	
	// Validate recipes
	validated, err := h.imageBuilder.ValidateRecipes(req.Recipes)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Recipe validation failed",
			"details": err.Error(),
		})
	}
	
	// Convert to response format
	response := make([]map[string]string, len(validated))
	for i, recipe := range validated {
		response[i] = map[string]string{
			"short_name": recipe.ShortName,
			"full_class": recipe.FullClass,
			"category":   recipe.Category,
			"artifact":   fmt.Sprintf("%s:%s:%s", recipe.GroupID, recipe.ArtifactID, recipe.Version),
		}
	}
	
	return c.JSON(fiber.Map{
		"valid":   true,
		"recipes": response,
	})
}

// ListRecipes handles GET /v1/arf/openrewrite/recipes
func (h *ARFOpenRewriteHandler) ListRecipes(c *fiber.Ctx) error {
	// Get category filter if provided
	category := c.Query("category")
	
	// Get all known recipes (this would come from imageBuilder.knownRecipes)
	recipes := []map[string]interface{}{
		// Java Migration
		{
			"short_name":  "java11to17",
			"full_class":  "org.openrewrite.java.migrate.Java11toJava17",
			"category":    "java-migration",
			"description": "Migrate from Java 11 to Java 17",
		},
		{
			"short_name":  "java8to11",
			"full_class":  "org.openrewrite.java.migrate.Java8toJava11",
			"category":    "java-migration",
			"description": "Migrate from Java 8 to Java 11",
		},
		{
			"short_name":  "jakarta",
			"full_class":  "org.openrewrite.java.migrate.jakarta.JavaxMigrationToJakarta",
			"category":    "java-migration",
			"description": "Migrate javax to Jakarta EE",
		},
		
		// Spring
		{
			"short_name":  "spring-boot-3",
			"full_class":  "org.openrewrite.java.spring.boot3.UpgradeSpringBoot_3_2",
			"category":    "spring",
			"description": "Upgrade to Spring Boot 3.2",
		},
		{
			"short_name":  "spring-security-6",
			"full_class":  "org.openrewrite.java.spring.security6.UpgradeSpringSecurity_6_0",
			"category":    "spring",
			"description": "Upgrade to Spring Security 6.0",
		},
		
		// Testing
		{
			"short_name":  "junit5",
			"full_class":  "org.openrewrite.java.testing.junit5.JUnit4to5Migration",
			"category":    "testing",
			"description": "Migrate from JUnit 4 to JUnit 5",
		},
		{
			"short_name":  "mockito",
			"full_class":  "org.openrewrite.java.testing.mockito.Mockito1to4Migration",
			"category":    "testing",
			"description": "Upgrade Mockito to version 4",
		},
		{
			"short_name":  "assertj",
			"full_class":  "org.openrewrite.java.testing.assertj.JUnitToAssertj",
			"category":    "testing",
			"description": "Convert JUnit assertions to AssertJ",
		},
		
		// Logging
		{
			"short_name":  "slf4j",
			"full_class":  "org.openrewrite.java.logging.slf4j.Slf4jBestPractices",
			"category":    "logging",
			"description": "Apply SLF4J best practices",
		},
		{
			"short_name":  "log4j2",
			"full_class":  "org.openrewrite.java.logging.log4j.Log4j1ToLog4j2",
			"category":    "logging",
			"description": "Migrate from Log4j 1 to Log4j 2",
		},
	}
	
	// Filter by category if specified
	if category != "" {
		filtered := []map[string]interface{}{}
		for _, recipe := range recipes {
			if recipe["category"] == category {
				filtered = append(filtered, recipe)
			}
		}
		recipes = filtered
	}
	
	return c.JSON(fiber.Map{
		"recipes": recipes,
		"total":   len(recipes),
	})
}

// GenerateImageNameRequest represents the API request for generating an image name
type GenerateImageNameRequest struct {
	Recipes        []string `json:"recipes" validate:"required,min=1"`
	PackageManager string   `json:"package_manager"`
}

// GenerateImageName handles POST /v1/arf/openrewrite/generate-name
func (h *ARFOpenRewriteHandler) GenerateImageName(c *fiber.Ctx) error {
	var req GenerateImageNameRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
			"details": err.Error(),
		})
	}
	
	// Set default package manager
	if req.PackageManager == "" {
		req.PackageManager = "maven"
	}
	
	// Generate image name
	imageName := h.imageBuilder.GenerateImageName(req.Recipes, req.PackageManager)
	
	return c.JSON(fiber.Map{
		"image_name": imageName,
		"full_image": fmt.Sprintf("%s/%s:latest", "registry.dev.ployman.app", imageName),
		"recipes":    req.Recipes,
		"manager":    req.PackageManager,
	})
}

// TransformRequest represents a transformation request
type TransformRequest struct {
	ProjectURL     string   `json:"project_url" validate:"required"`
	Recipes        []string `json:"recipes" validate:"required,min=1"`
	PackageManager string   `json:"package_manager"`
	BaseJDK        string   `json:"base_jdk"`
	Branch         string   `json:"branch"`
}

// Transform handles POST /v1/arf/openrewrite/transform
func (h *ARFOpenRewriteHandler) Transform(c *fiber.Ctx) error {
	var req TransformRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
			"details": err.Error(),
		})
	}
	
	// First, build or verify the image exists
	buildReq := arf.BuildImageRequest{
		Recipes:        req.Recipes,
		PackageManager: req.PackageManager,
		BaseJDK:        req.BaseJDK,
		Force:          false, // Don't force rebuild
	}
	
	buildResult, err := h.imageBuilder.BuildImage(buildReq)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to prepare image",
			"details": err.Error(),
		})
	}
	
	// Now submit the transformation job using the built image
	jobID := fmt.Sprintf("openrewrite-%d", time.Now().Unix())
	
	// Create transformation job parameters
	jobParams := map[string]string{
		"job_id":     jobID,
		"image":      buildResult.FullImage,
		"recipes":    strings.Join(req.Recipes, ","),
		"project_url": req.ProjectURL,
		"branch":     req.Branch,
	}
	
	// Submit to dispatcher
	if err := h.dispatcher.SubmitTransformation(jobID, jobParams); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to submit transformation job",
			"details": err.Error(),
		})
	}
	
	return c.Status(fiber.StatusAccepted).JSON(fiber.Map{
		"job_id":     jobID,
		"image":      buildResult.FullImage,
		"recipes":    req.Recipes,
		"status":     "submitted",
		"message":    "Transformation job submitted successfully",
	})
}

// JobStatus handles GET /v1/arf/openrewrite/status/:jobId
func (h *ARFOpenRewriteHandler) JobStatus(c *fiber.Ctx) error {
	jobID := c.Params("jobId")
	if jobID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Job ID is required",
		})
	}
	
	// Get job status from dispatcher
	status, err := h.dispatcher.GetJobStatus(jobID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "Job not found",
			"job_id": jobID,
		})
	}
	
	return c.JSON(status)
}

// RegisterRoutes registers all OpenRewrite ARF routes
func (h *ARFOpenRewriteHandler) RegisterRoutes(app *fiber.App) {
	arf := app.Group("/v1/arf/openrewrite")
	
	// Image building
	arf.Post("/build", h.BuildImage)
	arf.Post("/validate", h.ValidateRecipes)
	arf.Post("/generate-name", h.GenerateImageName)
	
	// Recipe management
	arf.Get("/recipes", h.ListRecipes)
	
	// Transformation
	arf.Post("/transform", h.Transform)
	arf.Get("/status/:jobId", h.JobStatus)
}

// getEnvOrDefault gets environment variable or returns default
func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}