package arf

import (
	"context"
	"fmt"
	"io/ioutil"
	"sync"
	"time"
	
	"github.com/gofiber/fiber/v2"
	"gopkg.in/yaml.v3"
)

// BenchmarkManager manages benchmark test executions
type BenchmarkManager struct {
	benchmarks  map[string]*RunningBenchmark
	mu          sync.RWMutex
	resultsPath string
}

// RunningBenchmark represents a benchmark in progress
type RunningBenchmark struct {
	ID              string           `json:"id"`
	Config          *BenchmarkConfig `json:"config"`
	Suite           *BenchmarkSuite  `json:"-"`
	Status          string           `json:"status"` // running, completed, failed, cancelled
	CurrentIteration int             `json:"current_iteration"`
	StartTime       time.Time        `json:"start_time"`
	EndTime         *time.Time       `json:"end_time,omitempty"`
	Result          *BenchmarkResult `json:"result,omitempty"`
	Errors          []string         `json:"errors,omitempty"`
	cancelFunc      context.CancelFunc
}

// NewBenchmarkManager creates a new benchmark manager
func NewBenchmarkManager(resultsPath string) *BenchmarkManager {
	return &BenchmarkManager{
		benchmarks:  make(map[string]*RunningBenchmark),
		resultsPath: resultsPath,
	}
}

// RunBenchmarkSuite handles POST /benchmark/run
func (h *Handler) RunBenchmarkSuite(c *fiber.Ctx) error {
	var req struct {
		ConfigFile string                 `json:"config_file"`
		Config     *BenchmarkConfig       `json:"config"`
		OutputDir  string                 `json:"output_dir"`
		Options    map[string]interface{} `json:"options"`
	}
	
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}
	
	// Load config from file or use provided config
	var config *BenchmarkConfig
	if req.ConfigFile != "" {
		data, err := ioutil.ReadFile(req.ConfigFile)
		if err != nil {
			return c.Status(400).JSON(fiber.Map{
				"error": fmt.Sprintf("Failed to read config file: %v", err),
			})
		}
		
		config = &BenchmarkConfig{}
		if err := yaml.Unmarshal(data, config); err != nil {
			return c.Status(400).JSON(fiber.Map{
				"error": fmt.Sprintf("Failed to parse config file: %v", err),
			})
		}
	} else if req.Config != nil {
		config = req.Config
	} else {
		return c.Status(400).JSON(fiber.Map{
			"error": "Either config_file or config must be provided",
		})
	}
	
	// Override output dir if provided
	if req.OutputDir != "" {
		config.OutputDir = req.OutputDir
	}
	
	// Create benchmark suite
	suite, err := NewBenchmarkSuite(config)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{
			"error": fmt.Sprintf("Failed to create benchmark suite: %v", err),
		})
	}
	
	// Create running benchmark
	benchmarkID := fmt.Sprintf("bench-%d", time.Now().Unix())
	ctx, cancel := context.WithCancel(context.Background())
	
	running := &RunningBenchmark{
		ID:               benchmarkID,
		Config:           config,
		Suite:            suite,
		Status:           "running",
		CurrentIteration: 0,
		StartTime:        time.Now(),
		cancelFunc:       cancel,
	}
	
	// Store in manager
	if h.benchmarkManager == nil {
		h.benchmarkManager = NewBenchmarkManager("./benchmark_results")
	}
	
	h.benchmarkManager.mu.Lock()
	h.benchmarkManager.benchmarks[benchmarkID] = running
	h.benchmarkManager.mu.Unlock()
	
	// Run benchmark asynchronously
	go func() {
		result, err := suite.Run(ctx)
		
		h.benchmarkManager.mu.Lock()
		if err != nil {
			running.Status = "failed"
			running.Errors = append(running.Errors, err.Error())
		} else {
			running.Status = "completed"
			running.Result = result
		}
		endTime := time.Now()
		running.EndTime = &endTime
		h.benchmarkManager.mu.Unlock()
	}()
	
	return c.JSON(fiber.Map{
		"benchmark_id": benchmarkID,
		"status":       "started",
		"message":      "Benchmark test started successfully",
	})
}

