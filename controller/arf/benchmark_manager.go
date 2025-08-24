package arf

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"sync"
	"time"
	
	"github.com/gofiber/fiber/v2"
	"github.com/hashicorp/consul/api"
	"gopkg.in/yaml.v3"
)

// BenchmarkManager manages benchmark test executions with distributed storage
type BenchmarkManager struct {
	benchmarks  map[string]*RunningBenchmark // Local cache for running benchmarks
	mu          sync.RWMutex
	resultsPath string
	consulClient *api.Client
	consulPrefix string // Key prefix for Consul storage
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

// NewBenchmarkManager creates a new benchmark manager with Consul storage
func NewBenchmarkManager(resultsPath string) *BenchmarkManager {
	// Initialize Consul client
	consulConfig := api.DefaultConfig()
	consulClient, err := api.NewClient(consulConfig)
	if err != nil {
		// Fall back to in-memory only if Consul is not available
		fmt.Printf("Warning: Failed to connect to Consul, using in-memory storage only: %v\n", err)
		consulClient = nil
	}
	
	return &BenchmarkManager{
		benchmarks:   make(map[string]*RunningBenchmark),
		resultsPath:  resultsPath,
		consulClient: consulClient,
		consulPrefix: "arf/benchmarks",
	}
}

// storeBenchmark stores a benchmark in both local cache and Consul
func (bm *BenchmarkManager) storeBenchmark(benchmark *RunningBenchmark) error {
	// Store in local cache first
	bm.benchmarks[benchmark.ID] = benchmark
	
	// Store in Consul if available
	if bm.consulClient != nil {
		// Create serializable version (exclude Suite and cancelFunc)
		serializable := &RunningBenchmark{
			ID:               benchmark.ID,
			Config:           benchmark.Config,
			Suite:            nil, // Cannot serialize, will be nil
			Status:           benchmark.Status,
			CurrentIteration: benchmark.CurrentIteration,
			StartTime:        benchmark.StartTime,
			EndTime:          benchmark.EndTime,
			Result:           benchmark.Result,
			Errors:           benchmark.Errors,
			cancelFunc:       nil, // Cannot serialize
		}
		
		data, err := json.Marshal(serializable)
		if err != nil {
			return fmt.Errorf("failed to marshal benchmark: %v", err)
		}
		
		key := fmt.Sprintf("%s/%s", bm.consulPrefix, benchmark.ID)
		kv := bm.consulClient.KV()
		_, err = kv.Put(&api.KVPair{
			Key:   key,
			Value: data,
		}, nil)
		if err != nil {
			fmt.Printf("Warning: Failed to store benchmark in Consul: %v\n", err)
		}
	}
	
	return nil
}

// getBenchmark retrieves a benchmark from local cache or Consul
func (bm *BenchmarkManager) getBenchmark(benchmarkID string) (*RunningBenchmark, bool) {
	// Check local cache first
	if benchmark, exists := bm.benchmarks[benchmarkID]; exists {
		return benchmark, true
	}
	
	// Check Consul if available
	if bm.consulClient != nil {
		key := fmt.Sprintf("%s/%s", bm.consulPrefix, benchmarkID)
		kv := bm.consulClient.KV()
		pair, _, err := kv.Get(key, nil)
		if err != nil {
			fmt.Printf("Warning: Failed to get benchmark from Consul: %v\n", err)
			return nil, false
		}
		
		if pair != nil {
			var benchmark RunningBenchmark
			if err := json.Unmarshal(pair.Value, &benchmark); err != nil {
				fmt.Printf("Warning: Failed to unmarshal benchmark from Consul: %v\n", err)
				return nil, false
			}
			
			// Store in local cache for future access
			bm.benchmarks[benchmarkID] = &benchmark
			return &benchmark, true
		}
	}
	
	return nil, false
}

// updateBenchmark updates a benchmark in both local cache and Consul
func (bm *BenchmarkManager) updateBenchmark(benchmark *RunningBenchmark) error {
	// Update local cache
	bm.benchmarks[benchmark.ID] = benchmark
	
	// Update in Consul if available
	if bm.consulClient != nil {
		// Create serializable version (exclude Suite and cancelFunc)
		serializable := &RunningBenchmark{
			ID:               benchmark.ID,
			Config:           benchmark.Config,
			Suite:            nil, // Cannot serialize
			Status:           benchmark.Status,
			CurrentIteration: benchmark.CurrentIteration,
			StartTime:        benchmark.StartTime,
			EndTime:          benchmark.EndTime,
			Result:           benchmark.Result,
			Errors:           benchmark.Errors,
			cancelFunc:       nil, // Cannot serialize
		}
		
		data, err := json.Marshal(serializable)
		if err != nil {
			return fmt.Errorf("failed to marshal benchmark: %v", err)
		}
		
		key := fmt.Sprintf("%s/%s", bm.consulPrefix, benchmark.ID)
		kv := bm.consulClient.KV()
		_, err = kv.Put(&api.KVPair{
			Key:   key,
			Value: data,
		}, nil)
		if err != nil {
			fmt.Printf("Warning: Failed to update benchmark in Consul: %v\n", err)
		}
	}
	
	return nil
}

// listAllBenchmarks retrieves all benchmarks from both local cache and Consul
func (bm *BenchmarkManager) listAllBenchmarks() map[string]*RunningBenchmark {
	allBenchmarks := make(map[string]*RunningBenchmark)
	
	// Add local benchmarks
	for id, benchmark := range bm.benchmarks {
		allBenchmarks[id] = benchmark
	}
	
	// Add benchmarks from Consul if available
	if bm.consulClient != nil {
		kv := bm.consulClient.KV()
		pairs, _, err := kv.List(bm.consulPrefix, nil)
		if err != nil {
			fmt.Printf("Warning: Failed to list benchmarks from Consul: %v\n", err)
		} else {
			for _, pair := range pairs {
				var benchmark RunningBenchmark
				if err := json.Unmarshal(pair.Value, &benchmark); err != nil {
					fmt.Printf("Warning: Failed to unmarshal benchmark from Consul: %v\n", err)
					continue
				}
				
				// Only add if not already in local cache
				if _, exists := allBenchmarks[benchmark.ID]; !exists {
					allBenchmarks[benchmark.ID] = &benchmark
				}
			}
		}
	}
	
	return allBenchmarks
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
		return c.Status(500).JSON(fiber.Map{
			"error": "Benchmark manager not initialized",
		})
	}
	
	h.benchmarkManager.mu.Lock()
	err = h.benchmarkManager.storeBenchmark(running)
	h.benchmarkManager.mu.Unlock()
	
	if err != nil {
		return c.Status(500).JSON(fiber.Map{
			"error": fmt.Sprintf("Failed to store benchmark: %v", err),
		})
	}
	
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
		
		// Update in distributed storage
		h.benchmarkManager.updateBenchmark(running)
		h.benchmarkManager.mu.Unlock()
	}()
	
	return c.JSON(fiber.Map{
		"benchmark_id": benchmarkID,
		"status":       "started",
		"message":      "Benchmark test started successfully",
	})
}

