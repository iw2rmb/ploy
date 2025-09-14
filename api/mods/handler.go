package mods

import (
	"bufio"
	"bytes"
	"context"
	crsha1 "crypto/sha1"
	"encoding/hex"
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
	"github.com/iw2rmb/ploy/internal/git/provider"
	mods "github.com/iw2rmb/ploy/internal/mods"
	"github.com/iw2rmb/ploy/internal/orchestration"
	internalStorage "github.com/iw2rmb/ploy/internal/storage"
	nomadtpl "github.com/iw2rmb/ploy/platform/nomad/mods"
	"gopkg.in/yaml.v3"
)

// Handler provides HTTP endpoints for mod operations
type Handler struct {
	gitProvider    provider.GitProvider
	storage        internalStorage.Storage
	statusStore    orchestration.KV
	arfRegistryURL string
	arfMavenGroup  string
}

// NewHandler creates a new Mods HTTP handler
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

// RegisterRoutes registers Mods routes with the Fiber app
func (h *Handler) RegisterRoutes(app *fiber.App) {
	tf := app.Group("/v1/mods")

	// Mod execution
	tf.Post("", h.RunMod)
	tf.Get("/:id/status", h.GetModStatus)
	tf.Get("", h.ListMods)
	tf.Delete("/:id", h.CancelMod)
	tf.Get("/:id/artifacts", h.GetArtifacts)
	tf.Get("/:id/artifacts/:name", h.DownloadArtifact)
	// Real-time events push endpoint
	tf.Post("/:id/events", h.ReportEvent)
	// Logs streaming (SSE stub)
	tf.Get("/:id/logs", h.StreamLogs)
	// Debug: Nomad recent job diagnostics (dev only)
	tf.Get("/debug/nomad", h.DebugNomad)
}

// ModRunRequest represents the request body for running a mod
type ModRunRequest struct {
	Config     string                 `json:"config,omitempty"`      // YAML config as string
	ConfigData map[string]interface{} `json:"config_data,omitempty"` // Or as structured data
	TestMode   bool                   `json:"test_mode,omitempty"`
}

// ModStatus represents the status of a mod execution
type ModStatus struct {
	ID        string                 `json:"id"`
	Status    string                 `json:"status"`
	StartTime time.Time              `json:"start_time"`
	EndTime   *time.Time             `json:"end_time,omitempty"`
	Error     string                 `json:"error,omitempty"`
	Result    map[string]interface{} `json:"result,omitempty"`
	// Enriched runtime fields
	Phase    string          `json:"phase,omitempty"`
	Overdue  bool            `json:"overdue,omitempty"`
	Duration string          `json:"duration,omitempty"`
	Steps    []ModStepStatus `json:"steps,omitempty"`
	LastJob  *ModLastJob     `json:"last_job,omitempty"`
}

// ModStepStatus represents a single step update with timestamp
type ModStepStatus struct {
	Step    string    `json:"step,omitempty"`
	Phase   string    `json:"phase,omitempty"`
	Level   string    `json:"level,omitempty"`
	Message string    `json:"message,omitempty"`
	Time    time.Time `json:"time"`
}

// ModLastJob captures metadata about the most recent submitted Nomad job
type ModLastJob struct {
	JobName     string    `json:"job_name,omitempty"`
	AllocID     string    `json:"alloc_id,omitempty"`
	SubmittedAt time.Time `json:"submitted_at"`
}

