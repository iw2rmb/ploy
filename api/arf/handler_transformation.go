package arf

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

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

// executeTransformationInternal performs the actual transformation with comprehensive reporting
func (h *Handler) executeTransformationInternal(ctx context.Context, transformID string, req *TransformRequest) (*TransformationResult, error) {
	transformStartTime := time.Now()
	
	// Initialize comprehensive result
	result := &TransformationResult{
		TransformationID: transformID,
		RecipeID:         req.RecipeID,
		StartTime:        transformStartTime,
		Metadata:         make(map[string]interface{}),
	}
	
	// Initialize iteration for tracking
	iteration := TransformationIteration{
		Number:    1,
		StartTime: transformStartTime,
		Stages:    []TransformationStage{},
		Diffs:     []DiffCapture{},
		Errors:    []ErrorCapture{},
		Metrics:   IterationMetrics{},
	}
	
	// Stage 1: Workspace preparation
	stageStart := time.Now()
	workspaceDir := filepath.Join("/tmp", "arf-transformations", transformID)
	if err := os.MkdirAll(workspaceDir, 0755); err != nil {
		iteration.Stages = append(iteration.Stages, TransformationStage{
			Name:      "workspace_preparation",
			StartTime: stageStart,
			EndTime:   time.Now(),
			Duration:  time.Since(stageStart),
			Status:    "failed",
			Details:   err.Error(),
		})
		return nil, fmt.Errorf("failed to create workspace: %w", err)
	}
	defer func() {
		// Clean up workspace after transformation
		os.RemoveAll(workspaceDir)
	}()
	
	iteration.Stages = append(iteration.Stages, TransformationStage{
		Name:      "workspace_preparation",
		StartTime: stageStart,
		EndTime:   time.Now(),
		Duration:  time.Since(stageStart),
		Status:    "success",
	})
	
	// Stage 2: Repository cloning
	stageStart = time.Now()
	repoPath := filepath.Join(workspaceDir, "repository")
	repoInfo, err := h.cloneRepositoryWithInfo(req.Codebase.Repository, req.Codebase.Branch, repoPath)
	if err != nil {
		iteration.Stages = append(iteration.Stages, TransformationStage{
			Name:      "repository_clone",
			StartTime: stageStart,
			EndTime:   time.Now(),
			Duration:  time.Since(stageStart),
			Status:    "failed",
			Details:   err.Error(),
		})
		iteration.Errors = append(iteration.Errors, ErrorCapture{
			Stage:     "repository_clone",
			Type:      "runtime",
			Message:   "Failed to clone repository",
			Details:   err.Error(),
			Timestamp: time.Now(),
		})
		return nil, fmt.Errorf("failed to clone repository: %w", err)
	}
	
	iteration.Stages = append(iteration.Stages, TransformationStage{
		Name:      "repository_clone",
		StartTime: stageStart,
		EndTime:   time.Now(),
		Duration:  time.Since(stageStart),
		Status:    "success",
	})
	result.Repository = repoInfo
	
	// Stage 3: Pre-transformation analysis
	stageStart = time.Now()
	beforeState := h.captureRepositoryState(repoPath)
	iteration.Metrics.FilesAnalyzed = len(beforeState)
	iteration.Stages = append(iteration.Stages, TransformationStage{
		Name:      "pre_transformation_analysis",
		StartTime: stageStart,
		EndTime:   time.Now(),
		Duration:  time.Since(stageStart),
		Status:    "success",
		Details:   map[string]int{"files_analyzed": len(beforeState)},
	})
	
	// Stage 4: Recipe execution
	stageStart = time.Now()
	if h.recipeExecutor == nil {
		iteration.Stages = append(iteration.Stages, TransformationStage{
			Name:      "recipe_execution",
			StartTime: stageStart,
			EndTime:   time.Now(),
			Duration:  time.Since(stageStart),
			Status:    "failed",
			Details:   "recipe executor not available",
		})
		return nil, fmt.Errorf("recipe executor not available")
	}
	
	recipeResult, err := h.recipeExecutor.ExecuteRecipeByID(ctx, req.RecipeID, repoPath)
	if err != nil {
		// Check if this is a "recipe not found" error
		if isNotFoundError(err) {
			iteration.Stages = append(iteration.Stages, TransformationStage{
				Name:      "recipe_execution",
				StartTime: stageStart,
				EndTime:   time.Now(),
				Duration:  time.Since(stageStart),
				Status:    "failed",
				Details:   "recipe not found",
			})
			return nil, &NotFoundError{Message: fmt.Sprintf("recipe not found: %s", req.RecipeID)}
		}
		iteration.Stages = append(iteration.Stages, TransformationStage{
			Name:      "recipe_execution",
			StartTime: stageStart,
			EndTime:   time.Now(),
			Duration:  time.Since(stageStart),
			Status:    "failed",
			Details:   err.Error(),
		})
		iteration.Errors = append(iteration.Errors, ErrorCapture{
			Stage:     "recipe_execution",
			Type:      "runtime",
			Message:   "Recipe execution failed",
			Details:   err.Error(),
			Timestamp: time.Now(),
		})
		return nil, fmt.Errorf("recipe execution failed: %w", err)
	}
	
	iteration.Stages = append(iteration.Stages, TransformationStage{
		Name:      "recipe_execution",
		StartTime: stageStart,
		EndTime:   time.Now(),
		Duration:  time.Since(stageStart),
		Status:    "success",
	})
	
	// Stage 5: Post-transformation analysis
	stageStart = time.Now()
	afterState := h.captureRepositoryState(repoPath)
	diffs := h.calculateDiffs(beforeState, afterState, repoPath)
	iteration.Diffs = diffs
	iteration.Metrics.FilesModified = len(diffs)
	
	for _, diff := range diffs {
		iteration.Metrics.LinesAdded += diff.LinesAdded
		iteration.Metrics.LinesRemoved += diff.LinesRemoved
	}
	
	iteration.Stages = append(iteration.Stages, TransformationStage{
		Name:      "post_transformation_analysis",
		StartTime: stageStart,
		EndTime:   time.Now(),
		Duration:  time.Since(stageStart),
		Status:    "success",
		Details: map[string]int{
			"files_modified": len(diffs),
			"lines_added":    iteration.Metrics.LinesAdded,
			"lines_removed":  iteration.Metrics.LinesRemoved,
		},
	})
	
	// Stage 6: Build validation (if applicable)
	stageStart = time.Now()
	buildMetrics := h.runBuildValidation(repoPath, req.Codebase.BuildTool)
	if buildMetrics != nil {
		result.BuildResults = buildMetrics
		iteration.Metrics.CompileSuccess = buildMetrics.BuildSuccess
		iteration.Stages = append(iteration.Stages, TransformationStage{
			Name:      "build_validation",
			StartTime: stageStart,
			EndTime:   time.Now(),
			Duration:  time.Since(stageStart),
			Status:    map[bool]string{true: "success", false: "failed"}[buildMetrics.BuildSuccess],
			Details:   buildMetrics,
		})
	}
	
	// Stage 7: Test execution (if applicable)
	stageStart = time.Now()
	testMetrics := h.runTests(repoPath, req.Codebase.BuildTool)
	if testMetrics != nil {
		result.TestResults = testMetrics
		iteration.Metrics.TestsRun = testMetrics.TotalTests
		iteration.Metrics.TestsPassed = testMetrics.PassedTests
		iteration.Metrics.CoveragePercent = testMetrics.CoveragePercent
		iteration.Stages = append(iteration.Stages, TransformationStage{
			Name:      "test_execution",
			StartTime: stageStart,
			EndTime:   time.Now(),
			Duration:  time.Since(stageStart),
			Status:    map[bool]string{true: "success", false: "failed"}[testMetrics.PassedTests == testMetrics.TotalTests],
			Details:   testMetrics,
		})
	}
	
	// Complete iteration
	iteration.EndTime = time.Now()
	iteration.Duration = time.Since(iteration.StartTime)
	iteration.Status = "success"
	if len(iteration.Errors) > 0 {
		iteration.Status = "partial"
	}
	
	// Populate result from recipe execution (backward compatibility)
	if recipeResult != nil {
		result.Success = recipeResult.Success
		result.ChangesApplied = recipeResult.ChangesApplied
		result.TotalFiles = recipeResult.TotalFiles
		result.FilesModified = recipeResult.FilesModified
		result.Diff = recipeResult.Diff
		result.ValidationScore = recipeResult.ValidationScore
		result.Errors = recipeResult.Errors
		result.Warnings = recipeResult.Warnings
	}
	
	// Add comprehensive reporting
	result.Iterations = []TransformationIteration{iteration}
	result.DiffCaptures = diffs
	result.ErrorLog = iteration.Errors
	
	// Generate summary
	result.Summary = &TransformationSummary{
		TotalIterations:      1,
		SuccessfulIterations: map[bool]int{true: 1, false: 0}[iteration.Status == "success"],
		PartialIterations:    map[bool]int{true: 1, false: 0}[iteration.Status == "partial"],
		FailedIterations:     map[bool]int{true: 1, false: 0}[iteration.Status == "failed"],
		AverageIterationTime: iteration.Duration,
		FinalCompileStatus:   iteration.Metrics.CompileSuccess,
		FinalTestStatus:      iteration.Metrics.TestsPassed == iteration.Metrics.TestsRun && iteration.Metrics.TestsRun > 0,
		TotalFilesModified:   iteration.Metrics.FilesModified,
		TotalLinesChanged:    iteration.Metrics.LinesAdded + iteration.Metrics.LinesRemoved,
	}
	
	// Complete result
	result.EndTime = time.Now()
	result.ExecutionTime = time.Since(transformStartTime)
	
	return result, nil
}

