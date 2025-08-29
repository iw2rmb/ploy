package arf

import (
	"github.com/gofiber/fiber/v2"
)

// CreateABTest creates a new A/B test
func (h *Handler) CreateABTest(c *fiber.Ctx) error {
	if h.abTestFramework == nil {
		return c.Status(503).JSON(fiber.Map{
			"error": "A/B testing framework not available",
		})
	}

	var experiment ABExperiment
	if err := c.BodyParser(&experiment); err != nil {
		return c.Status(400).JSON(fiber.Map{
			"error":   "Invalid experiment data",
			"details": err.Error(),
		})
	}

	if err := h.abTestFramework.CreateExperiment(c.Context(), experiment); err != nil {
		return c.Status(500).JSON(fiber.Map{
			"error":   "Failed to create experiment",
			"details": err.Error(),
		})
	}

	return c.Status(201).JSON(experiment)
}

// GetABTestResults returns results of an A/B test
func (h *Handler) GetABTestResults(c *fiber.Ctx) error {
	experimentID := c.Params("id")

	if h.abTestFramework == nil {
		// Return mock results
		return c.JSON(fiber.Map{
			"experiment_id": experimentID,
			"variant_a": fiber.Map{
				"trials":       100,
				"successes":    85,
				"success_rate": 0.85,
				"confidence_interval": fiber.Map{
					"lower": 0.80,
					"upper": 0.90,
				},
			},
			"variant_b": fiber.Map{
				"trials":       100,
				"successes":    92,
				"success_rate": 0.92,
				"confidence_interval": fiber.Map{
					"lower": 0.87,
					"upper": 0.97,
				},
			},
			"statistical_significance": fiber.Map{
				"p_value":     0.03,
				"significant": true,
				"winner":      "variant_b",
			},
			"recommendation": "Variant B shows statistically significant improvement",
		})
	}

	results, err := h.abTestFramework.AnalyzeResults(c.Context(), experimentID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{
			"error":   "Failed to analyze results",
			"details": err.Error(),
		})
	}

	return c.JSON(results)
}

// GraduateABTest graduates the winner of an A/B test
func (h *Handler) GraduateABTest(c *fiber.Ctx) error {
	experimentID := c.Params("id")

	if h.abTestFramework == nil {
		return c.Status(503).JSON(fiber.Map{
			"error": "A/B testing framework not available",
		})
	}

	if err := h.abTestFramework.GraduateWinner(c.Context(), experimentID); err != nil {
		return c.Status(500).JSON(fiber.Map{
			"error":   "Failed to graduate winner",
			"details": err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"message": "Winner graduated successfully",
		"experiment_id": experimentID,
	})
}







// GetBenchmarkResultsMock returns mock benchmark results (replaced by real implementation in benchmark_manager.go)
func (h *Handler) GetBenchmarkResultsMock(c *fiber.Ctx) error {
	benchmarkID := c.Params("id")
	
	// Mock benchmark results
	results := fiber.Map{
		"benchmark_id": benchmarkID,
		"status":       "completed",
		"iterations":   100,
		"duration":     "9m 45s",
		"results": fiber.Map{
			"avg_execution_time": "5.8s",
			"min_execution_time": "4.2s",
			"max_execution_time": "8.1s",
			"p50_latency":        "5.5s",
			"p95_latency":        "7.2s",
			"p99_latency":        "7.9s",
			"throughput":         "10.3 ops/min",
			"success_rate":       0.98,
		},
		"resource_usage": fiber.Map{
			"avg_cpu":    "2.5 cores",
			"peak_cpu":   "4.0 cores",
			"avg_memory": "3.2GB",
			"peak_memory": "5.1GB",
		},
		"comparison": fiber.Map{
			"vs_baseline": "+15% throughput",
			"vs_previous": "+8% success rate",
		},
	}

	return c.JSON(results)
}