// RunMod handles POST /v1/mods
func (h *Handler) RunMod(c *fiber.Ctx) error {
	var req ModRunRequest
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
	var config *mods.ModConfig
	if req.Config != "" {
		// Parse YAML string
		if err := yaml.Unmarshal([]byte(req.Config), &config); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": fiber.Map{
					"code":    "invalid_config",
					"message": "Failed to parse mods configuration",
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
					"message": "Failed to parse mods configuration",
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
	modID := fmt.Sprintf("tf-%s", uuid.New().String()[:8])

	// Store initial status
	status := ModStatus{
		ID:        modID,
		Status:    "initializing",
		StartTime: time.Now(),
		Phase:     "init",
	}
	if err := h.storeStatus(status); err != nil {
		log.Printf("Failed to store initial status: %v", err)
	}

	// Execute mod asynchronously
	go h.executeMod(modID, config, req.TestMode)

	// Return immediate response
	return c.Status(fiber.StatusAccepted).JSON(fiber.Map{
		"mod_id":     modID,
		"status":     "initializing",
		"message":    "Mod execution started",
		"status_url": fmt.Sprintf("/v1/mods/%s/status", modID),
	})
}

// executeMod runs the workflow asynchronously
func (h *Handler) executeMod(modID string, config *mods.ModConfig, testMode bool) {
	// Top-level guard: always convert panics to a terminal failure status
	defer func() {
		if r := recover(); r != nil {
			h.recordError(modID, fmt.Errorf("mod execution panic: %v", r))
		}
	}()

	// Top-level execution timeout to ensure terminal status is written
	execTimeout := 45 * time.Minute
	if v := os.Getenv("PLOY_MODS_EXEC_TIMEOUT"); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			execTimeout = d
		}
	}
	parentCtx := context.Background()
	ctx, cancel := context.WithTimeout(parentCtx, execTimeout)
	defer cancel()

	// Update status to running
	status := ModStatus{
		ID:        modID,
		Status:    "running",
		StartTime: time.Now(),
		Phase:     "running",
	}
	if err := h.storeStatus(status); err != nil {
		log.Printf("Failed to update status: %v", err)
	}

	// Create temp directory for workflow
	tempDir, err := os.MkdirTemp("", fmt.Sprintf("mods-%s-*", modID))
	if err != nil {
		h.recordError(modID, fmt.Errorf("failed to create temp directory: %w", err))
		return
	}
	defer func() { _ = os.RemoveAll(tempDir) }()

	// Prepare workspace structure and write templates for mod runner
	jobsDir := filepath.Join(tempDir, "roadmap", "mods", "jobs")
	if err := os.MkdirAll(jobsDir, 0755); err != nil {
		h.recordError(modID, fmt.Errorf("failed to create jobs dir: %w", err))
		return
	}
	// Write embedded HCL templates
	if err := os.WriteFile(filepath.Join(jobsDir, "planner.hcl"), nomadtpl.GetPlannerTemplate(), 0644); err != nil {
		h.recordError(modID, fmt.Errorf("failed to write planner.hcl: %w", err))
		return
	}
	if err := os.WriteFile(filepath.Join(jobsDir, "llm_exec.hcl"), nomadtpl.GetLLMExecTemplate(), 0644); err != nil {
		h.recordError(modID, fmt.Errorf("failed to write llm_exec.hcl: %w", err))
		return
	}
	if err := os.WriteFile(filepath.Join(jobsDir, "orw_apply.hcl"), nomadtpl.GetORWApplyTemplate(), 0644); err != nil {
		h.recordError(modID, fmt.Errorf("failed to write orw_apply.hcl: %w", err))
		return
	}
	if err := os.WriteFile(filepath.Join(jobsDir, "reducer.hcl"), nomadtpl.GetReducerTemplate(), 0644); err != nil {
		h.recordError(modID, fmt.Errorf("failed to write reducer.hcl: %w", err))
		return
	}

	// Write config to temp file
	configPath := filepath.Join(tempDir, "mods.yaml")
	configBytes, _ := yaml.Marshal(config)
	if err := os.WriteFile(configPath, configBytes, 0644); err != nil {
		h.recordError(modID, fmt.Errorf("failed to write config file: %w", err))
		return
	}

	// Create mod integrations (prefer explicit controller URL if provided)
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
	integrations := mods.NewModIntegrationsWithTestMode(controllerURL, tempDir, testMode)

	// Create configured runner
	runner, err := integrations.CreateConfiguredRunner(config)
	if err != nil {
		h.recordError(modID, fmt.Errorf("failed to create runner: %w", err))
		return
	}

	// Wire event reporter for real-time observability
	reporter := mods.NewControllerEventReporter(controllerURL, modID)
	runner.SetEventReporter(reporter)

	// Expose controller and execution ID to job templates for in-job event pushes
	_ = os.Setenv("PLOY_CONTROLLER", controllerURL)
	_ = os.Setenv("MOD_ID", modID)
	// Expose SeaweedFS URL default for task-side uploads
	if os.Getenv("PLOY_SEAWEEDFS_URL") == "" {
		_ = os.Setenv("PLOY_SEAWEEDFS_URL", "http://seaweedfs-filer.service.consul:8888")
	}

	// Image override: use MODS_ORW_APPLY_IMAGE only (set in API env when needed)

	// Execute the workflow with timeout awareness; ensure terminal status on any error
	var (
		result *mods.ModResult
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
				if persisted, err := h.persistArtifacts(modID, tempDir); err != nil {
					log.Printf("[Mod] Warning: artifact persistence on failure failed: %v", err)
				} else if repo := config.TargetRepo; repo != "" {
					if key, ok := persisted["source_sbom"]; ok {
						h.recordLatestSBOM(repo, key, "", modID)
					}
					if key, ok := persisted["sbom"]; ok {
						h.recordLatestSBOM(repo, key, "", modID)
					}
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
			h.recordError(modID, runErr)
			return
		}
		// continue to success handling below
	case <-ctx.Done():
		// Timeout/cancellation — record a terminal error
		h.recordError(modID, fmt.Errorf("mod execution exceeded max duration (%s)", execTimeout))
		return
	}

	// Store successful result (preserve accumulated steps/phase/last_job)
	endTime := time.Now()

	// Persist known artifacts (best-effort)
	artifacts := map[string]string{}
	if h.storage != nil {
		if persisted, err := h.persistArtifacts(modID, tempDir); err == nil {
			artifacts = persisted
			// Record latest SBOM pointer if available
			if repo := config.TargetRepo; repo != "" {
				if key, ok := artifacts["source_sbom"]; ok {
					h.recordLatestSBOM(repo, key, result.CommitSHA, modID)
				}
				if key, ok := artifacts["sbom"]; ok {
					h.recordLatestSBOM(repo, key, result.CommitSHA, modID)
				}
			}
		} else {
			log.Printf("[Mod] Warning: artifact persistence failed: %v", err)
		}
	}
	// Load current status to preserve steps and phase
	prevStatus, _ := h.getStatus(modID)
	var prevSteps []ModStepStatus
	var prevPhase string
	var prevLastJob *ModLastJob
	if prevStatus != nil {
		prevSteps = prevStatus.Steps
		prevPhase = prevStatus.Phase
		prevLastJob = prevStatus.LastJob
	}
	status = ModStatus{
		ID:        modID,
		Status:    "completed",
		StartTime: status.StartTime,
		EndTime:   &endTime,
		Phase:     prevPhase,
		Steps:     prevSteps,
		LastJob:   prevLastJob,
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

// GetModStatus handles GET /v1/mods/:id/status and enriches running statuses with duration/overdue
// (See bottom of file for the handler that returns status and includes runtime enrichment.)

// recordError records an error status for the execution
func (h *Handler) recordError(modID string, err error) {
	endTime := time.Now()
	// Preserve existing status (steps, phase, start time) if available
	var status *ModStatus
	if st, getErr := h.getStatus(modID); getErr == nil && st != nil {
		status = st
	} else {
		status = &ModStatus{ID: modID, StartTime: endTime}
	}
	status.Status = "failed"
	status.EndTime = &endTime
	status.Error = err.Error()
	if storeErr := h.storeStatus(*status); storeErr != nil {
		log.Printf("Failed to store error status: %v", storeErr)
	}
}

// persistArtifacts scans the temp workspace for known Mods artifacts and uploads them to storage.
// Returns a map of artifact logical names to storage keys.
func (h *Handler) persistArtifacts(modID, tempDir string) (map[string]string, error) {
	artifacts := map[string]string{}
	if h.storage == nil {
		return artifacts, nil
	}
	ctx := context.Background()
	// Scan for any SBOMs generated by jobs and persist
	_ = filepath.Walk(tempDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info == nil || info.IsDir() {
			return nil
		}
		if strings.HasSuffix(strings.ToLower(info.Name()), ".sbom.json") {
			key := fmt.Sprintf("artifacts/mods/%s/%s", modID, info.Name())
			f, _ := os.Open(path)
			defer func() { _ = f.Close() }()
			if err := h.storage.Put(ctx, key, f, internalStorage.WithContentType("application/json")); err == nil {
				// Best-effort: normalize a logical name for source/container if detectable
				lname := "sbom"
				n := strings.ToLower(info.Name())
				if strings.Contains(n, "source") || n == ".sbom.json" {
					lname = "source_sbom"
				}
				if strings.Contains(n, "container") {
					lname = "container_sbom"
				}
				artifacts[lname] = key
			}
			// Do not return error to allow other artifacts
			return nil
		}
		return nil
	})
	// Planner plan.json
	planPath := filepath.Join(tempDir, "planner", "out", "plan.json")
	if fi, err := os.Stat(planPath); err == nil && !fi.IsDir() {
		key := fmt.Sprintf("artifacts/mods/%s/plan.json", modID)
		f, _ := os.Open(planPath)
		defer func() { _ = f.Close() }()
		if err := h.storage.Put(ctx, key, f, internalStorage.WithContentType("application/json")); err == nil {
			artifacts["plan_json"] = key
		}
	}
	// Reducer next.json
	nextPath := filepath.Join(tempDir, "reducer", "out", "next.json")
	if fi, err := os.Stat(nextPath); err == nil && !fi.IsDir() {
		key := fmt.Sprintf("artifacts/mods/%s/next.json", modID)
		f, _ := os.Open(nextPath)
		defer func() { _ = f.Close() }()
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
			keyPrimary := fmt.Sprintf("artifacts/mods/%s/diff.patch", modID)
			keyAlt := fmt.Sprintf("mods/%s/diff.patch", modID)
			// If already present (task-side upload), record and skip
			if ok, _ := h.storage.Exists(ctx, keyPrimary); ok {
				artifacts["diff_patch"] = keyPrimary
			} else if ok2, _ := h.storage.Exists(ctx, keyAlt); ok2 {
				artifacts["diff_patch"] = keyAlt
			} else {
				f, _ := os.Open(path)
				defer func() { _ = f.Close() }()
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
			key := fmt.Sprintf("artifacts/mods/%s/error.log", modID)
			if ok, _ := h.storage.Exists(ctx, key); ok {
				artifacts["error_log"] = key
			} else {
				f, _ := os.Open(path)
				defer func() { _ = f.Close() }()
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
		keyPrimary := fmt.Sprintf("artifacts/mods/%s/diff.patch", modID)
		keyAlt := fmt.Sprintf("mods/%s/diff.patch", modID)
		if ok, _ := h.storage.Exists(ctx, keyPrimary); ok {
			artifacts["diff_patch"] = keyPrimary
		} else if ok2, _ := h.storage.Exists(ctx, keyAlt); ok2 {
			artifacts["diff_patch"] = keyAlt
		}
	}
	return artifacts, nil
}

// recordLatestSBOM writes a pointer file under mods/sbom/latest/<repo-hash>.json
func (h *Handler) recordLatestSBOM(repo, storageKey, sha, execID string) {
	if h.storage == nil || repo == "" || storageKey == "" {
		return
	}
	sum := crsha1.Sum([]byte(repo))
	slug := hex.EncodeToString(sum[:])
	data := map[string]interface{}{
		"repo":        repo,
		"sha":         sha,
		"storage_key": storageKey,
		"mod_id":      execID,
		"updated_at":  time.Now().UTC().Format(time.RFC3339),
	}
	b, _ := json.Marshal(data)
	now := time.Now().UTC().Format(time.RFC3339)
	latestKey := fmt.Sprintf("mods/sbom/latest/%s.json", slug)
	_ = h.storage.Put(context.Background(), latestKey, bytes.NewReader(b), internalStorage.WithContentType("application/json"))
	// Also append history entry for discoverability
	histKey := fmt.Sprintf("mods/sbom/history/%s/%s.json", slug, now)
	_ = h.storage.Put(context.Background(), histKey, bytes.NewReader(b), internalStorage.WithContentType("application/json"))
}

// GetModStatus handles GET /v1/mods/:id/status
func (h *Handler) GetModStatus(c *fiber.Ctx) error {
	modID := c.Params("id")
	if modID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": fiber.Map{
				"code":    "missing_id",
				"message": "Execution ID is required",
			},
		})
	}

	// Retrieve status from store
	status, err := h.getStatus(modID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": fiber.Map{
				"code":    "not_found",
				"message": fmt.Sprintf("Mod execution %s not found", modID),
			},
		})
	}

	// Enrich statuses: add duration and overdue fields without changing stored state
	if status.Status == "running" {
		elapsed := time.Since(status.StartTime)
		status.Duration = elapsed.String()
		overdueThresh := 30 * time.Minute
		if v := os.Getenv("PLOY_MODS_OVERDUE"); v != "" {
			if d, e := time.ParseDuration(v); e == nil && d > 0 {
				overdueThresh = d
			}
		}
		status.Overdue = elapsed > overdueThresh
	} else if (status.Status == "completed" || status.Status == "failed" || status.Status == "cancelled") && status.EndTime != nil {
		// Compute duration for terminal states if not already set
		if status.Duration == "" {
			status.Duration = status.EndTime.Sub(status.StartTime).String()
		}
	}

	return c.JSON(status)
}

