package arf

import (
	"fmt"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/iw2rmb/ploy/api/arf/models"
)

// ExecuteTransformation executes a transformation with robust self-healing capabilities
func (h *Handler) ExecuteTransformation(c *fiber.Ctx) error {
	// Debug: log the raw body first
	fmt.Printf("[DEBUG] Raw request body: %s\n", string(c.Body()))
	
	// Check if this is the new robust transformation request format
	var robustReq RobustTransformRequest
	if err := c.BodyParser(&robustReq); err == nil && 
		(len(robustReq.Transformations.RecipeIDs) > 0 || len(robustReq.Transformations.LLMPrompts) > 0) {
		// New robust transformation format - need to execute directly since body is already consumed
		fmt.Printf("[DEBUG] Detected robust transformation format - Repository: '%s', Archive: '%s', RecipeIDs: %v\n", 
			robustReq.InputSource.Repository, robustReq.InputSource.Archive, robustReq.Transformations.RecipeIDs)
		
		// Logger function for tracking progress
		logger := func(level, stage, message, details string) {
			if ctx := c.Context(); ctx != nil {
				if ctxLogger := ctx.Logger(); ctxLogger != nil {
					ctxLogger.Printf("[%s] %s: %s %s", level, stage, message, details)
				} else {
					fmt.Printf("[%s] %s: %s %s\n", level, stage, message, details)
				}
			} else {
				fmt.Printf("[%s] %s: %s %s\n", level, stage, message, details)
			}
		}
		
		// Execute the robust transformation directly with the already parsed request
		fmt.Printf("[DEBUG] About to call ExecuteRobustTransformation\n")
		result, err := ExecuteRobustTransformation(c.Context(), &robustReq, logger)
		if err != nil {
			fmt.Printf("[DEBUG] ExecuteRobustTransformation failed: %v\n", err)
			return c.Status(500).JSON(fiber.Map{
				"error":   "Transformation failed",
				"details": err.Error(),
			})
		}
		fmt.Printf("[DEBUG] ExecuteRobustTransformation succeeded\n")
		
		return c.JSON(result)
	}
	
	// Legacy transformation request format
	var req struct {
		RecipeID   string            `json:"recipe_id"`
		Repository Repository        `json:"repository"`
		Options    map[string]string `json:"options"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{
			"error":   "Invalid request",
			"details": err.Error(),
		})
	}

	// Convert legacy request to robust format
	robustReq = RobustTransformRequest{}
	robustReq.InputSource.Repository = req.Repository.URL
	robustReq.InputSource.Branch = req.Repository.Branch
	robustReq.Transformations.RecipeIDs = []string{req.RecipeID}
	robustReq.Execution.MaxIterations = 3
	robustReq.Execution.ParallelTries = 3
	robustReq.Execution.Timeout = "30m"
	robustReq.Execution.PlanModel = "ollama/codellama:7b"
	robustReq.Execution.ExecModel = "ollama/codellama:7b"
	robustReq.Output.Format = "diff"
	robustReq.Output.ReportLevel = "standard"
	
	// Execute robust transformation
	logger := func(level, stage, message, details string) {
		// Log to Fiber context logger if available
		if ctx := c.Context(); ctx != nil {
			if ctxLogger := ctx.Logger(); ctxLogger != nil {
				ctxLogger.Printf("[%s] %s: %s %s", level, stage, message, details)
			} else {
				fmt.Printf("[%s] %s: %s %s\n", level, stage, message, details)
			}
		} else {
			fmt.Printf("[%s] %s: %s %s\n", level, stage, message, details)
		}
	}
	
	result, err := ExecuteRobustTransformation(c.Context(), &robustReq, logger)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{
			"error":   "Transformation failed",
			"details": err.Error(),
		})
	}

	return c.JSON(result)
}

// ExecuteRobustTransformation handles the new robust transformation format
func (h *Handler) ExecuteRobustTransformation(c *fiber.Ctx) error {
	var req RobustTransformRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{
			"error":   "Invalid request",
			"details": err.Error(),
		})
	}
	
	// Debug: log the parsed request
	fmt.Printf("[DEBUG] Parsed request - Repository: '%s', Archive: '%s', RecipeIDs: %v\n", 
		req.InputSource.Repository, req.InputSource.Archive, req.Transformations.RecipeIDs)
	
	// Logger function for tracking progress
	logger := func(level, stage, message, details string) {
		if ctx := c.Context(); ctx != nil {
			if ctxLogger := ctx.Logger(); ctxLogger != nil {
				ctxLogger.Printf("[%s] %s: %s %s", level, stage, message, details)
			} else {
				fmt.Printf("[%s] %s: %s %s\n", level, stage, message, details)
			}
		} else {
			fmt.Printf("[%s] %s: %s %s\n", level, stage, message, details)
		}
	}
	
	// Execute the robust transformation
	result, err := ExecuteRobustTransformation(c.Context(), &req, logger)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{
			"error":   "Transformation failed",
			"details": err.Error(),
		})
	}
	
	return c.JSON(result)
}

// GetTransformationResult retrieves the result of a transformation
func (h *Handler) GetTransformationResult(c *fiber.Ctx) error {
	transformationID := c.Params("id")

	// Mock getting transformation result
	result := TransformationResult{
		RecipeID:       "recipe-123",
		Success:        true,
		ChangesApplied: 3,
		FilesModified:  []string{"build.gradle"},
		ExecutionTime:  1 * time.Minute,
	}
	_ = transformationID
	// Return the result

	return c.JSON(result)
}

// ExecuteHybridTransformation executes a hybrid transformation
func (h *Handler) ExecuteHybridTransformation(c *fiber.Ctx) error {
	if h.hybridPipeline == nil {
		return c.Status(503).JSON(fiber.Map{
			"error": "Hybrid pipeline not available",
		})
	}

	var req HybridRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{
			"error":   "Invalid request",
			"details": err.Error(),
		})
	}

	result, err := h.hybridPipeline.ExecuteHybridTransformation(c.Context(), req)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{
			"error":   "Hybrid transformation failed",
			"details": err.Error(),
		})
	}

	return c.JSON(result)
}

// SelectTransformationStrategy selects the optimal transformation strategy
func (h *Handler) SelectTransformationStrategy(c *fiber.Ctx) error {
	if h.strategySelector == nil {
		// Return default strategy
		return c.JSON(fiber.Map{
			"strategy": "openrewrite_only",
			"confidence": 0.8,
			"reasoning": "Default strategy (strategy selector not configured)",
		})
	}

	var req StrategyRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{
			"error":   "Invalid request",
			"details": err.Error(),
		})
	}

	strategy, err := h.strategySelector.SelectStrategy(c.Context(), req)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{
			"error":   "Strategy selection failed",
			"details": err.Error(),
		})
	}

	return c.JSON(strategy)
}

// AnalyzeComplexity analyzes transformation complexity
func (h *Handler) AnalyzeComplexity(c *fiber.Ctx) error {
	var req struct {
		Repository Repository `json:"repository"`
		Recipe     *models.Recipe `json:"recipe"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{
			"error":   "Invalid request",
			"details": err.Error(),
		})
	}

	// Mock complexity analysis for now
	analysis := ComplexityAnalysis{
		OverallComplexity: 0.65,
		FactorBreakdown: map[string]float64{
			"code_size":       0.5,
			"dependencies":    0.7,
			"recipe_type":     0.6,
			"language_factors": 0.8,
		},
		PredictedChallenges: []PredictedChallenge{
			{
				Type:        "dependency_conflicts",
				Severity:    0.3,
				Description: "Potential dependency version conflicts",
				Mitigation:  "Use dependency resolution strategy",
			},
		},
		RecommendedApproach: RecommendedApproach{
			Strategy:   StrategyHybridSequential,
			Confidence: 0.85,
			Reasoning:  "Medium complexity suggests hybrid approach for best results",
			Alternatives: []StrategyType{
				StrategyOpenRewriteOnly,
				StrategyLLMOnly,
			},
		},
	}

	return c.JSON(analysis)
}

