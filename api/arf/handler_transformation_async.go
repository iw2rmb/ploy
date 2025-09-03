package arf

import (
    "context"
    "fmt"
    "time"

    "github.com/gofiber/fiber/v2"
    "github.com/google/uuid"
    "github.com/iw2rmb/ploy/api/arf/models"
    "strings"
)

// ExecuteTransformationAsync handles POST /v1/arf/transform with async execution
func (h *Handler) ExecuteTransformationAsync(c *fiber.Ctx) error {
    var req TransformRequest
    if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error":   "Invalid request format",
			"details": err.Error(),
		})
	}

	// Validate required fields
	if req.RecipeID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "recipe_id is required",
		})
	}

	if req.Codebase.Repository == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "codebase.repository is required",
		})
	}

	// Set default branch if not specified
	if req.Codebase.Branch == "" {
		req.Codebase.Branch = "main"
	}

	// Require explicit type specification
	if req.Type == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "recipe type is required - specify 'openrewrite' or other valid type",
		})
    }

    // Optional: Validate recipe_id against catalog (suggestions on 400)
    if h.catalog != nil && req.Type == "openrewrite" {
        if _, err := h.catalog.GetRecipe(c.Context(), req.RecipeID); err != nil {
            // Build suggestions using catalog search (by full ID and last segment)
            suggestions := []string{}
            collect := func(items []*models.Recipe) {
                for _, r := range items {
                    id := r.ID
                    // Fallback if ID missing
                    if id == "" {
                        id = r.Metadata.Name
                    }
                    if id == "" {
                        continue
                    }
                    // Deduplicate
                    exists := false
                    for _, s := range suggestions {
                        if s == id {
                            exists = true
                            break
                        }
                    }
                    if !exists {
                        suggestions = append(suggestions, id)
                        if len(suggestions) >= 5 {
                            return
                        }
                    }
                }
            }

            // Search by full value
            if list, e := h.catalog.SearchRecipes(c.Context(), req.RecipeID); e == nil {
                collect(list)
            }
            // Search by last segment (after last dot)
            if dot := strings.LastIndex(req.RecipeID, "."); dot != -1 {
                seg := req.RecipeID[dot+1:]
                if seg != "" {
                    if list, e := h.catalog.SearchRecipes(c.Context(), seg); e == nil {
                        collect(list)
                    }
                }
            }

            return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
                "error":       "invalid recipe_id",
                "message":     "Recipe not found in catalog",
                "recipe_id":   req.RecipeID,
                "suggestions": suggestions,
            })
        }
    }

    // Generate transformation ID
    transformID := uuid.New().String()

	// Consul store is required for async transformations
	if h.consulStore == nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Async transformations require Consul store to be configured",
		})
	}

	// Store initial status in Consul immediately
	initialStatus := &TransformationStatus{
		TransformationID: transformID,
		Status:           "initiated",
		WorkflowStage:    req.Type, // Start with recipe type as initial stage
		StartTime:        time.Now(),
		Children:         []HealingAttempt{},
	}

	if err := h.consulStore.StoreTransformationStatus(c.Context(), transformID, initialStatus); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error":   "Failed to initialize transformation",
			"details": err.Error(),
		})
	}

	// Start background execution
	go h.executeTransformationBackground(transformID, &req)

	// Return immediately with status link
	return c.JSON(fiber.Map{
		"transformation_id": transformID,
		"status":            "initiated",
		"status_url":        fmt.Sprintf("/v1/arf/transforms/%s/status", transformID),
		"message":           "Transformation started, use status_url to monitor progress",
	})
}

// executeTransformationBackground executes the transformation in the background
func (h *Handler) executeTransformationBackground(transformID string, req *TransformRequest) {
	ctx := context.Background()

	// Update status to in_progress
	status, _ := h.consulStore.GetTransformationStatus(ctx, transformID)
	if status != nil {
		status.Status = "in_progress"
		status.WorkflowStage = "openrewrite"
		h.consulStore.StoreTransformationStatus(ctx, transformID, status)
	}

	// Execute transformation using existing internal method
	result, err := h.executeTransformationInternal(ctx, transformID, req)

	// Update final status with all result data
	if status != nil {
		if err != nil {
			status.Status = "failed"
			status.Error = err.Error()
		} else {
			status.Status = "completed"
			if result != nil {
				// Store all result data directly in status
				status.RecipeID = result.RecipeID
				status.Diff = result.Diff
				status.FilesModified = result.FilesModified
				status.ChangesApplied = result.ChangesApplied
				status.ValidationScore = result.ValidationScore
			}
		}
		status.EndTime = time.Now()
		h.consulStore.StoreTransformationStatus(ctx, transformID, status)
	}
}