// GetBenchmarkStatus handles GET /benchmark/status/:id
func (h *Handler) GetBenchmarkStatus(c *fiber.Ctx) error {
	benchmarkID := c.Params("id")
	
	if h.benchmarkManager == nil {
		return c.Status(404).JSON(fiber.Map{
			"error": "No benchmarks found",
		})
	}
	
	h.benchmarkManager.mu.RLock()
	running, exists := h.benchmarkManager.benchmarks[benchmarkID]
	h.benchmarkManager.mu.RUnlock()
	
	if !exists {
		return c.Status(404).JSON(fiber.Map{
			"error": "Benchmark not found",
		})
	}
	
	response := fiber.Map{
		"benchmark_id":      running.ID,
		"status":           running.Status,
		"current_iteration": running.CurrentIteration,
		"start_time":       running.StartTime,
	}
	
	if running.EndTime != nil {
		response["end_time"] = *running.EndTime
		response["duration"] = running.EndTime.Sub(running.StartTime).String()
	}
	
	if len(running.Errors) > 0 {
		response["errors"] = running.Errors
	}
	
	return c.JSON(response)
}

// GetBenchmarkResults handles GET /benchmark/results/:id
func (h *Handler) GetBenchmarkResults(c *fiber.Ctx) error {
	benchmarkID := c.Params("id")
	
	if h.benchmarkManager == nil {
		return c.Status(404).JSON(fiber.Map{
			"error": "No benchmarks found",
		})
	}
	
	h.benchmarkManager.mu.RLock()
	running, exists := h.benchmarkManager.benchmarks[benchmarkID]
	h.benchmarkManager.mu.RUnlock()
	
	if !exists {
		return c.Status(404).JSON(fiber.Map{
			"error": "Benchmark not found",
		})
	}
	
	if running.Status != "completed" {
		return c.Status(400).JSON(fiber.Map{
			"error": "Benchmark not completed yet",
			"status": running.Status,
		})
	}
	
	if running.Result == nil {
		return c.Status(404).JSON(fiber.Map{
			"error": "Results not available",
		})
	}
	
	return c.JSON(running.Result)
}

// GetBenchmarkErrors handles GET /benchmark/errors/:id
func (h *Handler) GetBenchmarkErrors(c *fiber.Ctx) error {
	benchmarkID := c.Params("id")
	
	if h.benchmarkManager == nil {
		return c.Status(404).JSON(fiber.Map{
			"error": "No benchmarks found",
		})
	}
	
	h.benchmarkManager.mu.RLock()
	running, exists := h.benchmarkManager.benchmarks[benchmarkID]
	h.benchmarkManager.mu.RUnlock()
	
	if !exists {
		return c.Status(404).JSON(fiber.Map{
			"error": "Benchmark not found",
		})
	}
	
	errors := []ErrorCapture{}
	if running.Result != nil {
		for _, iter := range running.Result.Iterations {
			errors = append(errors, iter.Errors...)
		}
	}
	
	return c.JSON(fiber.Map{
		"benchmark_id": benchmarkID,
		"status":      running.Status,
		"errors":      errors,
		"error_count": len(errors),
	})
}

// CompareBenchmarks handles POST /benchmark/compare
func (h *Handler) CompareBenchmarks(c *fiber.Ctx) error {
	var req struct {
		Results []string `json:"results"`
	}
	
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}
	
	if len(req.Results) < 2 {
		return c.Status(400).JSON(fiber.Map{
			"error": "At least 2 benchmark IDs required for comparison",
		})
	}
	
	if h.benchmarkManager == nil {
		return c.Status(404).JSON(fiber.Map{
			"error": "No benchmarks found",
		})
	}
	
	// Collect results
	var results []*BenchmarkResult
	h.benchmarkManager.mu.RLock()
	for _, id := range req.Results {
		if running, exists := h.benchmarkManager.benchmarks[id]; exists {
			if running.Result != nil {
				results = append(results, running.Result)
			}
		}
	}
	h.benchmarkManager.mu.RUnlock()
	
	if len(results) < 2 {
		return c.Status(400).JSON(fiber.Map{
			"error": "Not enough completed benchmarks for comparison",
		})
	}
	
	// Perform comparison
	comparison := CompareBenchmarks(results)
	
	return c.JSON(comparison)
}