// RecordTransformationOutcome records the outcome of a transformation
func (h *Handler) RecordTransformationOutcome(c *fiber.Ctx) error {
	if h.learningSystem == nil {
		return c.Status(503).JSON(fiber.Map{
			"error": "Learning system not available",
		})
	}

	var outcome TransformationOutcome
	if err := c.BodyParser(&outcome); err != nil {
		return c.Status(400).JSON(fiber.Map{
			"error":   "Invalid outcome data",
			"details": err.Error(),
		})
	}

	if err := h.learningSystem.RecordTransformationOutcome(c.Context(), outcome); err != nil {
		return c.Status(500).JSON(fiber.Map{
			"error":   "Failed to record outcome",
			"details": err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"message": "Outcome recorded successfully",
	})
}

// ExtractLearningPatterns extracts patterns from transformation history
func (h *Handler) ExtractLearningPatterns(c *fiber.Ctx) error {
	if h.learningSystem == nil {
		// Return mock patterns if learning system not available
		return c.JSON(fiber.Map{
			"patterns": fiber.Map{
				"success_patterns": []fiber.Map{
					{
						"signature":      "spring_boot_upgrade",
						"frequency":      15,
						"success_rate":   0.92,
						"context_factors": []string{"spring_version", "java_version"},
					},
				},
				"failure_patterns": []fiber.Map{
					{
						"signature":    "complex_dependency_update",
						"frequency":    8,
						"failure_rate": 0.65,
						"common_errors": []string{"version_conflict", "api_breaking_change"},
					},
				},
				"strategy_effectiveness": fiber.Map{
					"openrewrite_only": 0.75,
					"hybrid_sequential": 0.88,
					"llm_only":         0.62,
				},
				"confidence": 0.85,
			},
		})
	}

	timeWindow := c.Query("time_window", "7d")
	duration, err := time.ParseDuration(timeWindow)
	if err != nil {
		duration = 7 * 24 * time.Hour
	}

	patterns, err := h.learningSystem.ExtractPatterns(c.Context(), duration)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{
			"error":   "Failed to extract patterns",
			"details": err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"patterns": patterns,
	})
}

