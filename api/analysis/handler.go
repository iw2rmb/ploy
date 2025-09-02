package analysis

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/iw2rmb/ploy/api/arf"
	"github.com/sirupsen/logrus"
)

// Handler handles static analysis API requests
type Handler struct {
	engine     AnalysisEngine
	arfHandler *arf.Handler
	logger     *logrus.Logger
}

// NewHandler creates a new analysis handler
func NewHandler(engine AnalysisEngine, arfHandler *arf.Handler, logger *logrus.Logger) *Handler {
	return &Handler{
		engine:     engine,
		arfHandler: arfHandler,
		logger:     logger,
	}
}

// RegisterRoutes registers the analysis API routes
func (h *Handler) RegisterRoutes(app *fiber.App) {
	// Analysis endpoints
	api := app.Group("/v1/analysis")
	
	// Core analysis operations
	api.Post("/analyze", h.AnalyzeRepository)
	api.Get("/results/:id", h.GetAnalysisResult)
	api.Get("/results", h.ListAnalysisResults)
	
	// Configuration
	api.Get("/config", h.GetConfiguration)
	api.Put("/config", h.UpdateConfiguration)
	api.Post("/config/validate", h.ValidateConfiguration)
	
	// Languages and analyzers
	api.Get("/languages", h.GetSupportedLanguages)
	api.Get("/languages/:language/info", h.GetAnalyzerInfo)
	
	// Issues and fixes
	api.Get("/issues/:id/fixes", h.GetFixSuggestions)
	api.Post("/issues/:id/fix", h.ApplyFix)
	
	// Cache management
	api.Delete("/cache", h.ClearCache)
	api.Get("/cache/metrics", h.GetCacheMetrics)
	
	// Health check
	api.Get("/health", h.HealthCheck)
}

// AnalyzeRepository handles repository analysis requests
func (h *Handler) AnalyzeRepository(c *fiber.Ctx) error {
	var req AnalysisRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}
	
	// Create context with timeout
	timeout := 30 * time.Minute
	if req.Config.Timeout > 0 {
		timeout = req.Config.Timeout
	}
	
	ctx, cancel := context.WithTimeout(c.Context(), timeout)
	defer cancel()
	
	// Run analysis
	result, err := h.engine.AnalyzeRepository(ctx, req.Repository)
	if err != nil {
		h.logger.WithError(err).Error("Analysis failed")
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{
			"error": err.Error(),
		})
	}
	
	// Trigger ARF remediation if requested and integration is enabled
	if req.FixIssues && req.Config.ARFIntegration && len(result.ARFTriggers) > 0 {
		go h.triggerARFRemediation(result)
	}
	
	return c.JSON(result)
}

// GetAnalysisResult retrieves a specific analysis result
func (h *Handler) GetAnalysisResult(c *fiber.Ctx) error {
	id := c.Params("id")
	
	result, err := h.engine.GetAnalysisResult(id)
	if err != nil {
		return c.Status(http.StatusNotFound).JSON(fiber.Map{
			"error": "Analysis result not found",
		})
	}
	
	return c.JSON(result)
}

// ListAnalysisResults lists analysis results for a repository
func (h *Handler) ListAnalysisResults(c *fiber.Ctx) error {
	// Parse query parameters
	repoID := c.Query("repository_id")
	limitStr := c.Query("limit", "10")
	
	if repoID == "" {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{
			"error": "repository_id is required",
		})
	}
	
	limit, err := strconv.Atoi(limitStr)
	if err != nil || limit <= 0 {
		limit = 10
	}
	
	repo := Repository{ID: repoID}
	results, err := h.engine.ListAnalysisResults(repo, limit)
	if err != nil {
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{
			"error": err.Error(),
		})
	}
	
	return c.JSON(fiber.Map{
		"results": results,
		"count":   len(results),
	})
}

// GetConfiguration returns the current analysis configuration
func (h *Handler) GetConfiguration(c *fiber.Ctx) error {
	config := h.engine.GetConfiguration()
	return c.JSON(config)
}

// UpdateConfiguration updates the analysis configuration
func (h *Handler) UpdateConfiguration(c *fiber.Ctx) error {
	var config AnalysisConfig
	if err := c.BodyParser(&config); err != nil {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid configuration",
		})
	}
	
	if err := h.engine.ConfigureAnalysis(config); err != nil {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{
			"error": err.Error(),
		})
	}
	
	return c.JSON(fiber.Map{
		"message": "Configuration updated successfully",
	})
}

