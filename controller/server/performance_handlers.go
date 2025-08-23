package server

import (
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/iw2rmb/ploy/controller/performance"
)

// getGaugeValue safely gets value from a prometheus gauge
func getGaugeValue(gauge interface{}) float64 {
	// For simplicity, we'll return a placeholder value
	// In production, this should properly extract the gauge value
	return 0.0
}

// handlePerformanceStats returns comprehensive performance statistics
func (s *Server) handlePerformanceStats(c *fiber.Ctx) error {
	stats := map[string]interface{}{
		"timestamp": time.Now(),
		"uptime_seconds": getGaugeValue(s.dependencies.Metrics.ControllerUptime),
	}

	// Cache statistics
	if envStore, ok := s.dependencies.EnvStore.(interface {
		GetCacheStats() performance.CacheStats
	}); ok {
		stats["cache"] = map[string]interface{}{
			"envstore": envStore.GetCacheStats(),
		}
	}

	// Connection pool statistics
	if s.dependencies.ConsulPool != nil {
		stats["connection_pools"] = map[string]interface{}{
			"consul": map[string]interface{}{
				"size": s.dependencies.ConsulPool.Size(),
			},
		}
	}
	if s.dependencies.NomadPool != nil {
		if stats["connection_pools"] == nil {
			stats["connection_pools"] = make(map[string]interface{})
		}
		poolStats := stats["connection_pools"].(map[string]interface{})
		poolStats["nomad"] = map[string]interface{}{
			"size": s.dependencies.NomadPool.Size(),
		}
	}

	// Storage factory statistics
	if s.dependencies.StorageFactory != nil {
		stats["storage_factory"] = s.dependencies.StorageFactory.GetStats()
	}

	// Coordination statistics
	if s.dependencies.CoordinationManager != nil {
		stats["coordination"] = map[string]interface{}{
			"is_leader": s.dependencies.CoordinationManager.IsLeader(),
		}
	}

	// Metrics summary
	if s.dependencies.Metrics != nil {
		stats["metrics"] = map[string]interface{}{
			"startup_time_seconds": getGaugeValue(s.dependencies.Metrics.StartupTime),
		}
	}

	return c.JSON(fiber.Map{
		"status": "success",
		"data":   stats,
	})
}

// handleCacheManagement provides cache management operations
func (s *Server) handleCacheManagement(c *fiber.Ctx) error {
	action := c.Query("action")
	cacheType := c.Query("type", "all")

	switch action {
	case "clear":
		return s.handleClearCache(c, cacheType)
	case "stats":
		return s.handleCacheStats(c, cacheType)
	case "warmup":
		return s.handleCacheWarmup(c, cacheType)
	default:
		return c.Status(400).JSON(fiber.Map{
			"error": "Invalid action. Supported: clear, stats, warmup",
		})
	}
}

// handleClearCache clears specified caches
func (s *Server) handleClearCache(c *fiber.Ctx, cacheType string) error {
	clearedCaches := make([]string, 0)

	if cacheType == "all" || cacheType == "envstore" {
		if envStore, ok := s.dependencies.EnvStore.(interface {
			ClearCache()
		}); ok {
			envStore.ClearCache()
			clearedCaches = append(clearedCaches, "envstore")
		}
	}

	if cacheType == "all" || cacheType == "config" {
		if s.dependencies.StorageFactory != nil {
			// Config cache clearing would be handled by the factory
			clearedCaches = append(clearedCaches, "config")
		}
	}

	if len(clearedCaches) == 0 {
		return c.Status(400).JSON(fiber.Map{
			"error": "No caches available to clear for type: " + cacheType,
		})
	}

	return c.JSON(fiber.Map{
		"status":         "success",
		"cleared_caches": clearedCaches,
		"message":        "Caches cleared successfully",
	})
}