// GetBenchmark handles GET /benchmark/:id to get full benchmark details
func (h *Handler) GetBenchmark(c *fiber.Ctx) error {
	benchmarkID := c.Params("id")
	
	if h.benchmarkManager == nil {
		return c.Status(404).JSON(fiber.Map{
			"error": "No benchmarks found",
		})
	}
	
	h.benchmarkManager.mu.RLock()
	benchmark, exists := h.benchmarkManager.getBenchmark(benchmarkID)
	h.benchmarkManager.mu.RUnlock()
	
	if !exists {
		return c.Status(404).JSON(fiber.Map{
			"error": "Benchmark not found",
		})
	}
	
	// Return the full benchmark object including all details
	return c.JSON(benchmark)
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
	running, exists := h.benchmarkManager.getBenchmark(benchmarkID)
	h.benchmarkManager.mu.RUnlock()
	
	if !exists {
		return c.Status(404).JSON(fiber.Map{
			"error": "Benchmark not found",
		})
	}
	
	// Build comprehensive status response with all benchmark details
	response := fiber.Map{
		"benchmark_id":      running.ID,
		"status":           running.Status,
		"current_iteration": running.CurrentIteration,
		"start_time":       running.StartTime,
		
		// Include benchmark configuration details
		"name":             running.Config.Name,
		"repository":       running.Config.RepoURL,
		"repo_branch":      running.Config.RepoBranch,
		"task_type":        running.Config.TaskType,
		"source_lang":      running.Config.SourceLang,
		"target_spec":      running.Config.TargetSpec,
		"recipe_ids":       running.Config.RecipeIDs,
		"max_iterations":   running.Config.MaxIterations,
		
		// Deployment configuration details
		"app_name":         "",  // Will be filled from deployment config
		"lane":            "",   // Will be filled from deployment config
		
		// Progress calculation
		"progress":         0.0,
	}
	
	// Extract deployment configuration if available
	if running.Config.DeploymentConfig != nil {
		deployConfig := running.Config.DeploymentConfig
		if appName, exists := deployConfig["app_name"]; exists {
			response["app_name"] = appName
		}
		if lane, exists := deployConfig["lane"]; exists {
			response["lane"] = lane
		}
	}
	
	// Calculate progress based on iteration and stages
	if running.Config.MaxIterations > 0 {
		progress := float64(running.CurrentIteration) / float64(running.Config.MaxIterations)
		response["progress"] = progress
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

// GetBenchmarkLogs handles GET /benchmark/logs/:id
func (h *Handler) GetBenchmarkLogs(c *fiber.Ctx) error {
	benchmarkID := c.Params("id")
	stage := c.Query("stage", "all")
	
	if h.benchmarkManager == nil {
		return c.Status(404).JSON(fiber.Map{
			"error": "No benchmarks found",
		})
	}
	
	h.benchmarkManager.mu.RLock()
	running, exists := h.benchmarkManager.getBenchmark(benchmarkID)
	h.benchmarkManager.mu.RUnlock()
	
	if !exists {
		return c.Status(404).JSON(fiber.Map{
			"error": "Benchmark not found",
		})
	}
	
	// For now, return mock logs until we implement proper logging
	mockLogs := []map[string]interface{}{
		{
			"timestamp": running.StartTime,
			"level":     "INFO",
			"stage":     "initialization",
			"message":   fmt.Sprintf("Starting benchmark %s", running.Config.Name),
		},
		{
			"timestamp": running.StartTime.Add(1 * time.Second),
			"level":     "INFO",
			"stage":     "repository_preparation",
			"message":   fmt.Sprintf("Cloning repository: %s", running.Config.RepoURL),
		},
		{
			"timestamp": running.StartTime.Add(5 * time.Second),
			"level":     "INFO",
			"stage":     "openrewrite_transform",
			"message":   fmt.Sprintf("Applying %d OpenRewrite recipes", len(running.Config.RecipeIDs)),
		},
		{
			"timestamp": running.StartTime.Add(10 * time.Second),
			"level":     "INFO",
			"stage":     "deployment",
			"message":   "Deploying transformed application to Ploy",
		},
		{
			"timestamp": running.StartTime.Add(20 * time.Second),
			"level":     "INFO",
			"stage":     "application_testing",
			"message":   "Testing deployed application endpoints",
		},
	}
	
	// Add completion log if benchmark is done
	if running.EndTime != nil {
		mockLogs = append(mockLogs, map[string]interface{}{
			"timestamp": *running.EndTime,
			"level":     "INFO", 
			"stage":     "completion",
			"message":   fmt.Sprintf("Benchmark completed with status: %s", running.Status),
		})
	}
	
	// Filter logs by stage if specified
	filteredLogs := mockLogs
	if stage != "all" {
		filteredLogs = []map[string]interface{}{}
		for _, log := range mockLogs {
			if log["stage"] == stage {
				filteredLogs = append(filteredLogs, log)
			}
		}
	}
	
	return c.JSON(fiber.Map{
		"benchmark_id": benchmarkID,
		"stage":        stage,
		"logs":         filteredLogs,
	})
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
	running, exists := h.benchmarkManager.getBenchmark(benchmarkID)
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
	running, exists := h.benchmarkManager.getBenchmark(benchmarkID)
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
		if running, exists := h.benchmarkManager.getBenchmark(id); exists {
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
	running, exists := h.benchmarkManager.getBenchmark(benchmarkID)
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
	
	allBenchmarks := h.benchmarkManager.listAllBenchmarks()
	benchmarks := []fiber.Map{}
	for id, running := range allBenchmarks {
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
	running, exists := h.benchmarkManager.getBenchmark(benchmarkID)
	if exists && running.Status == "running" {
		// Note: cancelFunc may not be available for benchmarks loaded from Consul
		if running.cancelFunc != nil {
			running.cancelFunc()
		}
		running.Status = "cancelled"
		endTime := time.Now()
		running.EndTime = &endTime
		
		// Update in distributed storage
		h.benchmarkManager.updateBenchmark(running)
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