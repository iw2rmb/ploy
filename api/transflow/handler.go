package transflow

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/iw2rmb/ploy/internal/cli/transflow"
	"github.com/iw2rmb/ploy/internal/git/provider"
	"github.com/iw2rmb/ploy/internal/orchestration"
	internalStorage "github.com/iw2rmb/ploy/internal/storage"
	"gopkg.in/yaml.v3"
)

// Handler provides HTTP endpoints for Transflow operations
type Handler struct {
	gitProvider    provider.GitProvider
	storage        internalStorage.Storage
	statusStore    orchestration.KV
	arfRegistryURL string
	arfMavenGroup  string
}

// NewHandler creates a new Transflow HTTP handler
func NewHandler(
	gitProvider provider.GitProvider,
	storage internalStorage.Storage,
	statusStore orchestration.KV,
) *Handler {
	return &Handler{
		gitProvider:    gitProvider,
		storage:        storage,
		statusStore:    statusStore,
		arfRegistryURL: os.Getenv("PLOY_ARF_REGISTRY"),
		arfMavenGroup:  os.Getenv("PLOY_ARF_MAVEN_GROUP"),
	}
}

// RegisterRoutes registers Transflow routes with the Fiber app
func (h *Handler) RegisterRoutes(app *fiber.App) {
	tf := app.Group("/v1/transflow")

	// Transflow execution
	tf.Post("/run", h.RunTransflow)
	tf.Get("/status/:id", h.GetTransflowStatus)
	tf.Get("/list", h.ListTransflows)
	tf.Delete("/:id", h.CancelTransflow)
}

// TransflowRunRequest represents the request body for running a transflow
type TransflowRunRequest struct {
	Config     string                 `json:"config,omitempty"`      // YAML config as string
	ConfigData map[string]interface{} `json:"config_data,omitempty"` // Or as structured data
	TestMode   bool                   `json:"test_mode,omitempty"`
}

// TransflowStatus represents the status of a transflow execution
type TransflowStatus struct {
	ID        string                 `json:"id"`
	Status    string                 `json:"status"`
	StartTime time.Time              `json:"start_time"`
	EndTime   *time.Time             `json:"end_time,omitempty"`
	Error     string                 `json:"error,omitempty"`
	Result    map[string]interface{} `json:"result,omitempty"`
}

// RunTransflow handles POST /v1/transflow/run
func (h *Handler) RunTransflow(c *fiber.Ctx) error {
	var req TransflowRunRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": fiber.Map{
				"code":    "invalid_request",
				"message": "Invalid request format",
				"details": err.Error(),
			},
		})
	}

	// Parse configuration
	var config *transflow.TransflowConfig
	if req.Config != "" {
		// Parse YAML string
		if err := yaml.Unmarshal([]byte(req.Config), &config); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": fiber.Map{
					"code":    "invalid_config",
					"message": "Failed to parse transflow configuration",
					"details": err.Error(),
				},
			})
		}
	} else if req.ConfigData != nil {
		// Convert structured data to YAML then parse
		yamlBytes, err := yaml.Marshal(req.ConfigData)
		if err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": fiber.Map{
					"code":    "invalid_config",
					"message": "Failed to convert configuration",
					"details": err.Error(),
				},
			})
		}
		if err := yaml.Unmarshal(yamlBytes, &config); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": fiber.Map{
					"code":    "invalid_config",
					"message": "Failed to parse transflow configuration",
					"details": err.Error(),
				},
			})
		}
	} else {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": fiber.Map{
				"code":    "missing_config",
				"message": "Either 'config' or 'config_data' must be provided",
			},
		})
	}

	// Validate configuration
	if err := config.Validate(); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": fiber.Map{
				"code":    "invalid_config",
				"message": "Configuration validation failed",
				"details": err.Error(),
			},
		})
	}

	// Generate execution ID
	executionID := fmt.Sprintf("tf-%s", uuid.New().String()[:8])

	// Store initial status
	status := TransflowStatus{
		ID:        executionID,
		Status:    "initializing",
		StartTime: time.Now(),
	}
	if err := h.storeStatus(status); err != nil {
		log.Printf("Failed to store initial status: %v", err)
	}

	// Execute transflow asynchronously
	go h.executeTransflow(executionID, config, req.TestMode)

	// Return immediate response
	return c.Status(fiber.StatusAccepted).JSON(fiber.Map{
		"execution_id": executionID,
		"status":       "initializing",
		"message":      "Transflow execution started",
		"status_url":   fmt.Sprintf("/v1/transflow/status/%s", executionID),
	})
}