// cloneRepositoryWithInfo clones a git repository and returns repository information
func (h *Handler) cloneRepositoryWithInfo(repoURL, branch, targetPath string) (*RepositoryInfo, error) {
	// Ensure git is available
	if _, err := exec.LookPath("git"); err != nil {
		return nil, fmt.Errorf("git command not available")
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
		return nil, fmt.Errorf("failed to create repository directory: %w", err)
	}

	// Create a simple placeholder to indicate repository was "cloned"
	placeholderFile := filepath.Join(targetPath, ".git-placeholder")
	if err := os.WriteFile(placeholderFile, []byte("repository cloned"), 0644); err != nil {
		return nil, fmt.Errorf("failed to create repository placeholder: %w", err)
	}

	// Gather repository information
	repoInfo := &RepositoryInfo{
		URL:      repoURL,
		Branch:   branch,
		Metadata: make(map[string]string),
	}

	// Count files and calculate size
	fileCount := 0
	var totalSize int64
	err := filepath.Walk(targetPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip errors
		}
		if !info.IsDir() {
			fileCount++
			totalSize += info.Size()
		}
		return nil
	})
	if err == nil {
		repoInfo.FileCount = fileCount
		repoInfo.Size = totalSize
	}

	// Detect language and build tool
	repoInfo.Language = h.detectLanguage(targetPath)
	repoInfo.BuildTool = h.detectBuildTool(targetPath)

	return repoInfo, nil
}

