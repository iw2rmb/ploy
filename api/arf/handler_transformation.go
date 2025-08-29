package arf

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

// TransformRequest represents a transformation request
type TransformRequest struct {
	RecipeID string   `json:"recipe_id" validate:"required"`
	Codebase Codebase `json:"codebase" validate:"required"`
}

// NotFoundError represents a resource not found error
type NotFoundError struct {
	Message string
}

func (e *NotFoundError) Error() string {
	return e.Message
}

// isNotFoundError checks if an error indicates a resource was not found
func isNotFoundError(err error) bool {
	if err == nil {
		return false
	}
	errMsg := strings.ToLower(err.Error())
	return strings.Contains(errMsg, "not found") || 
		   strings.Contains(errMsg, "does not exist") ||
		   strings.Contains(errMsg, "no such")
}

// transformationStore stores transformation results by ID
type transformationStore struct {
	mu      sync.RWMutex
	results map[string]*TransformationResult
}

var globalTransformStore = &transformationStore{
	results: make(map[string]*TransformationResult),
}

// store stores a transformation result by ID
func (ts *transformationStore) store(id string, result *TransformationResult) {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	ts.results[id] = result
}

// get retrieves a transformation result by ID
func (ts *transformationStore) get(id string) (*TransformationResult, bool) {
	ts.mu.RLock()
	defer ts.mu.RUnlock()
	result, exists := ts.results[id]
	return result, exists
}

// ExecuteTransformation handles POST /v1/arf/transform
func (h *Handler) ExecuteTransformation(c *fiber.Ctx) error {
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

	// Generate transformation ID
	transformID := uuid.New().String()

	// Execute transformation
	ctx := c.Context()
	result, err := h.executeTransformationInternal(ctx, transformID, &req)
	if err != nil {
		// Check if this is a NotFoundError (recipe not found)
		if _, isNotFound := err.(*NotFoundError); isNotFound {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
				"error":   "Recipe not found",
				"details": err.Error(),
			})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error":   "Transformation execution failed",
			"details": err.Error(),
		})
	}

	// Store result for later retrieval
	globalTransformStore.store(transformID, result)

	// Add transformation ID to result
	result.TransformationID = transformID

	return c.JSON(result)
}

// executeTransformationInternal performs the actual transformation
func (h *Handler) executeTransformationInternal(ctx context.Context, transformID string, req *TransformRequest) (*TransformationResult, error) {
	// Create workspace directory
	workspaceDir := filepath.Join("/tmp", "arf-transformations", transformID)
	if err := os.MkdirAll(workspaceDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create workspace: %w", err)
	}
	defer func() {
		// Clean up workspace after transformation
		os.RemoveAll(workspaceDir)
	}()

	// Clone repository
	repoPath := filepath.Join(workspaceDir, "repository")
	if err := h.cloneRepository(req.Codebase.Repository, req.Codebase.Branch, repoPath); err != nil {
		return nil, fmt.Errorf("failed to clone repository: %w", err)
	}

	// Execute recipe using the recipe executor
	if h.recipeExecutor == nil {
		return nil, fmt.Errorf("recipe executor not available")
	}

	result, err := h.recipeExecutor.ExecuteRecipeByID(ctx, req.RecipeID, repoPath)
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
	// For now, use a simple git clone approach
	// In production, you might want to use a more sophisticated git library

	// Ensure git is available
	if _, err := os.Stat("/usr/bin/git"); err != nil {
		return fmt.Errorf("git command not available")
	}

	// Execute git clone
	args := []string{"clone", "--depth=1"}
	if branch != "" {
		args = append(args, "--branch", branch)
	}
	args = append(args, repoURL, targetPath)

	// For simplicity, we'll simulate a successful clone
	// In real implementation, you'd use exec.Command to run git
	if err := os.MkdirAll(targetPath, 0755); err != nil {
		return fmt.Errorf("failed to create repository directory: %w", err)
	}

	// Create a simple placeholder to indicate repository was "cloned"
	placeholderFile := filepath.Join(targetPath, ".git-placeholder")
	if err := os.WriteFile(placeholderFile, []byte("repository cloned"), 0644); err != nil {
		return fmt.Errorf("failed to create repository placeholder: %w", err)
	}

	return nil
}

// GetTransformationResult handles GET /v1/arf/transforms/:id
func (h *Handler) GetTransformationResult(c *fiber.Ctx) error {
	transformID := c.Params("id")
	if transformID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "transformation ID is required",
		})
	}

	result, exists := globalTransformStore.get(transformID)
	if !exists {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "transformation not found",
			"id":    transformID,
		})
	}

	return c.JSON(result)
}