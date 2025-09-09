package transflow

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
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
	tf.Get("/artifacts/:id", h.GetArtifacts)
	tf.Get("/artifacts/:id/:name", h.DownloadArtifact)
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

    // Prepare workspace structure and write templates for transflow runner
    jobsDir := filepath.Join(tempDir, "roadmap", "transflow", "jobs")
    if err := os.MkdirAll(jobsDir, 0755); err != nil {
        h.recordError(executionID, fmt.Errorf("failed to create jobs dir: %w", err))
        return
    }
    // Write embedded HCL templates
    if err := os.WriteFile(filepath.Join(jobsDir, "planner.hcl"), plannerHCL, 0644); err != nil {
        h.recordError(executionID, fmt.Errorf("failed to write planner.hcl: %w", err))
        return
    }
    if err := os.WriteFile(filepath.Join(jobsDir, "llm_exec.hcl"), llmExecHCL, 0644); err != nil {
        h.recordError(executionID, fmt.Errorf("failed to write llm_exec.hcl: %w", err))
        return
    }
    if err := os.WriteFile(filepath.Join(jobsDir, "orw_apply.hcl"), orwApplyHCL, 0644); err != nil {
        h.recordError(executionID, fmt.Errorf("failed to write orw_apply.hcl: %w", err))
        return
    }
    if err := os.WriteFile(filepath.Join(jobsDir, "reducer.hcl"), reducerHCL, 0644); err != nil {
        h.recordError(executionID, fmt.Errorf("failed to write reducer.hcl: %w", err))
        return
    }

    // Write config to temp file
    configPath := filepath.Join(tempDir, "transflow.yaml")
	configBytes, _ := yaml.Marshal(config)
	if err := os.WriteFile(configPath, configBytes, 0644); err != nil {
		h.recordError(executionID, fmt.Errorf("failed to write config file: %w", err))
		return
	}

	// Create transflow integrations (prefer explicit controller URL if provided)
	controllerURL := os.Getenv("PLOY_CONTROLLER")
	if controllerURL == "" {
		controllerURL = fmt.Sprintf("http://localhost:%s", os.Getenv("PORT"))
		if controllerURL == "http://localhost:" {
			controllerURL = "http://localhost:8080"
		}
	}
	// Ensure controllerURL includes /v1 base for client helpers
	if !strings.HasSuffix(controllerURL, "/v1") {
		controllerURL = strings.TrimRight(controllerURL, "/") + "/v1"
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

	// Persist known artifacts (best-effort)
	artifacts := map[string]string{}
	if h.storage != nil {
		if persisted, err := h.persistArtifacts(executionID, tempDir); err == nil {
			artifacts = persisted
		} else {
			log.Printf("[Transflow] Warning: artifact persistence failed: %v", err)
		}
	}
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
			"artifacts":     artifacts,
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

// persistArtifacts scans the temp workspace for known Transflow artifacts and uploads them to storage.
// Returns a map of artifact logical names to storage keys.
func (h *Handler) persistArtifacts(executionID, tempDir string) (map[string]string, error) {
	artifacts := map[string]string{}
	if h.storage == nil {
		return artifacts, nil
	}
	ctx := context.Background()
	// Planner plan.json
	planPath := filepath.Join(tempDir, "planner", "out", "plan.json")
	if fi, err := os.Stat(planPath); err == nil && !fi.IsDir() {
		key := fmt.Sprintf("artifacts/transflow/%s/plan.json", executionID)
		f, _ := os.Open(planPath)
		defer f.Close()
		if err := h.storage.Put(ctx, key, f, internalStorage.WithContentType("application/json")); err == nil {
			artifacts["plan_json"] = key
		}
	}
	// Reducer next.json
	nextPath := filepath.Join(tempDir, "reducer", "out", "next.json")
	if fi, err := os.Stat(nextPath); err == nil && !fi.IsDir() {
		key := fmt.Sprintf("artifacts/transflow/%s/next.json", executionID)
		f, _ := os.Open(nextPath)
		defer f.Close()
		if err := h.storage.Put(ctx, key, f, internalStorage.WithContentType("application/json")); err == nil {
			artifacts["next_json"] = key
		}
	}
	// ORW diff.patch (search first match)
	// orw-apply/<option>/out/diff.patch
	orwDir := filepath.Join(tempDir, "orw-apply")
	_ = filepath.Walk(orwDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info == nil || info.IsDir() {
			return nil
		}
		if filepath.Base(path) == "diff.patch" {
			key := fmt.Sprintf("artifacts/transflow/%s/diff.patch", executionID)
			f, _ := os.Open(path)
			defer f.Close()
			// Read once for content-type neutrality
			var buf []byte
			buf, _ = io.ReadAll(f)
			_ = f.Close()
			if err := h.storage.Put(ctx, key, io.NopCloser(bytes.NewReader(buf))); err == nil {
				artifacts["diff_patch"] = key
			}
			// Stop after first match
			return io.EOF
		}
		return nil
	})
	return artifacts, nil
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

// GetArtifacts returns the artifact keys for a given execution
func (h *Handler) GetArtifacts(c *fiber.Ctx) error {
	id := c.Params("id")
	st, err := h.getStatus(id)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": fiber.Map{"code": "not_found", "message": "execution not found"}})
	}
	var arts map[string]any
	if st.Result != nil {
		if a, ok := st.Result["artifacts"].(map[string]any); ok {
			arts = a
		}
	}
	if arts == nil {
		arts = map[string]any{}
	}
	return c.JSON(fiber.Map{"artifacts": arts})
}

// DownloadArtifact streams the requested artifact (plan_json|next_json|diff_patch)
func (h *Handler) DownloadArtifact(c *fiber.Ctx) error {
	if h.storage == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{"error": fiber.Map{"code": "storage_disabled", "message": "artifact storage not configured"}})
	}
	id := c.Params("id")
	name := c.Params("name")
	st, err := h.getStatus(id)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": fiber.Map{"code": "not_found", "message": "execution not found"}})
	}
	var arts map[string]any
	if st.Result != nil {
		if a, ok := st.Result["artifacts"].(map[string]any); ok {
			arts = a
		}
	}
	if arts == nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": fiber.Map{"code": "no_artifacts", "message": "no artifacts recorded"}})
	}
	keyAny, ok := arts[name]
	if !ok {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": fiber.Map{"code": "artifact_not_found", "message": "artifact not present"}})
	}
	key, _ := keyAny.(string)
	if key == "" {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": fiber.Map{"code": "artifact_not_found", "message": "artifact not present"}})
	}
	reader, err := h.storage.Get(c.Context(), key)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": fiber.Map{"code": "storage_error", "message": err.Error()}})
	}
	defer reader.Close()
	// Stream
	c.Set("Content-Disposition", fmt.Sprintf("inline; filename=%s", name))
	if strings.HasSuffix(key, ".json") {
		c.Type("json")
	} else if strings.HasSuffix(key, ".patch") || name == "diff_patch" {
		c.Type("text/plain")
	}
	_, _ = io.Copy(c, reader)
	return nil
}