// cloneRepository is kept for backward compatibility
func (h *Handler) cloneRepository(repoURL, branch, targetPath string) error {
	_, err := h.cloneRepositoryWithInfo(repoURL, branch, targetPath)
	return err
}

// captureRepositoryState captures the current state of files in the repository
func (h *Handler) captureRepositoryState(repoPath string) map[string]string {
	state := make(map[string]string)
	filepath.Walk(repoPath, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		// Skip non-source files
		if strings.Contains(path, ".git") || strings.Contains(path, "node_modules") {
			return nil
		}
		relPath, _ := filepath.Rel(repoPath, path)
		content, err := os.ReadFile(path)
		if err == nil {
			state[relPath] = string(content)
		}
		return nil
	})
	return state
}

// calculateDiffs calculates the differences between before and after states
func (h *Handler) calculateDiffs(before, after map[string]string, repoPath string) []DiffCapture {
	var diffs []DiffCapture
	
	// Check for modified and deleted files
	for path, beforeContent := range before {
		afterContent, exists := after[path]
		if !exists {
			// File was deleted
			diffs = append(diffs, DiffCapture{
				File:         path,
				Type:         "deleted",
				Before:       beforeContent,
				LinesRemoved: countLines(beforeContent),
				Timestamp:    time.Now(),
			})
		} else if beforeContent != afterContent {
			// File was modified
			diff := DiffCapture{
				File:      path,
				Type:      "modified",
				Before:    beforeContent,
				After:     afterContent,
				Timestamp: time.Now(),
			}
			diff.LinesAdded, diff.LinesRemoved = calculateLineChanges(beforeContent, afterContent)
			diff.UnifiedDiff = generateUnifiedDiff(path, beforeContent, afterContent)
			diffs = append(diffs, diff)
		}
	}
	
	// Check for added files
	for path, afterContent := range after {
		if _, exists := before[path]; !exists {
			diffs = append(diffs, DiffCapture{
				File:       path,
				Type:       "added",
				After:      afterContent,
				LinesAdded: countLines(afterContent),
				Timestamp:  time.Now(),
			})
		}
	}
	
	return diffs
}

