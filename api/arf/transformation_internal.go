package arf

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/gofiber/fiber/v2"
)

// GetTransformationResult handles GET /v1/arf/transforms/:id (legacy endpoint)
func (h *Handler) GetTransformationResult(c *fiber.Ctx) error {
	transformID := c.Params("id")
	if transformID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "transformation_id is required",
		})
	}

	// Get transformation status from Consul
	status, err := h.consulStore.GetTransformationStatus(c.Context(), transformID)
	if err != nil || status == nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "transformation not found",
		})
	}

	// Build TransformationResult from status
	result := &TransformationResult{
		TransformationID: status.TransformationID,
		RecipeID:         status.RecipeID,
		StartTime:        status.StartTime,
		EndTime:          status.EndTime,
		Success:          status.Status == "completed",
		ChangesApplied:   status.ChangesApplied,
		FilesModified:    status.FilesModified,
		Diff:             status.Diff,
		ValidationScore:  status.ValidationScore,
		ExecutionTime:    status.EndTime.Sub(status.StartTime),
	}

	// Set TotalFiles based on FilesModified
	if result.FilesModified != nil {
		result.TotalFiles = len(result.FilesModified)
	}

	return c.JSON(result)
}

// executeTransformationInternal executes transformation with basic tracking
func (h *Handler) executeTransformationInternal(ctx context.Context, transformID string, req *TransformRequest) (*TransformationResult, error) {
	transformStartTime := time.Now()
	fmt.Printf("[ARF Transform Internal] Starting internal transformation for ID: %s at %v\n", transformID, transformStartTime)

	// Create workspace directory
	workspaceDir := filepath.Join("/tmp", "arf-transformations", transformID)
	if err := os.MkdirAll(workspaceDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create workspace: %w", err)
	}
	defer func() {
		// Clean up workspace after transformation
		os.RemoveAll(workspaceDir)
	}()

	// Clone repository (simplified)
	repoPath := filepath.Join(workspaceDir, "repository")
	fmt.Printf("[DEBUG] [%s] Starting repository cloning stage\n", transformID)
	fmt.Printf("[DEBUG] [%s] Repository URL: %s, Branch: %s, Target: %s\n", transformID, req.Codebase.Repository, req.Codebase.Branch, repoPath)
	fmt.Printf("[DEBUG] [%s] About to call cloneRepositoryWithInfo...\n", transformID)

	if err := h.cloneRepository(req.Codebase.Repository, req.Codebase.Branch, repoPath); err != nil {
		return nil, fmt.Errorf("failed to clone repository: %w", err)
	}

	// Execute recipe using the recipe executor
	if h.recipeExecutor == nil {
		return nil, fmt.Errorf("recipe executor not available")
	}

	result, err := h.recipeExecutor.ExecuteRecipeByID(ctx, req.RecipeID, repoPath, req.Type, transformID)
	if err != nil {
		// Check if this is a "recipe not found" error and handle appropriately
		if isNotFoundError(err) {
			return nil, &NotFoundError{Message: fmt.Sprintf("recipe not found: %s", req.RecipeID)}
		}
		return nil, fmt.Errorf("recipe execution failed: %w", err)
	}

	return result, nil
}

// cloneRepository clones a git repository to the specified path
func (h *Handler) cloneRepository(repoURL, branch, targetPath string) error {
	args := []string{"clone", "--depth=1"}
	if branch != "" {
		args = append(args, "--branch", branch)
	}
	args = append(args, repoURL, targetPath)

	fmt.Printf("[DEBUG] Git command: git %v\n", args)
	cmd := exec.Command("git", args...)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("git clone failed: %w", err)
	}
	return nil
}

// Helper function to check if error is a not found error
func isNotFoundError(err error) bool {
	return err != nil && (err.Error() == "recipe not found" ||
		err.Error() == "recipe not available" ||
		err.Error() == "no matching recipe")
}