// GetTransformationStatusAsync handles GET /v1/arf/transforms/:id/status with Consul
func (h *Handler) GetTransformationStatusAsync(c *fiber.Ctx) error {
	transformID := c.Params("id")
	if transformID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "transformation_id is required",
		})
	}

	// Consul store is required for async transformation status
	if h.consulStore == nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Async transformation status requires Consul store to be configured",
		})
	}

	// Get status from Consul
	status, err := h.consulStore.GetTransformationStatus(c.Context(), transformID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error":   "Failed to retrieve transformation status",
			"details": err.Error(),
		})
	}

	if status == nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "Transformation not found",
		})
	}

	// Calculate and update healing summary
	if len(status.Children) > 0 || status.WorkflowStage == "heal" {
		status.HealingSummary = &HealingSummary{
			TotalAttempts:   status.TotalHealingAttempts,
			ActiveAttempts:  status.ActiveHealingCount,
			SuccessfulHeals: countSuccessfulHeals(status.Children),
			FailedHeals:     countFailedHeals(status.Children),
			MaxDepthReached: calculateMaxDepth(status.Children, 1),
		}

		// Add LLM metrics if coordinator metrics are available
		if status.CoordinatorMetrics != nil {
			status.HealingSummary.TotalLLMCalls = status.CoordinatorMetrics.TotalLLMCalls
			status.HealingSummary.TotalLLMTokens = status.CoordinatorMetrics.TotalLLMTokens
			status.HealingSummary.TotalLLMCost = status.CoordinatorMetrics.TotalLLMCost
			status.HealingSummary.LLMCacheHitRate = status.CoordinatorMetrics.LLMCacheHitRate
		}
	}

	// Add progress information if in progress
	if status.Status == "in_progress" && status.Progress == nil {
		status.Progress = calculateProgress(status)
	}

	// Populate sandbox information
	status.SandboxInfo = h.getSandboxInfo(c.Context(), transformID, status)

	// Add active attempts if any
	if status.ActiveHealingCount > 0 {
		activeAttempts, _ := h.consulStore.GetActiveHealingAttempts(c.Context(), transformID)
		// Enhance children with progress for active attempts
		enhanceActiveAttempts(status.Children, activeAttempts)
	}

	// Add healing coordinator metrics if available
	if h.healingCoordinator != nil && h.healingCoordinator.IsRunning() {
		metrics := h.healingCoordinator.GetMetrics()
		status.CoordinatorMetrics = &metrics
	}

	// Return the complete status structure
	return c.JSON(status)
}

// Helper functions for healing metrics
func countSuccessfulHeals(attempts []HealingAttempt) int {
	count := 0
	for _, attempt := range attempts {
		if attempt.Status == "completed" && attempt.Result == "success" {
			count++
		}
		count += countSuccessfulHeals(attempt.Children)
	}
	return count
}

func countFailedHeals(attempts []HealingAttempt) int {
	count := 0
	for _, attempt := range attempts {
		if attempt.Status == "completed" && attempt.Result == "failed" {
			count++
		}
		count += countFailedHeals(attempt.Children)
	}
	return count
}

func calculateMaxDepth(attempts []HealingAttempt, currentDepth int) int {
	if len(attempts) == 0 {
		return currentDepth - 1
	}

	maxDepth := currentDepth
	for _, attempt := range attempts {
		childDepth := calculateMaxDepth(attempt.Children, currentDepth+1)
		if childDepth > maxDepth {
			maxDepth = childDepth
		}
	}
	return maxDepth
}

// calculateProgress determines the current progress percentage based on workflow stage
func calculateProgress(status *TransformationStatus) *TransformationProgress {
	progress := &TransformationProgress{
		Stage: status.WorkflowStage,
	}

	// Calculate percentage based on workflow stage
	switch status.WorkflowStage {
	case "openrewrite":
		progress.PercentComplete = 25
		progress.Message = "Executing transformation recipe"
	case "build":
		progress.PercentComplete = 50
		progress.Message = "Building transformed code"
	case "deploy":
		progress.PercentComplete = 60
		progress.Message = "Deploying to sandbox environment"
	case "test":
		progress.PercentComplete = 75
		progress.Message = "Running test suites"
	case "heal":
		// For healing, calculate based on completed attempts
		if status.TotalHealingAttempts > 0 {
			completedAttempts := countCompletedAttempts(status.Children)
			progress.PercentComplete = 75 + (25 * completedAttempts / status.TotalHealingAttempts)
			progress.Message = fmt.Sprintf("Healing in progress (%d/%d attempts)", completedAttempts, status.TotalHealingAttempts)
		} else {
			progress.PercentComplete = 80
			progress.Message = "Analyzing errors for healing"
		}
	default:
		progress.PercentComplete = 10
		progress.Message = "Initializing transformation"
	}

	return progress
}