// GenerateBenchmarkReport handles POST /benchmark/report/:id
func (h *Handler) GenerateBenchmarkReport(c *fiber.Ctx) error {
	benchmarkID := c.Params("id")
	
	var req struct {
		Format       string `json:"format"` // html, pdf, markdown
		IncludeDiffs bool   `json:"include_diffs"`
	}
	
	if err := c.BodyParser(&req); err != nil {
		req.Format = "html"
		req.IncludeDiffs = true
	}
	
	if h.benchmarkManager == nil {
		return c.Status(404).JSON(fiber.Map{
			"error": "No benchmarks found",
		})
	}
	
	h.benchmarkManager.mu.RLock()
	running, exists := h.benchmarkManager.benchmarks[benchmarkID]
	h.benchmarkManager.mu.RUnlock()
	
	if !exists {
		return c.Status(404).JSON(fiber.Map{
			"error": "Benchmark not found",
		})
	}
	
	if running.Result == nil {
		return c.Status(400).JSON(fiber.Map{
			"error": "No results available for report generation",
		})
	}
	
	// Generate report URL (in production, this would generate actual report)
	reportURL := fmt.Sprintf("/v1/arf/benchmark/reports/%s.%s", benchmarkID, req.Format)
	
	return c.JSON(fiber.Map{
		"benchmark_id": benchmarkID,
		"report_url":   reportURL,
		"format":       req.Format,
		"generated_at": time.Now(),
	})
}

// ListBenchmarks handles GET /benchmark/list
func (h *Handler) ListBenchmarks(c *fiber.Ctx) error {
	if h.benchmarkManager == nil {
		return c.JSON(fiber.Map{
			"benchmarks": []fiber.Map{},
			"total":      0,
		})
	}
	
	h.benchmarkManager.mu.RLock()
	defer h.benchmarkManager.mu.RUnlock()
	
	benchmarks := []fiber.Map{}
	for id, running := range h.benchmarkManager.benchmarks {
		benchmark := fiber.Map{
			"id":         id,
			"name":       running.Config.Name,
			"status":     running.Status,
			"start_time": running.StartTime,
		}
		
		if running.EndTime != nil {
			benchmark["end_time"] = *running.EndTime
			benchmark["duration"] = running.EndTime.Sub(running.StartTime).String()
		}
		
		if running.Result != nil {
			benchmark["iterations"] = running.Result.Summary.TotalIterations
			benchmark["success_rate"] = fmt.Sprintf("%.1f%%", 
				float64(running.Result.Summary.SuccessfulIterations) / 
				float64(running.Result.Summary.TotalIterations) * 100)
		}
		
		benchmarks = append(benchmarks, benchmark)
	}
	
	return c.JSON(fiber.Map{
		"benchmarks": benchmarks,
		"total":      len(benchmarks),
	})
}

// CancelBenchmark handles DELETE /benchmark/:id
func (h *Handler) CancelBenchmark(c *fiber.Ctx) error {
	benchmarkID := c.Params("id")
	
	if h.benchmarkManager == nil {
		return c.Status(404).JSON(fiber.Map{
			"error": "No benchmarks found",
		})
	}
	
	h.benchmarkManager.mu.Lock()
	running, exists := h.benchmarkManager.benchmarks[benchmarkID]
	if exists && running.Status == "running" {
		running.cancelFunc()
		running.Status = "cancelled"
		endTime := time.Now()
		running.EndTime = &endTime
	}
	h.benchmarkManager.mu.Unlock()
	
	if !exists {
		return c.Status(404).JSON(fiber.Map{
			"error": "Benchmark not found",
		})
	}
	
	return c.JSON(fiber.Map{
		"benchmark_id": benchmarkID,
		"status":       "cancelled",
		"message":      "Benchmark cancelled successfully",
	})
}