// ListMods handles GET /v1/mods
func (h *Handler) ListMods(c *fiber.Ctx) error {
	// List all mod executions from the status store
	prefix := "mods/status/"
	keys, err := h.statusStore.Keys(prefix, "")
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": fiber.Map{
				"code":    "storage_error",
				"message": "Failed to list mod executions",
				"details": err.Error(),
			},
		})
	}

	executions := []ModStatus{}
	for _, key := range keys {
		data, err := h.statusStore.Get(key)
		if err != nil {
			continue
		}

		var status ModStatus
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

// CancelMod handles DELETE /v1/mods/:id
func (h *Handler) CancelMod(c *fiber.Ctx) error {
	modID := c.Params("id")
	if modID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": fiber.Map{
				"code":    "missing_id",
				"message": "Execution ID is required",
			},
		})
	}

	// Check if execution exists
	status, err := h.getStatus(modID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": fiber.Map{
				"code":    "not_found",
				"message": fmt.Sprintf("Mod execution %s not found", modID),
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
		"message": fmt.Sprintf("Mod execution %s cancelled", modID),
		"status":  status,
	})
}

// storeStatus stores the status in the KV store
func (h *Handler) storeStatus(status ModStatus) error {
	if h.statusStore == nil {
		return nil // Silently skip if no store configured
	}

	key := fmt.Sprintf("mods/status/%s", status.ID)
	data, err := json.Marshal(status)
	if err != nil {
		return err
	}

	return h.statusStore.Put(key, data)
}