// countCompletedAttempts counts all completed healing attempts recursively
func countCompletedAttempts(attempts []HealingAttempt) int {
	count := 0
	for _, attempt := range attempts {
		if attempt.Status == "completed" {
			count++
		}
		count += countCompletedAttempts(attempt.Children)
	}
	return count
}

// getSandboxInfo retrieves sandbox deployment information for the transformation
func (h *Handler) getSandboxInfo(ctx context.Context, transformID string, status *TransformationStatus) *TransformationSandboxInfo {
	if h.sandboxMgr == nil {
		return nil
	}

	info := &TransformationSandboxInfo{
		HealingSandboxes: []SandboxDeployment{},
	}

	// Get primary sandbox if exists
	if status.WorkflowStage != "openrewrite" && status.WorkflowStage != "" {
		primarySandbox := h.getSandboxDeployment(ctx, transformID, "primary")
		if primarySandbox != nil {
			info.PrimarySandbox = primarySandbox
		}
	}

	// Get healing sandboxes
	if len(status.Children) > 0 {
		info.HealingSandboxes = h.getHealingSandboxes(ctx, status.Children)
	}

	if info.PrimarySandbox == nil && len(info.HealingSandboxes) == 0 {
		return nil
	}

	return info
}

// getSandboxDeployment retrieves deployment info for a specific sandbox
func (h *Handler) getSandboxDeployment(ctx context.Context, transformID, sandboxType string) *SandboxDeployment {
	// This is a placeholder - actual implementation would query sandbox manager
	// For now, return mock data to demonstrate the structure
	sandboxID := fmt.Sprintf("sandbox-%s-%s", transformID[:8], sandboxType)

	return &SandboxDeployment{
		TransformationID: transformID,
		SandboxID:        sandboxID,
		DeploymentURL:    fmt.Sprintf("https://%s.ployd.app", sandboxID),
		BuildStatus:      "success",
		TestStatus:       "in_progress",
		CreatedAt:        time.Now().Add(-10 * time.Minute),
		LastUpdated:      time.Now(),
	}
}

// getHealingSandboxes retrieves sandbox info for healing attempts
func (h *Handler) getHealingSandboxes(ctx context.Context, attempts []HealingAttempt) []SandboxDeployment {
	var sandboxes []SandboxDeployment

	for _, attempt := range attempts {
		if attempt.Status == "in_progress" || attempt.Status == "completed" {
			if attempt.SandboxID != "" {
				sandbox := SandboxDeployment{
					TransformationID: attempt.TransformationID,
					SandboxID:        attempt.SandboxID,
					DeploymentURL:    fmt.Sprintf("https://%s.ployd.app", attempt.SandboxID),
					BuildStatus:      "success",
					TestStatus:       attempt.Status,
					CreatedAt:        attempt.StartTime,
					LastUpdated:      time.Now(),
				}
				sandboxes = append(sandboxes, sandbox)
			}
		}

		// Recursively get sandboxes from children
		childSandboxes := h.getHealingSandboxes(ctx, attempt.Children)
		sandboxes = append(sandboxes, childSandboxes...)
	}

	return sandboxes
}

// enhanceActiveAttempts adds progress information to active healing attempts
func enhanceActiveAttempts(attempts []HealingAttempt, activeIDs []string) {
	activeMap := make(map[string]bool)
	for _, id := range activeIDs {
		activeMap[id] = true
	}

	enhanceAttemptsRecursive(attempts, activeMap)
}

// enhanceAttemptsRecursive recursively adds progress to active attempts
func enhanceAttemptsRecursive(attempts []HealingAttempt, activeMap map[string]bool) {
	for i := range attempts {
		if activeMap[attempts[i].TransformationID] && attempts[i].Status == "in_progress" {
			// Add progress information for active attempts
			if attempts[i].Progress == nil {
				attempts[i].Progress = &TransformationProgress{
					Stage:           "build_validation",
					PercentComplete: 45,
					Message:         "Validating healing transformation",
				}
			}
		}
		// Recursively enhance children
		enhanceAttemptsRecursive(attempts[i].Children, activeMap)
	}
}