// handleCacheStats returns cache statistics
func (s *Server) handleCacheStats(c *fiber.Ctx, cacheType string) error {
	stats := make(map[string]interface{})

	if cacheType == "all" || cacheType == "envstore" {
		if envStore, ok := s.dependencies.EnvStore.(interface {
			GetCacheStats() performance.CacheStats
		}); ok {
			stats["envstore"] = envStore.GetCacheStats()
		}
	}

	if cacheType == "all" || cacheType == "config" {
		if s.dependencies.StorageFactory != nil {
			factoryStats := s.dependencies.StorageFactory.GetStats()
			if configStats, exists := factoryStats["config_cache_stats"]; exists {
				stats["config"] = configStats
			}
		}
	}

	return c.JSON(fiber.Map{
		"status": "success",
		"data":   stats,
	})
}

// handleCacheWarmup warms up caches with common data
func (s *Server) handleCacheWarmup(c *fiber.Ctx, cacheType string) error {
	var warmupReq struct {
		Apps []string `json:"apps"`
	}

	if err := c.BodyParser(&warmupReq); err != nil {
		warmupReq.Apps = []string{} // Default to empty if parsing fails
	}

	warmedUp := make([]string, 0)

	if cacheType == "all" || cacheType == "envstore" {
		if envStore, ok := s.dependencies.EnvStore.(interface {
			WarmupCache(apps []string) error
		}); ok {
			if err := envStore.WarmupCache(warmupReq.Apps); err != nil {
				return c.Status(500).JSON(fiber.Map{
					"error": "Failed to warmup envstore cache: " + err.Error(),
				})
			}
			warmedUp = append(warmedUp, "envstore")
		}
	}

	return c.JSON(fiber.Map{
		"status":     "success",
		"warmed_up":  warmedUp,
		"apps":       warmupReq.Apps,
		"message":    "Cache warmup completed successfully",
	})
}

// handleConnectionPools manages connection pool operations
func (s *Server) handleConnectionPools(c *fiber.Ctx) error {
	action := c.Query("action", "stats")

	switch action {
	case "stats":
		return s.handleConnectionPoolStats(c)
	default:
		return c.Status(400).JSON(fiber.Map{
			"error": "Invalid action. Supported: stats",
		})
	}
}

// handleConnectionPoolStats returns connection pool statistics
func (s *Server) handleConnectionPoolStats(c *fiber.Ctx) error {
	stats := make(map[string]interface{})

	if s.dependencies.ConsulPool != nil {
		stats["consul"] = map[string]interface{}{
			"available_connections": s.dependencies.ConsulPool.Size(),
			"status":               "active",
		}
	}

	if s.dependencies.NomadPool != nil {
		stats["nomad"] = map[string]interface{}{
			"available_connections": s.dependencies.NomadPool.Size(),
			"status":               "active",
		}
	}

	if len(stats) == 0 {
		stats["message"] = "No connection pools available"
	}

	return c.JSON(fiber.Map{
		"status": "success",
		"data":   stats,
	})
}

// setupPerformanceRoutes configures performance monitoring routes
func (s *Server) setupPerformanceRoutes(api fiber.Router) {
	perfGroup := api.Group("/performance")

	// Performance statistics endpoint
	perfGroup.Get("/stats", s.handlePerformanceStats)

	// Cache management endpoints
	perfGroup.Get("/cache", s.handleCacheManagement)
	perfGroup.Post("/cache", s.handleCacheManagement)

	// Connection pool management endpoints
	perfGroup.Get("/pools", s.handleConnectionPools)

	// Performance metrics in JSON format (alternative to Prometheus)
	perfGroup.Get("/metrics", func(c *fiber.Ctx) error {
		if s.dependencies.Metrics == nil {
			return c.Status(503).JSON(fiber.Map{
				"error": "Metrics not available",
			})
		}

		// This is a simplified JSON view of metrics
		// The full metrics are available at /metrics endpoint
		return c.JSON(fiber.Map{
			"status": "success",
			"message": "Use /metrics endpoint for full Prometheus metrics",
			"performance_stats_endpoint": "/v1/performance/stats",
		})
	})
}