// getStatus retrieves the status from the KV store
func (h *Handler) getStatus(modID string) (*ModStatus, error) {
	if h.statusStore == nil {
		return nil, fmt.Errorf("status store not configured")
	}

	key := fmt.Sprintf("mods/status/%s", modID)
	data, err := h.statusStore.Get(key)
	if err != nil {
		return nil, err
	}

	var status ModStatus
	if err := json.Unmarshal([]byte(data), &status); err != nil {
		return nil, err
	}

	return &status, nil
}

// ModEvent represents a real-time event emitted by runner/jobs
type ModEvent struct {
	ExecutionID string    `json:"mod_id"`
	Phase       string    `json:"phase,omitempty"`
	Step        string    `json:"step,omitempty"`
	Level       string    `json:"level,omitempty"`
	Message     string    `json:"message,omitempty"`
	Time        time.Time `json:"ts,omitempty"`
	JobName     string    `json:"job_name,omitempty"`
	AllocID     string    `json:"alloc_id,omitempty"`
}

// ReportEvent handles POST /v1/mods/:id/events to update live status metadata
func (h *Handler) ReportEvent(c *fiber.Ctx) error {
	if h.statusStore == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error": fiber.Map{"code": "storage_disabled", "message": "status store not configured"},
		})
	}
	var ev ModEvent
	if err := c.BodyParser(&ev); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": fiber.Map{"code": "invalid_event", "message": "failed to parse event", "details": err.Error()},
		})
	}
	if ev.ExecutionID == "" {
		// Allow path param to carry execution id when payload omits it
		ev.ExecutionID = c.Params("id")
	}
	if ev.ExecutionID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": fiber.Map{"code": "missing_mod_id", "message": "mod_id is required"},
		})
	}
	// Load or initialize status
	st, err := h.getStatus(ev.ExecutionID)
	if err != nil || st == nil || st.ID == "" {
		now := time.Now()
		st = &ModStatus{ID: ev.ExecutionID, Status: "running", StartTime: now}
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
		st.Steps = append(st.Steps, ModStepStatus{
			Step:    ev.Step,
			Phase:   ev.Phase,
			Level:   ev.Level,
			Message: ev.Message,
			Time:    ts,
		})
	}
	// Last job metadata if provided
	if ev.JobName != "" {
		st.LastJob = &ModLastJob{JobName: ev.JobName, AllocID: ev.AllocID, SubmittedAt: ts}
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
	// Validate artifact path safety and prefix
	if !validTransflowArtifactKey(id, key) {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": fiber.Map{"code": "invalid_artifact_key", "message": "artifact key failed validation"}})
	}
	reader, err := h.storage.Get(c.Context(), key)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": fiber.Map{"code": "storage_error", "message": err.Error()}})
	}
	defer func() { _ = reader.Close() }()
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

