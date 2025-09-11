package transflow

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
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
	// Real-time events push endpoint
	tf.Post("/event", h.ReportEvent)
	// Logs streaming (SSE stub)
	tf.Get("/logs/:id", h.StreamLogs)
	// Debug: Nomad recent job diagnostics (dev only)
	tf.Get("/debug/nomad", h.DebugNomad)
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
	// Enriched runtime fields
	Phase    string                `json:"phase,omitempty"`
	Overdue  bool                  `json:"overdue,omitempty"`
	Duration string                `json:"duration,omitempty"`
	Steps    []TransflowStepStatus `json:"steps,omitempty"`
	LastJob  *TransflowLastJob     `json:"last_job,omitempty"`
}

// TransflowStepStatus represents a single step update with timestamp
type TransflowStepStatus struct {
	Step    string    `json:"step,omitempty"`
	Phase   string    `json:"phase,omitempty"`
	Level   string    `json:"level,omitempty"`
	Message string    `json:"message,omitempty"`
	Time    time.Time `json:"time"`
}

// TransflowLastJob captures metadata about the most recent submitted Nomad job
type TransflowLastJob struct {
	JobName     string    `json:"job_name,omitempty"`
	AllocID     string    `json:"alloc_id,omitempty"`
	SubmittedAt time.Time `json:"submitted_at"`
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
		Phase:     "init",
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
	// Top-level guard: always convert panics to a terminal failure status
	defer func() {
		if r := recover(); r != nil {
			h.recordError(executionID, fmt.Errorf("transflow execution panic: %v", r))
		}
	}()

	// Top-level execution timeout to ensure terminal status is written
	execTimeout := 45 * time.Minute
	if v := os.Getenv("PLOY_TRANSFLOW_EXEC_TIMEOUT"); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			execTimeout = d
		}
	}
	parentCtx := context.Background()
	ctx, cancel := context.WithTimeout(parentCtx, execTimeout)
	defer cancel()

	// Update status to running
	status := TransflowStatus{
		ID:        executionID,
		Status:    "running",
		StartTime: time.Now(),
		Phase:     "running",
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

	// Wire event reporter for real-time observability
	reporter := transflow.NewControllerEventReporter(controllerURL, executionID)
	runner.SetEventReporter(reporter)

	// Expose controller and execution ID to job templates for in-job event pushes
	_ = os.Setenv("PLOY_CONTROLLER", controllerURL)
	_ = os.Setenv("PLOY_TRANSFLOW_EXECUTION_ID", executionID)
	// Expose SeaweedFS URL default for task-side uploads
	if os.Getenv("PLOY_SEAWEEDFS_URL") == "" {
		_ = os.Setenv("PLOY_SEAWEEDFS_URL", "http://seaweedfs-filer.service.consul:8888")
	}

	// Image override: use TRANSFLOW_ORW_APPLY_IMAGE only (set in API env when needed)

	// Execute the workflow with timeout awareness; ensure terminal status on any error
	var (
		result *transflow.TransflowResult
		runErr error
		doneCh = make(chan struct{})
	)
	go func() {
		defer close(doneCh)
		result, runErr = runner.Run(ctx)
	}()

	select {
	case <-doneCh:
		if runErr != nil {
			// Best-effort: persist any artifacts (e.g., error logs) and enrich error message
			if h.storage != nil {
				if _, err := h.persistArtifacts(executionID, tempDir); err != nil {
					log.Printf("[Transflow] Warning: artifact persistence on failure failed: %v", err)
				}
			}
			// Try to include first error.log contents from orw-apply in the error message for clarity
			orwDir := filepath.Join(tempDir, "orw-apply")
			var errDetail string
			_ = filepath.Walk(orwDir, func(path string, info os.FileInfo, err error) error {
				if err != nil || info == nil || info.IsDir() {
					return nil
				}
				if filepath.Base(path) == "error.log" {
					if b, e := os.ReadFile(path); e == nil {
						s := strings.TrimSpace(string(b))
						if len(s) > 0 {
							// Limit to a reasonable snippet
							if len(s) > 1024 {
								s = s[:1024]
							}
							errDetail = s
						}
					}
					return io.EOF // stop after first match
				}
				return nil
			})
			if errDetail != "" {
				runErr = fmt.Errorf("%v; details: %s", runErr, errDetail)
			}
			h.recordError(executionID, runErr)
			return
		}
		// continue to success handling below
	case <-ctx.Done():
		// Timeout/cancellation — record a terminal error
		h.recordError(executionID, fmt.Errorf("transflow execution exceeded max duration (%s)", execTimeout))
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

// GetTransflowStatus handles GET /v1/transflow/status/:id and enriches running statuses with duration/overdue
// (See bottom of file for the handler that returns status and includes runtime enrichment.)

// recordError records an error status for the execution
func (h *Handler) recordError(executionID string, err error) {
	endTime := time.Now()
	// Preserve existing status (steps, phase, start time) if available
	var status *TransflowStatus
	if st, getErr := h.getStatus(executionID); getErr == nil && st != nil {
		status = st
	} else {
		status = &TransflowStatus{ID: executionID, StartTime: endTime}
	}
	status.Status = "failed"
	status.EndTime = &endTime
	status.Error = err.Error()
	if storeErr := h.storeStatus(*status); storeErr != nil {
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
	// ORW diff.patch
	// Prefer task-side upload key when present; also support legacy presence on local FS (pre-mount design)
	orwDir := filepath.Join(tempDir, "orw-apply")
	_ = filepath.Walk(orwDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info == nil || info.IsDir() {
			return nil
		}
		if filepath.Base(path) == "diff.patch" {
			// Prefer task-side upload key if present; support both legacy and new prefixes
			keyPrimary := fmt.Sprintf("artifacts/transflow/%s/diff.patch", executionID)
			keyAlt := fmt.Sprintf("transflow/%s/diff.patch", executionID)
			// If already present (task-side upload), record and skip
			if ok, _ := h.storage.Exists(ctx, keyPrimary); ok {
				artifacts["diff_patch"] = keyPrimary
			} else if ok2, _ := h.storage.Exists(ctx, keyAlt); ok2 {
				artifacts["diff_patch"] = keyAlt
			} else {
				f, _ := os.Open(path)
				defer f.Close()
				// Read once for content-type neutrality
				var buf []byte
				buf, _ = io.ReadAll(f)
				_ = f.Close()
				if err := h.storage.Put(ctx, keyPrimary, io.NopCloser(bytes.NewReader(buf))); err == nil {
					artifacts["diff_patch"] = keyPrimary
				}
			}
			return nil
		}
		if filepath.Base(path) == "error.log" {
			key := fmt.Sprintf("artifacts/transflow/%s/error.log", executionID)
			if ok, _ := h.storage.Exists(ctx, key); ok {
				artifacts["error_log"] = key
			} else {
				f, _ := os.Open(path)
				defer f.Close()
				if err := h.storage.Put(ctx, key, f, internalStorage.WithContentType("text/plain")); err == nil {
					artifacts["error_log"] = key
				}
			}
			return nil
		}
		return nil
	})
	// If no local diff found or persisted above, check storage proactively for known keys (SeaweedFS-only IO path)
	if _, ok := artifacts["diff_patch"]; !ok {
		keyPrimary := fmt.Sprintf("artifacts/transflow/%s/diff.patch", executionID)
		keyAlt := fmt.Sprintf("transflow/%s/diff.patch", executionID)
		if ok, _ := h.storage.Exists(ctx, keyPrimary); ok {
			artifacts["diff_patch"] = keyPrimary
		} else if ok2, _ := h.storage.Exists(ctx, keyAlt); ok2 {
			artifacts["diff_patch"] = keyAlt
		}
	}
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

	// Enrich running statuses: add duration and overdue fields without changing stored state
	if status.Status == "running" {
		elapsed := time.Since(status.StartTime)
		status.Duration = elapsed.String()
		overdueThresh := 30 * time.Minute
		if v := os.Getenv("PLOY_TRANSFLOW_OVERDUE"); v != "" {
			if d, e := time.ParseDuration(v); e == nil && d > 0 {
				overdueThresh = d
			}
		}
		status.Overdue = elapsed > overdueThresh
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

// TransflowEvent represents a real-time event emitted by runner/jobs
type TransflowEvent struct {
	ExecutionID string    `json:"execution_id"`
	Phase       string    `json:"phase,omitempty"`
	Step        string    `json:"step,omitempty"`
	Level       string    `json:"level,omitempty"`
	Message     string    `json:"message,omitempty"`
	Time        time.Time `json:"ts,omitempty"`
	JobName     string    `json:"job_name,omitempty"`
	AllocID     string    `json:"alloc_id,omitempty"`
}

// ReportEvent handles POST /v1/transflow/event to update live status metadata
func (h *Handler) ReportEvent(c *fiber.Ctx) error {
	if h.statusStore == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error": fiber.Map{"code": "storage_disabled", "message": "status store not configured"},
		})
	}
	var ev TransflowEvent
	if err := c.BodyParser(&ev); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": fiber.Map{"code": "invalid_event", "message": "failed to parse event", "details": err.Error()},
		})
	}
	if ev.ExecutionID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": fiber.Map{"code": "missing_execution_id", "message": "execution_id is required"},
		})
	}
	// Load or initialize status
	st, err := h.getStatus(ev.ExecutionID)
	if err != nil || st == nil || st.ID == "" {
		now := time.Now()
		st = &TransflowStatus{ID: ev.ExecutionID, Status: "running", StartTime: now}
	}
	// Event timestamp
	ts := ev.Time
	if ts.IsZero() {
		ts = time.Now()
	}
	// Update phase
	if ev.Phase != "" {
		st.Phase = ev.Phase
	}
	// Append step record
	if ev.Step != "" || ev.Message != "" || ev.Phase != "" {
		st.Steps = append(st.Steps, TransflowStepStatus{
			Step:    ev.Step,
			Phase:   ev.Phase,
			Level:   ev.Level,
			Message: ev.Message,
			Time:    ts,
		})
	}
	// Last job metadata if provided
	if ev.JobName != "" {
		st.LastJob = &TransflowLastJob{JobName: ev.JobName, AllocID: ev.AllocID, SubmittedAt: ts}
	}
	if err := h.storeStatus(*st); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": fiber.Map{"code": "storage_error", "message": "failed to persist status", "details": err.Error()},
		})
	}
	return c.JSON(fiber.Map{"ok": true})
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