// GenerateLLMRecipe generates a recipe using LLM
func (h *Handler) GenerateLLMRecipe(c *fiber.Ctx) error {
	if h.llmGenerator == nil {
		return c.Status(503).JSON(fiber.Map{
			"error": "LLM generator not available",
		})
	}

	var req RecipeGenerationRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{
			"error":   "Invalid request",
			"details": err.Error(),
		})
	}

	recipe, err := h.llmGenerator.GenerateRecipe(c.Context(), req)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{
			"error":   "Failed to generate recipe",
			"details": err.Error(),
		})
	}

	// Validate the generated recipe
	validationResult, err := h.llmGenerator.ValidateGenerated(c.Context(), *recipe)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{
			"error":   "Recipe validation failed",
			"details": err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"recipe":     recipe,
		"validation": validationResult,
	})
}

// OptimizeRecipe optimizes a recipe based on feedback
func (h *Handler) OptimizeRecipe(c *fiber.Ctx) error {
	if h.llmGenerator == nil {
		return c.Status(503).JSON(fiber.Map{
			"error": "LLM generator not available",
		})
	}

	var req struct {
		Recipe   *models.Recipe         `json:"recipe"`
		Feedback TransformationFeedback `json:"feedback"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{
			"error":   "Invalid request",
			"details": err.Error(),
		})
	}

	optimizedRecipe, err := h.llmGenerator.OptimizeRecipe(c.Context(), req.Recipe, req.Feedback)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{
			"error":   "Failed to optimize recipe",
			"details": err.Error(),
		})
	}

	return c.JSON(optimizedRecipe)
}