// validTransflowArtifactKey enforces prefix and path safety for artifact keys.
func validTransflowArtifactKey(id, key string) bool {
	if id == "" || key == "" {
		return false
	}
	prefix := fmt.Sprintf("artifacts/mods/%s/", id)
	if !strings.HasPrefix(key, prefix) {
		return false
	}
	if strings.Contains(key, "..") {
		return false
	}
	if strings.Contains(key, "\\") {
		return false
	}
	return true
}

// StreamLogs provides a basic Server-Sent Events (SSE) stub for live mod logs.
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
	if v := os.Getenv("PLOY_MODS_SSE_INTERVAL"); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			interval = d
		}
	}
	// Optional time cap
	maxDur := 30 * time.Minute
	if v := os.Getenv("PLOY_MODS_SSE_TIMEOUT"); v != "" {
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
	// Use the job manager wrapper to list recent jobs known to mod (prefix heuristics)
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
		recent := true
		switch t := submitTimeAny.(type) {
		case float64:
			// milliseconds
			ts := time.UnixMilli(int64(t))
			recent = ts.After(cutoff)
		}
		if recent && (strings.HasPrefix(name, "orw-apply-") || strings.Contains(strings.ToLower(name), "mod") || strings.HasPrefix(name, "mod-llm-exec") || strings.HasPrefix(name, "mod-planner") || strings.HasPrefix(name, "mod-reducer")) {
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
			defer func() { _ = resp.Body.Close() }()
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