// executeTransflow runs the transflow workflow asynchronously
func (h *Handler) executeTransflow(executionID string, config *transflow.TransflowConfig, testMode bool) {
	ctx := context.Background()

	// Update status to running
	status := TransflowStatus{
		ID:        executionID,
		Status:    "running",
		StartTime: time.Now(),
	}
	if err := h.storeStatus(status); err != nil {
		log.Printf("Failed to update status: %v", err)
	}

	// Create temp directory for workflow
	tempDir, err := os.MkdirTemp("", fmt.Sprintf("transflow-%s-*", executionID))
	if err != nil {
		h.recordError(executionID, fmt.Errorf("failed to create temp directory: %w", err))
		return
	}
	defer os.RemoveAll(tempDir)

	// Write config to temp file
	configPath := filepath.Join(tempDir, "transflow.yaml")
	configBytes, _ := yaml.Marshal(config)
	if err := os.WriteFile(configPath, configBytes, 0644); err != nil {
		h.recordError(executionID, fmt.Errorf("failed to write config file: %w", err))
		return
	}

	// Create transflow integrations
	controllerURL := fmt.Sprintf("http://localhost:%s", os.Getenv("PORT"))
	if controllerURL == "http://localhost:" {
		controllerURL = "http://localhost:8080" // Default port
	}
	integrations := transflow.NewTransflowIntegrationsWithTestMode(controllerURL, tempDir, testMode)

	// Create configured runner
	runner, err := integrations.CreateConfiguredRunner(config)
	if err != nil {
		h.recordError(executionID, fmt.Errorf("failed to create runner: %w", err))
		return
	}

	// Execute the workflow
	result, err := runner.Run(ctx)
	if err != nil {
		h.recordError(executionID, err)
		return
	}

	// Store successful result
	endTime := time.Now()
	status = TransflowStatus{
		ID:        executionID,
		Status:    "completed",
		StartTime: status.StartTime,
		EndTime:   &endTime,
		Result: map[string]interface{}{
			"success":       result.Success,
			"workflow_id":   result.WorkflowID,
			"branch_name":   result.BranchName,
			"commit_sha":    result.CommitSHA,
			"build_version": result.BuildVersion,
			"mr_url":        result.MRURL,
			"healing_used":  result.HealingSummary != nil && result.HealingSummary.Enabled,
			"duration":      result.Duration.String(),
		},
	}
	if err := h.storeStatus(status); err != nil {
		log.Printf("Failed to store final status: %v", err)
	}
}

// recordError records an error status for the execution
func (h *Handler) recordError(executionID string, err error) {
	endTime := time.Now()
	status := TransflowStatus{
		ID:      executionID,
		Status:  "failed",
		EndTime: &endTime,
		Error:   err.Error(),
	}
	if storeErr := h.storeStatus(status); storeErr != nil {
		log.Printf("Failed to store error status: %v", storeErr)
	}
}

// GetTransflowStatus handles GET /v1/transflow/status/:id
func (h *Handler) GetTransflowStatus(c *fiber.Ctx) error {
	executionID := c.Params("id")
	if executionID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": fiber.Map{
				"code":    "missing_id",
				"message": "Execution ID is required",
			},
		})
	}

	// Retrieve status from store
	status, err := h.getStatus(executionID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": fiber.Map{
				"code":    "not_found",
				"message": fmt.Sprintf("Transflow execution %s not found", executionID),
			},
		})
	}

	return c.JSON(status)
}

// ListTransflows handles GET /v1/transflow/list
func (h *Handler) ListTransflows(c *fiber.Ctx) error {
	// List all transflow executions from the status store
	prefix := "transflow/status/"
	keys, err := h.statusStore.Keys(prefix, "")
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": fiber.Map{
				"code":    "storage_error",
				"message": "Failed to list transflow executions",
				"details": err.Error(),
			},
		})
	}

	executions := []TransflowStatus{}
	for _, key := range keys {
		data, err := h.statusStore.Get(key)
		if err != nil {
			continue
		}

		var status TransflowStatus
		if err := json.Unmarshal([]byte(data), &status); err != nil {
			continue
		}
		executions = append(executions, status)
	}

	return c.JSON(fiber.Map{
		"executions": executions,
		"count":      len(executions),
	})
}

// CancelTransflow handles DELETE /v1/transflow/:id
func (h *Handler) CancelTransflow(c *fiber.Ctx) error {
	executionID := c.Params("id")
	if executionID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": fiber.Map{
				"code":    "missing_id",
				"message": "Execution ID is required",
			},
		})
	}

	// Check if execution exists
	status, err := h.getStatus(executionID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": fiber.Map{
				"code":    "not_found",
				"message": fmt.Sprintf("Transflow execution %s not found", executionID),
			},
		})
	}

	// Can only cancel running executions
	if status.Status != "running" && status.Status != "initializing" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": fiber.Map{
				"code":    "invalid_state",
				"message": fmt.Sprintf("Cannot cancel execution in state: %s", status.Status),
			},
		})
	}

	// Update status to cancelled
	endTime := time.Now()
	status.Status = "cancelled"
	status.EndTime = &endTime
	if err := h.storeStatus(*status); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": fiber.Map{
				"code":    "storage_error",
				"message": "Failed to update execution status",
				"details": err.Error(),
			},
		})
	}

	return c.JSON(fiber.Map{
		"message": fmt.Sprintf("Transflow execution %s cancelled", executionID),
		"status":  status,
	})
}

// storeStatus stores the status in the KV store
func (h *Handler) storeStatus(status TransflowStatus) error {
	if h.statusStore == nil {
		return nil // Silently skip if no store configured
	}

	key := fmt.Sprintf("transflow/status/%s", status.ID)
	data, err := json.Marshal(status)
	if err != nil {
		return err
	}

	return h.statusStore.Put(key, data)
}

// getStatus retrieves the status from the KV store
func (h *Handler) getStatus(executionID string) (*TransflowStatus, error) {
	if h.statusStore == nil {
		return nil, fmt.Errorf("status store not configured")
	}

	key := fmt.Sprintf("transflow/status/%s", executionID)
	data, err := h.statusStore.Get(key)
	if err != nil {
		return nil, err
	}

	var status TransflowStatus
	if err := json.Unmarshal([]byte(data), &status); err != nil {
		return nil, err
	}

	return &status, nil
}
