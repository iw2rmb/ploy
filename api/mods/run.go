package mods

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	mods "github.com/iw2rmb/ploy/internal/mods"
	nomadtpl "github.com/iw2rmb/ploy/platform/nomad/mods"
	"gopkg.in/yaml.v3"
)

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
	modID := fmt.Sprintf("mod-%s", uuid.New().String()[:8])

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
		_ = os.Setenv("PLOY_SEAWEEDFS_URL", "http://seaweedfs-filer.storage.ploy.local:8888")
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
			var failedArtifacts map[string]string
			if h.storage != nil {
				if persisted, err := h.persistArtifacts(modID, tempDir); err != nil {
					log.Printf("[Mod] Warning: artifact persistence on failure failed: %v", err)
				} else {
					failedArtifacts = persisted
					if repo := config.TargetRepo; repo != "" {
						if key, ok := persisted["source_sbom"]; ok {
							h.recordLatestSBOM(repo, key, "", modID)
						}
						if key, ok := persisted["sbom"]; ok {
							h.recordLatestSBOM(repo, key, "", modID)
						}
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
			// Build and store a failed status that includes any discovered artifacts for diagnostics
			endTime := time.Now()
			prevStatus, _ := h.getStatus(modID)
			status := ModStatus{ID: modID, StartTime: endTime}
			if prevStatus != nil {
				status = *prevStatus
			}
			status.Status = "failed"
			status.EndTime = &endTime
			status.Error = runErr.Error()
			// Attach artifacts if we found any during failure path
			if len(failedArtifacts) > 0 {
				if status.Result == nil {
					status.Result = map[string]interface{}{}
				}
				status.Result["artifacts"] = failedArtifacts
			}
			if err := h.storeStatus(status); err != nil {
				log.Printf("Failed to store error status: %v", err)
			}
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