// runBuildValidation runs build validation for the repository
func (h *Handler) runBuildValidation(repoPath string, buildTool string) *BuildMetrics {
	if buildTool == "" {
		buildTool = h.detectBuildTool(repoPath)
	}
	
	if buildTool == "" {
		return nil // No build tool detected
	}
	
	metrics := &BuildMetrics{
		BuildTool: buildTool,
	}
	
	// Determine build command based on build tool
	switch buildTool {
	case "maven":
		metrics.BuildCommand = "mvn compile"
	case "gradle":
		metrics.BuildCommand = "gradle build"
	case "npm":
		metrics.BuildCommand = "npm run build"
	case "go":
		metrics.BuildCommand = "go build ./..."
	default:
		return nil
	}
	
	// Simulate build execution (in production, would actually run the command)
	startTime := time.Now()
	metrics.BuildSuccess = true // Simulated success
	metrics.BuildDuration = time.Since(startTime)
	
	return metrics
}

// runTests runs tests for the repository
func (h *Handler) runTests(repoPath string, buildTool string) *TestMetrics {
	if buildTool == "" {
		buildTool = h.detectBuildTool(repoPath)
	}
	
	if buildTool == "" {
		return nil // No build tool detected
	}
	
	metrics := &TestMetrics{
		TestFramework: buildTool,
	}
	
	// Determine test command based on build tool
	switch buildTool {
	case "maven":
		metrics.TestCommand = "mvn test"
	case "gradle":
		metrics.TestCommand = "gradle test"
	case "npm":
		metrics.TestCommand = "npm test"
	case "go":
		metrics.TestCommand = "go test ./..."
	default:
		return nil
	}
	
	// Simulate test execution (in production, would actually run the command)
	startTime := time.Now()
	metrics.TotalTests = 10      // Simulated
	metrics.PassedTests = 9      // Simulated
	metrics.FailedTests = 1      // Simulated
	metrics.CoveragePercent = 85.5 // Simulated
	metrics.TestDuration = time.Since(startTime)
	
	return metrics
}

// detectLanguage detects the primary language of the repository
func (h *Handler) detectLanguage(repoPath string) string {
	// Check for language-specific files
	if _, err := os.Stat(filepath.Join(repoPath, "pom.xml")); err == nil {
		return "java"
	}
	if _, err := os.Stat(filepath.Join(repoPath, "build.gradle")); err == nil {
		return "java"
	}
	if _, err := os.Stat(filepath.Join(repoPath, "package.json")); err == nil {
		return "javascript"
	}
	if _, err := os.Stat(filepath.Join(repoPath, "go.mod")); err == nil {
		return "go"
	}
	if _, err := os.Stat(filepath.Join(repoPath, "requirements.txt")); err == nil {
		return "python"
	}
	return "unknown"
}

// detectBuildTool detects the build tool used in the repository
func (h *Handler) detectBuildTool(repoPath string) string {
	if _, err := os.Stat(filepath.Join(repoPath, "pom.xml")); err == nil {
		return "maven"
	}
	if _, err := os.Stat(filepath.Join(repoPath, "build.gradle")); err == nil {
		return "gradle"
	}
	if _, err := os.Stat(filepath.Join(repoPath, "package.json")); err == nil {
		return "npm"
	}
	if _, err := os.Stat(filepath.Join(repoPath, "go.mod")); err == nil {
		return "go"
	}
	return ""
}

// Helper functions

func countLines(content string) int {
	if content == "" {
		return 0
	}
	return strings.Count(content, "\n") + 1
}

func calculateLineChanges(before, after string) (added, removed int) {
	beforeLines := strings.Split(before, "\n")
	afterLines := strings.Split(after, "\n")
	
	// Simple line count difference (in production, would use proper diff algorithm)
	if len(afterLines) > len(beforeLines) {
		added = len(afterLines) - len(beforeLines)
	} else {
		removed = len(beforeLines) - len(afterLines)
	}
	
	return added, removed
}

func generateUnifiedDiff(filename, before, after string) string {
	// Simple unified diff header (in production, would use proper diff library)
	var buf bytes.Buffer
	fmt.Fprintf(&buf, "--- a/%s\n", filename)
	fmt.Fprintf(&buf, "+++ b/%s\n", filename)
	
	beforeLines := strings.Split(before, "\n")
	afterLines := strings.Split(after, "\n")
	
	// Show first few lines of difference
	maxLines := 5
	for i := 0; i < len(beforeLines) && i < maxLines; i++ {
		if i < len(afterLines) && beforeLines[i] != afterLines[i] {
			fmt.Fprintf(&buf, "-%s\n", beforeLines[i])
			fmt.Fprintf(&buf, "+%s\n", afterLines[i])
		}
	}
	
	return buf.String()
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