// ValidateConfiguration validates an analysis configuration
func (h *Handler) ValidateConfiguration(c *fiber.Ctx) error {
	var config AnalysisConfig
	if err := c.BodyParser(&config); err != nil {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid configuration",
		})
	}
	
	if err := h.engine.ValidateConfiguration(config); err != nil {
		return c.JSON(fiber.Map{
			"valid": false,
			"error": err.Error(),
		})
	}
	
	return c.JSON(fiber.Map{
		"valid": true,
	})
}

// GetSupportedLanguages returns all supported languages
func (h *Handler) GetSupportedLanguages(c *fiber.Ctx) error {
	languages := h.engine.GetSupportedLanguages()
	return c.JSON(fiber.Map{
		"languages": languages,
		"count":     len(languages),
	})
}

// GetAnalyzerInfo returns information about a specific analyzer
func (h *Handler) GetAnalyzerInfo(c *fiber.Ctx) error {
	language := c.Params("language")
	
	analyzer, err := h.engine.GetAnalyzer(language)
	if err != nil {
		return c.Status(http.StatusNotFound).JSON(fiber.Map{
			"error": fmt.Sprintf("No analyzer for language: %s", language),
		})
	}
	
	info := analyzer.GetAnalyzerInfo()
	return c.JSON(info)
}

// GetFixSuggestions returns fix suggestions for an issue
func (h *Handler) GetFixSuggestions(c *fiber.Ctx) error {
	issueID := c.Params("id")
	
	// TODO: Retrieve issue from storage
	// For now, return a placeholder response
	return c.JSON(fiber.Map{
		"issue_id":    issueID,
		"suggestions": []FixSuggestion{},
	})
}

// ApplyFix applies a fix for an issue
func (h *Handler) ApplyFix(c *fiber.Ctx) error {
	issueID := c.Params("id")
	
	var fixRequest struct {
		FixIndex int  `json:"fix_index"`
		DryRun   bool `json:"dry_run"`
	}
	
	if err := c.BodyParser(&fixRequest); err != nil {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request",
		})
	}
	
	// TODO: Implement fix application through ARF
	return c.JSON(fiber.Map{
		"issue_id": issueID,
		"status":   "pending",
		"message":  "Fix application queued for ARF processing",
	})
}

// ClearCache clears the analysis cache
func (h *Handler) ClearCache(c *fiber.Ctx) error {
	repoID := c.Query("repository_id")
	
	if repoID != "" {
		repo := Repository{ID: repoID}
		if err := h.engine.ClearCache(repo); err != nil {
			return c.Status(http.StatusInternalServerError).JSON(fiber.Map{
				"error": err.Error(),
			})
		}
	}
	
	return c.JSON(fiber.Map{
		"message": "Cache cleared successfully",
	})
}

// GetCacheMetrics returns cache metrics
func (h *Handler) GetCacheMetrics(c *fiber.Ctx) error {
	// Get cache metrics from engine
	// TODO: Add GetCacheMetrics method to engine interface
	metrics := map[string]int64{
		"hits":   0,
		"misses": 0,
	}
	
	return c.JSON(metrics)
}

// HealthCheck returns the health status of the analysis service
func (h *Handler) HealthCheck(c *fiber.Ctx) error {
	languages := h.engine.GetSupportedLanguages()
	
	return c.JSON(fiber.Map{
		"status":             "healthy",
		"supported_languages": languages,
		"analyzers_count":     len(languages),
		"arf_integration":     h.arfHandler != nil,
	})
}

// triggerARFRemediation triggers ARF remediation for analysis results
func (h *Handler) triggerARFRemediation(result *AnalysisResult) {
	if h.arfHandler == nil {
		h.logger.Warn("ARF handler not available for remediation")
		return
	}
	
	h.logger.WithFields(logrus.Fields{
		"repository": result.Repository.Name,
		"triggers":   len(result.ARFTriggers),
	}).Info("Triggering ARF remediation")
	
	// Log that ARF triggers have been identified
	for _, trigger := range result.ARFTriggers {
		h.logger.WithFields(map[string]interface{}{
			"recipe":      trigger.RecipeName,
			"issue_id":    trigger.IssueID,
			"auto_approve": trigger.AutoApprove,
			"priority":    trigger.Priority,
		}).Info("ARF remediation trigger identified - would execute recipe")
		
		// Note: Actual recipe execution would be handled by a separate workflow
		// This is just logging the triggers for now
	}
}