// StreamLogs provides a basic Server-Sent Events (SSE) stub for live transflow logs.
// For now, it emits a single init event and returns; future work will stream steps and job tails.
func (h *Handler) StreamLogs(c *fiber.Ctx) error {
	id := c.Params("id")
	if id == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": fiber.Map{"code": "missing_id", "message": "Execution ID is required"}})
	}
	// Set SSE headers
	c.Set("Content-Type", "text/event-stream")
	c.Set("Cache-Control", "no-cache")
	c.Set("Connection", "keep-alive")
	c.Set("X-Accel-Buffering", "no")

	follow := strings.ToLower(c.Query("follow", "true")) != "false"
	interval := 2 * time.Second
	if v := os.Getenv("PLOY_TRANSFLOW_SSE_INTERVAL"); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			interval = d
		}
	}
	// Optional time cap
	maxDur := 30 * time.Minute
	if v := os.Getenv("PLOY_TRANSFLOW_SSE_TIMEOUT"); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			maxDur = d
		}
	}

	start := time.Now()
	c.Context().SetBodyStreamWriter(func(w *bufio.Writer) {
		// helper to write event
		writeEvent := func(event string, data string) bool {
			if _, err := w.WriteString("event: " + event + "\n"); err != nil {
				return false
			}
			if _, err := w.WriteString("data: " + data + "\n\n"); err != nil {
				return false
			}
			if err := w.Flush(); err != nil {
				return false
			}
			return true
		}

		// Send init
		initPayload := fmt.Sprintf(`{"id":"%s","message":"SSE connected"}`, id)
		if !writeEvent("init", initPayload) {
			return
		}

		// Always send current snapshot of steps
		lastCount := 0
		st, err := h.getStatus(id)
		if err == nil && st != nil {
			if len(st.Steps) > 0 {
				for i := 0; i < len(st.Steps); i++ {
					b, _ := json.Marshal(st.Steps[i])
					if !writeEvent("step", string(b)) {
						return
					} else if !follow {
						// No status available but follow=false: end immediately
						_ = writeEvent("end", `{"status":"unknown"}`)
						return
					}
				}
				lastCount = len(st.Steps)
			}
			// If not following, end now
			if !follow {
				fin := map[string]any{"status": st.Status, "phase": st.Phase, "duration": st.Duration}
				b, _ := json.Marshal(fin)
				_ = writeEvent("end", string(b))
				return
			}
			// If already terminal, end
			if st.Status == "completed" || st.Status == "failed" || st.Status == "cancelled" {
				fin := map[string]any{"status": st.Status, "phase": st.Phase, "duration": st.Duration}
				b, _ := json.Marshal(fin)
				_ = writeEvent("end", string(b))
				return
			}
		}

		// Follow mode: poll for new steps and status
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		var lastLogPreview string
		for {
			if time.Since(start) > maxDur {
				_ = writeEvent("end", `{"status":"timeout"}`)
				return
			}
			select {
			case <-ticker.C:
				st, err := h.getStatus(id)
				if err != nil || st == nil {
					if !writeEvent("ping", `{"ok":false}`) {
						return
					}
					continue
				}
				// Phase/status updates as event
				meta := map[string]any{"status": st.Status, "phase": st.Phase, "duration": st.Duration, "overdue": st.Overdue}
				if b, e := json.Marshal(meta); e == nil {
					if !writeEvent("meta", string(b)) {
						return
					}
				}

				// Optional: stream last job log preview if available and changed
				if st.LastJob != nil && st.LastJob.AllocID != "" {
					task := taskForJob(st.LastJob.JobName)
					if preview := tailAllocLogs(st.LastJob.AllocID, task, 50); preview != "" && preview != lastLogPreview {
						payload := map[string]any{"task": task, "preview": preview}
						if b, e := json.Marshal(payload); e == nil {
							if !writeEvent("log", string(b)) {
								return
							}
							lastLogPreview = preview
						}
					}
				}
				// Stream new steps
				if len(st.Steps) > lastCount {
					for i := lastCount; i < len(st.Steps); i++ {
						b, _ := json.Marshal(st.Steps[i])
						if !writeEvent("step", string(b)) {
							return
						}
					}
					lastCount = len(st.Steps)
				}
				// Terminal?
				if st.Status == "completed" || st.Status == "failed" || st.Status == "cancelled" {
					b, _ := json.Marshal(meta)
					_ = writeEvent("end", string(b))
					return
				}
			default:
				// Best-effort CPU yield
				time.Sleep(10 * time.Millisecond)
			}
		}
	})
	return nil
}

// tailAllocLogs fetches a short preview of allocation logs using the VPS job manager wrapper.
// Returns empty string on any error.
func tailAllocLogs(allocID, task string, lines int) string {
	mgr := os.Getenv("NOMAD_JOB_MANAGER")
	if mgr == "" {
		mgr = "/opt/hashicorp/bin/nomad-job-manager.sh"
	}
	if _, err := os.Stat(mgr); err != nil {
		return ""
	}
	if task == "" {
		task = "api"
	}
	if lines <= 0 {
		lines = 50
	}
	cmd := exec.Command(mgr, "logs", "--alloc-id", allocID, "--task", task, "--both", "--lines", fmt.Sprintf("%d", lines))
	out, err := cmd.CombinedOutput()
	if err != nil {
		return ""
	}
	s := string(out)
	if len(s) > 4000 {
		s = s[len(s)-4000:]
	}
	return s
}

// DebugNomad returns recent Nomad job diagnostics (allocs and evaluation summary) for troubleshooting
func (h *Handler) DebugNomad(c *fiber.Ctx) error {
	if os.Getenv("PLOY_DEBUG") != "1" {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": fiber.Map{"code": "forbidden", "message": "debug endpoint disabled"}})
	}
	// Use the job manager wrapper to list recent jobs known to transflow (prefix heuristics)
	// Fallback: scan logs for runID is non-trivial here; instead, list recent jobs by prefix
	type JobInfo struct {
		Name        string                 `json:"name"`
		AllocCount  int                    `json:"alloc_count"`
		AllocStates map[string]int         `json:"alloc_states"`
		LastEval    map[string]interface{} `json:"last_eval,omitempty"`
		Error       string                 `json:"error,omitempty"`
	}
	jobs := []JobInfo{}

	// Helper to run job-manager wrapper and parse JSON
	run := func(args ...string) ([]byte, error) {
		mgr := os.Getenv("NOMAD_JOB_MANAGER")
		if mgr == "" {
			mgr = "/opt/hashicorp/bin/nomad-job-manager.sh"
		}
		if _, err := os.Stat(mgr); err != nil {
			return nil, fmt.Errorf("job manager not available")
		}
		cmd := exec.Command(mgr, args...)
		out, err := cmd.CombinedOutput()
		if err != nil {
			return nil, fmt.Errorf("%v: %s", err, string(out))
		}
		return out, nil
	}

	// Heuristic: scan last 50 jobs via job-manager 'jobs --format json' then filter
	out, err := run("jobs", "--format", "json")
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": fiber.Map{"code": "internal_error", "message": "jobs listing failed", "details": err.Error()}})
	}
	var jmJobs []map[string]interface{}
	_ = json.Unmarshal(out, &jmJobs)
	candidates := []string{}
	cutoff := time.Now().Add(-24 * time.Hour)
	for _, j := range jmJobs {
		name, _ := j["Name"].(string)
		submitTimeAny := j["SubmitTime"]
		var recent bool = true
		switch t := submitTimeAny.(type) {
		case float64:
			// milliseconds
			ts := time.UnixMilli(int64(t))
			recent = ts.After(cutoff)
		}
		if recent && (strings.HasPrefix(name, "orw-apply-") || strings.Contains(strings.ToLower(name), "transflow") || strings.HasPrefix(name, "transflow-llm-exec") || strings.HasPrefix(name, "transflow-planner") || strings.HasPrefix(name, "transflow-reducer")) {
			candidates = append(candidates, name)
		}
	}
	// Limit candidates
	if len(candidates) > 30 {
		candidates = candidates[len(candidates)-30:]
	}

	for _, jobName := range candidates {
		ji := JobInfo{Name: jobName, AllocStates: map[string]int{}}
		// Get allocs for job
		aout, aerr := run("allocs", "--job", jobName, "--format", "json")
		if aerr == nil {
			var allocs []map[string]interface{}
			_ = json.Unmarshal(aout, &allocs)
			ji.AllocCount = len(allocs)
			for _, a := range allocs {
				st, _ := a["ClientStatus"].(string)
				ji.AllocStates[st] = ji.AllocStates[st] + 1
			}
		} else {
			ji.Error = aerr.Error()
		}
		// Evaluations via Nomad HTTP API
		// Best effort: http://127.0.0.1:4646/v1/job/<job>/evaluations
		func() {
			client := &http.Client{Timeout: 5 * time.Second}
			req, _ := http.NewRequest(http.MethodGet, fmt.Sprintf("http://127.0.0.1:4646/v1/job/%s/evaluations", jobName), nil)
			resp, err := client.Do(req)
			if err != nil {
				return
			}
			defer resp.Body.Close()
			var evals []map[string]interface{}
			if err := json.NewDecoder(resp.Body).Decode(&evals); err != nil {
				return
			}
			if len(evals) == 0 {
				return
			}
			last := evals[len(evals)-1]
			ji.LastEval = map[string]interface{}{
				"Status":         last["Status"],
				"TriggeredBy":    last["TriggeredBy"],
				"Class":          last["Class"],
				"NodesEvaluated": last["NodesEvaluated"],
				"NodesFiltered":  last["NodesFiltered"],
				"FailedTGAllocs": last["FailedTGAllocs"],
			}
		}()

		jobs = append(jobs, ji)
	}

	return c.JSON(fiber.Map{"recent_jobs": jobs, "count": len(jobs)})
}

// taskForJob maps a job name to its task name for log tailing
func taskForJob(jobName string) string {
	n := strings.ToLower(jobName)
	switch {
	case strings.Contains(n, "orw-apply"):
		return "openrewrite-apply"
	case strings.Contains(n, "planner"):
		return "planner"
	case strings.Contains(n, "reducer"):
		return "reducer"
	case strings.Contains(n, "llm-exec") || strings.Contains(n, "llm_exec"):
		return "llm-exec"
	default:
		return "api"
	}
}
