package arf

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// BenchmarkConfig defines configuration for a benchmark test run
type BenchmarkConfig struct {
	// Test identification
	Name        string `json:"name" yaml:"name"`
	Description string `json:"description" yaml:"description"`
	
	// Repository configuration
	RepoURL      string `json:"repo_url" yaml:"repo_url"`
	RepoBranch   string `json:"repo_branch" yaml:"repo_branch"`
	LocalPath    string `json:"local_path" yaml:"local_path"`
	
	// Task configuration
	TaskType     string   `json:"task_type" yaml:"task_type"` // migration, security, cleanup, refactor
	SourceLang   string   `json:"source_lang" yaml:"source_lang"`
	TargetSpec   string   `json:"target_spec" yaml:"target_spec"` // e.g., "java:17", "spring-boot:3.0"
	RecipeIDs    []string `json:"recipe_ids" yaml:"recipe_ids"`
	
	// LLM configuration
	LLMProvider  string            `json:"llm_provider" yaml:"llm_provider"` // openai, ollama, anthropic, azure
	LLMModel     string            `json:"llm_model" yaml:"llm_model"`
	LLMOptions   map[string]string `json:"llm_options" yaml:"llm_options"`
	
	// Iteration control
	MaxIterations      int           `json:"max_iterations" yaml:"max_iterations"`
	TimeoutPerIteration time.Duration `json:"timeout_per_iteration" yaml:"timeout_per_iteration"`
	StopOnSuccess      bool          `json:"stop_on_success" yaml:"stop_on_success"`
	
	// Output configuration
	OutputDir          string `json:"output_dir" yaml:"output_dir"`
	CaptureFullDiffs   bool   `json:"capture_full_diffs" yaml:"capture_full_diffs"`
	CapturePartialDiffs bool   `json:"capture_partial_diffs" yaml:"capture_partial_diffs"`
	SaveIntermediateState bool `json:"save_intermediate_state" yaml:"save_intermediate_state"`
}

// BenchmarkIteration represents a single iteration in the benchmark
type BenchmarkIteration struct {
	Number      int                    `json:"number"`
	StartTime   time.Time              `json:"start_time"`
	EndTime     time.Time              `json:"end_time"`
	Duration    time.Duration          `json:"duration"`
	Status      string                 `json:"status"` // success, partial, failed, timeout
	Stages      []BenchmarkStage       `json:"stages"`
	Diffs       []DiffCapture          `json:"diffs"`
	Errors      []ErrorCapture         `json:"errors"`
	Metrics     IterationMetrics       `json:"metrics"`
	LLMCalls    []LLMCallMetrics       `json:"llm_calls"`
}

// BenchmarkStage represents a stage within an iteration
type BenchmarkStage struct {
	Name      string        `json:"name"`
	StartTime time.Time     `json:"start_time"`
	EndTime   time.Time     `json:"end_time"`
	Duration  time.Duration `json:"duration"`
	Status    string        `json:"status"`
	Details   interface{}   `json:"details,omitempty"`
}

// DiffCapture captures code changes made during an iteration
type DiffCapture struct {
	File      string    `json:"file"`
	Type      string    `json:"type"` // added, modified, deleted
	Before    string    `json:"before,omitempty"`
	After     string    `json:"after,omitempty"`
	UnifiedDiff string  `json:"unified_diff"`
	Timestamp time.Time `json:"timestamp"`
}

// ErrorCapture captures errors during execution
type ErrorCapture struct {
	Stage     string    `json:"stage"`
	Type      string    `json:"type"` // compile, test, validation, runtime
	Message   string    `json:"message"`
	Details   string    `json:"details,omitempty"`
	StackTrace string   `json:"stack_trace,omitempty"`
	Timestamp time.Time `json:"timestamp"`
}

// IterationMetrics captures metrics for an iteration
type IterationMetrics struct {
	FilesAnalyzed     int     `json:"files_analyzed"`
	FilesModified     int     `json:"files_modified"`
	LinesAdded        int     `json:"lines_added"`
	LinesRemoved      int     `json:"lines_removed"`
	CompileSuccess    bool    `json:"compile_success"`
	TestsRun          int     `json:"tests_run"`
	TestsPassed       int     `json:"tests_passed"`
	CoveragePercent   float64 `json:"coverage_percent,omitempty"`
	ComplexityDelta   int     `json:"complexity_delta,omitempty"`
}

// LLMCallMetrics tracks LLM usage
type LLMCallMetrics struct {
	Purpose      string        `json:"purpose"`
	Model        string        `json:"model"`
	InputTokens  int           `json:"input_tokens"`
	OutputTokens int           `json:"output_tokens"`
	Duration     time.Duration `json:"duration"`
	Cost         float64       `json:"cost,omitempty"`
	Success      bool          `json:"success"`
	Error        string        `json:"error,omitempty"`
}

// BenchmarkResult represents the complete result of a benchmark run
type BenchmarkResult struct {
	Config       BenchmarkConfig      `json:"config"`
	StartTime    time.Time           `json:"start_time"`
	EndTime      time.Time           `json:"end_time"`
	TotalDuration time.Duration       `json:"total_duration"`
	Iterations   []BenchmarkIteration `json:"iterations"`
	Summary      BenchmarkSummary     `json:"summary"`
	Comparison   *ComparisonResult    `json:"comparison,omitempty"`
}

// BenchmarkSummary provides high-level metrics
type BenchmarkSummary struct {
	TotalIterations    int           `json:"total_iterations"`
	SuccessfulIterations int         `json:"successful_iterations"`
	PartialIterations  int           `json:"partial_iterations"`
	FailedIterations   int           `json:"failed_iterations"`
	AverageIterationTime time.Duration `json:"average_iteration_time"`
	TotalLLMCalls      int           `json:"total_llm_calls"`
	TotalLLMTokens     int           `json:"total_llm_tokens"`
	TotalLLMCost       float64       `json:"total_llm_cost"`
	FinalCompileStatus bool          `json:"final_compile_status"`
	FinalTestStatus    bool          `json:"final_test_status"`
	TotalFilesModified int           `json:"total_files_modified"`
	TotalLinesChanged  int           `json:"total_lines_changed"`
}

// ComparisonResult compares multiple benchmark runs
type ComparisonResult struct {
	BaselineRun   string                 `json:"baseline_run"`
	ComparedRuns  []string               `json:"compared_runs"`
	Metrics       map[string]interface{} `json:"metrics"`
	Winner        string                 `json:"winner"`
	Analysis      string                 `json:"analysis"`
}

// BenchmarkSuite manages benchmark test execution
type BenchmarkSuite struct {
	config          *BenchmarkConfig
	llmGenerator    LLMRecipeGenerator
	arfEngine       ARFEngine
	multiLangEngine MultiLanguageEngine
	outputWriter    io.Writer
	gitOps          *GitOperations
	buildOps        *BuildOperations
	mockOpenRewrite *MockOpenRewriteEngine
	sandboxMgr      SandboxManager
}

// NewBenchmarkSuite creates a new benchmark suite
func NewBenchmarkSuite(config *BenchmarkConfig) (*BenchmarkSuite, error) {
	// Create LLM generator based on provider
	var llmGen LLMRecipeGenerator
	var err error
	
	switch config.LLMProvider {
	case "ollama":
		// Extract Ollama configuration from config
		model := config.LLMModel
		baseURL := "http://localhost:11434" // default
		temperature := 0.1 // default
		contextLength := 4096 // default
		
		// Override with options if provided
		if config.LLMOptions != nil {
			if url, ok := config.LLMOptions["base_url"]; ok && url != "" {
				baseURL = url
			}
			if tempStr, ok := config.LLMOptions["temperature"]; ok && tempStr != "" {
				if temp, parseErr := strconv.ParseFloat(tempStr, 64); parseErr == nil {
					temperature = temp
				}
			}
			if tokensStr, ok := config.LLMOptions["max_tokens"]; ok && tokensStr != "" {
				if tokens, parseErr := strconv.Atoi(tokensStr); parseErr == nil {
					contextLength = tokens
				}
			}
		}
		
		llmGen, err = NewOllamaLLMGeneratorWithConfig(model, baseURL, temperature, contextLength)
		if err != nil {
			return nil, fmt.Errorf("failed to create Ollama generator: %w", err)
		}
	case "openai":
		llmGen, err = NewOpenAILLMGenerator()
		if err != nil {
			return nil, fmt.Errorf("failed to create OpenAI generator: %w", err)
		}
	case "anthropic":
		// TODO: Implement Anthropic provider
		return nil, fmt.Errorf("Anthropic provider not yet implemented")
	case "azure":
		// TODO: Implement Azure provider
		return nil, fmt.Errorf("Azure provider not yet implemented")
	default:
		return nil, fmt.Errorf("unsupported LLM provider: %s", config.LLMProvider)
	}
	
	// Create multi-language engine
	multiLang, err := NewTreeSitterMultiLanguageEngine()
	if err != nil {
		return nil, fmt.Errorf("failed to create multi-language engine: %w", err)
	}
	
	// Create output directory if needed
	if config.OutputDir != "" {
		if err := os.MkdirAll(config.OutputDir, 0755); err != nil {
			return nil, fmt.Errorf("failed to create output directory: %w", err)
		}
	}
	
	// Create sandbox manager (will use deployment integration if PLOY_CONTROLLER is set)
	sandboxMgr := NewSandboxManagerForOS(
		"/tmp/arf-jails", 
		"/tmp/arf-templates", 
		5, 
		config.TimeoutPerIteration*2, 
		"lo0",
	)
	
	return &BenchmarkSuite{
		config:          config,
		llmGenerator:    llmGen,
		multiLangEngine: multiLang,
		gitOps:          NewGitOperations(config.OutputDir),
		buildOps:        NewBuildOperations(config.TimeoutPerIteration),
		mockOpenRewrite: NewMockOpenRewriteEngine(),
		sandboxMgr:     sandboxMgr,
	}, nil
}

// Run executes the benchmark test
func (bs *BenchmarkSuite) Run(ctx context.Context) (*BenchmarkResult, error) {
	result := &BenchmarkResult{
		Config:    *bs.config,
		StartTime: time.Now(),
	}
	
	// Clone or prepare repository
	repoPath, err := bs.prepareRepository()
	if err != nil {
		return nil, fmt.Errorf("failed to prepare repository: %w", err)
	}
	
	// Run iterations
	for i := 0; i < bs.config.MaxIterations; i++ {
		iteration, err := bs.runIteration(ctx, i+1, repoPath)
		if err != nil {
			fmt.Printf("Iteration %d failed: %v\n", i+1, err)
		}
		
		result.Iterations = append(result.Iterations, *iteration)
		
		// Check if we should stop
		if bs.config.StopOnSuccess && iteration.Status == "success" {
			break
		}
		
		// Save intermediate state if configured
		if bs.config.SaveIntermediateState {
			bs.saveIntermediateState(i+1, repoPath, iteration)
		}
	}
	
	// Generate summary
	result.EndTime = time.Now()
	result.TotalDuration = result.EndTime.Sub(result.StartTime)
	result.Summary = bs.generateSummary(result)
	
	// Save final result
	if err := bs.saveResult(result); err != nil {
		fmt.Printf("Warning: failed to save result: %v\n", err)
	}
	
	return result, nil
}

// runIteration executes a single benchmark iteration
func (bs *BenchmarkSuite) runIteration(ctx context.Context, number int, repoPath string) (*BenchmarkIteration, error) {
	iteration := &BenchmarkIteration{
		Number:    number,
		StartTime: time.Now(),
	}
	
	// Stage 1: OpenRewrite transformation
	stage1 := bs.runStage("openrewrite_transform", func() error {
		// Apply OpenRewrite recipes
		for _, recipeID := range bs.config.RecipeIDs {
			if err := bs.applyOpenRewriteRecipe(ctx, repoPath, recipeID); err != nil {
				return err
			}
		}
		return nil
	})
	iteration.Stages = append(iteration.Stages, stage1)
	
	// Stage 2: Application Deployment
	var sandbox *Sandbox
	stage2 := bs.runStage("deployment", func() error {
		var err error
		sandbox, err = bs.deployApplication(ctx, repoPath)
		return err
	})
	iteration.Stages = append(iteration.Stages, stage2)
	
	// Stage 3: Application Testing
	stage3 := bs.runStage("application_testing", func() error {
		if sandbox == nil {
			return fmt.Errorf("no sandbox available for testing")
		}
		return bs.testDeployedApp(ctx, sandbox)
	})
	iteration.Stages = append(iteration.Stages, stage3)
	
	// Stage 4: Error detection and self-healing (if deployment/tests failed)
	if stage2.Status != "success" || stage3.Status != "success" {
		stage4 := bs.runStage("error_detection", func() error {
			errors := bs.detectDeploymentErrors(ctx, repoPath, sandbox)
			for _, err := range errors {
				iteration.Errors = append(iteration.Errors, err)
			}
			return nil
		})
		iteration.Stages = append(iteration.Stages, stage4)
		
		// Stage 5: LLM self-healing
		if len(iteration.Errors) > 0 {
			stage5 := bs.runStage("llm_self_healing", func() error {
				return bs.performSelfHealing(ctx, repoPath, iteration.Errors)
			})
			iteration.Stages = append(iteration.Stages, stage5)
			
			// Capture diffs after self-healing
			diffs := bs.captureDiffs(repoPath)
			iteration.Diffs = append(iteration.Diffs, diffs...)
		}
	}
	
	// Stage 6: Cleanup
	stage6 := bs.runStage("cleanup", func() error {
		if sandbox != nil {
			return bs.sandboxMgr.DestroySandbox(ctx, sandbox.ID)
		}
		return nil
	})
	iteration.Stages = append(iteration.Stages, stage6)
	
	// Collect metrics
	iteration.Metrics = bs.collectMetrics(repoPath)
	
	// Determine final status
	iteration.EndTime = time.Now()
	iteration.Duration = iteration.EndTime.Sub(iteration.StartTime)
	iteration.Status = bs.determineIterationStatus(iteration)
	
	return iteration, nil
}

// Helper methods

func (bs *BenchmarkSuite) prepareRepository() (string, error) {
	if bs.config.LocalPath != "" {
		return bs.config.LocalPath, nil
	}
	
	// Create unique directory for this benchmark run
	timestamp := time.Now().Unix()
	tempDir := filepath.Join(os.TempDir(), "arf-benchmark", fmt.Sprintf("%s-%d", bs.config.Name, timestamp))
	
	// Clone the repository
	ctx := context.Background()
	if err := bs.gitOps.CloneRepository(ctx, bs.config.RepoURL, bs.config.RepoBranch, tempDir); err != nil {
		return "", fmt.Errorf("failed to clone repository: %w", err)
	}
	
	fmt.Printf("Repository cloned successfully to: %s\n", tempDir)
	return tempDir, nil
}

func (bs *BenchmarkSuite) runStage(name string, fn func() error) BenchmarkStage {
	stage := BenchmarkStage{
		Name:      name,
		StartTime: time.Now(),
	}
	
	err := fn()
	stage.EndTime = time.Now()
	stage.Duration = stage.EndTime.Sub(stage.StartTime)
	
	if err != nil {
		stage.Status = "failed"
		stage.Details = err.Error()
	} else {
		stage.Status = "success"
	}
	
	return stage
}

// deployApplication deploys the transformed application for testing
func (bs *BenchmarkSuite) deployApplication(ctx context.Context, repoPath string) (*Sandbox, error) {
	// Check if we have a deployment-integrated sandbox manager
	deploySandbox, ok := bs.sandboxMgr.(*DeploymentSandboxManager)
	if !ok {
		return nil, fmt.Errorf("Application deployment requires DeploymentSandboxManager - check PLOY_CONTROLLER environment")
	}

	// Detect language and build tool for sandbox configuration
	buildSystem := bs.buildOps.DetectBuildSystem(repoPath)

	// Determine language from build system
	language := "java" // Default for ARF benchmarks
	switch buildSystem {
	case "maven", "gradle":
		language = "java"
	case "npm", "yarn":
		language = "node"
	case "go":
		language = "go"
	case "python", "pip":
		language = "python"
	}

	// Create sandbox configuration for deployment
	config := SandboxConfig{
		Repository:    repoPath,
		Branch:        "main", // Using local repository
		Language:      language,
		BuildTool:     buildSystem,
		TTL:           bs.config.TimeoutPerIteration * 2, // Double timeout for deployment
		MemoryLimit:   "1G",
		CPULimit:      "2",
		NetworkAccess: true, // Required for health checks
		TempSpace:     "2G",
	}

	// Deploy application
	fmt.Printf("Deploying application (language: %s, build: %s)\n", language, buildSystem)
	sandbox, err := deploySandbox.CreateSandbox(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("Application deployment failed: %w", err)
	}

	fmt.Printf("Application deployed successfully: %s (URL: %s)\n", 
		sandbox.ID, sandbox.Metadata["app_url"])
	
	return sandbox, nil
}

// testDeployedApp tests the deployed application endpoints and functionality
func (bs *BenchmarkSuite) testDeployedApp(ctx context.Context, sandbox *Sandbox) error {
	if sandbox == nil {
		return fmt.Errorf("no sandbox provided for testing")
	}

	appURL, ok := sandbox.Metadata["app_url"]
	if !ok {
		return fmt.Errorf("sandbox missing app_url metadata")
	}

	// Test 1: Health check endpoint
	fmt.Printf("Testing health endpoint: %s/healthz\n", appURL)
	if err := bs.testHealthEndpoint(ctx, appURL+"/healthz"); err != nil {
		return fmt.Errorf("health check failed: %w", err)
	}

	// Test 2: Basic application functionality
	fmt.Printf("Testing application functionality: %s\n", appURL)
	if err := bs.testApplicationEndpoints(ctx, appURL); err != nil {
		return fmt.Errorf("application functionality test failed: %w", err)
	}

	// Test 3: Performance validation
	fmt.Printf("Running performance validation on: %s\n", appURL)
	if err := bs.validatePerformance(ctx, appURL); err != nil {
		// Performance issues are warnings, not failures
		fmt.Printf("Performance validation warning: %v\n", err)
	}

	fmt.Printf("All application tests passed for sandbox: %s\n", sandbox.ID)
	return nil
}

// detectDeploymentErrors analyzes deployment and application errors for self-healing
func (bs *BenchmarkSuite) detectDeploymentErrors(ctx context.Context, repoPath string, sandbox *Sandbox) []ErrorCapture {
	var errors []ErrorCapture

	// Error 1: Deployment logs analysis
	if sandbox != nil {
		if deploySandbox, ok := bs.sandboxMgr.(*DeploymentSandboxManager); ok {
			logs, err := deploySandbox.GetSandboxLogs(ctx, sandbox.ID)
			if err == nil {
				deployErrors := bs.parseDeploymentLogs(logs)
				errors = append(errors, deployErrors...)
			}
		}
	}

	// Error 2: Build system analysis
	buildErrors := bs.analyzeBuildErrors(repoPath)
	errors = append(errors, buildErrors...)

	// Error 3: Application configuration issues
	configErrors := bs.detectConfigurationErrors(repoPath)
	errors = append(errors, configErrors...)

	// Error 4: Dependency and compatibility issues
	depErrors := bs.analyzeDependencies(repoPath)
	errors = append(errors, depErrors...)

	fmt.Printf("Detected %d errors for self-healing analysis\n", len(errors))
	return errors
}

func (bs *BenchmarkSuite) applyOpenRewriteRecipe(ctx context.Context, repoPath string, recipeID string) error {
	// Apply the recipe using mock OpenRewrite engine
	result, err := bs.mockOpenRewrite.ApplyRecipe(ctx, recipeID, repoPath)
	if err != nil {
		return fmt.Errorf("failed to apply recipe %s: %w", recipeID, err)
	}
	
	if !result.Success {
		return fmt.Errorf("recipe %s failed", recipeID)
	}
	
	fmt.Printf("Applied recipe %s: %d changes in %d files\n", 
		recipeID, result.ChangesApplied, len(result.FilesModified))
	
	// Commit the changes to track them
	if result.ChangesApplied > 0 {
		commitMsg := fmt.Sprintf("Applied OpenRewrite recipe: %s", recipeID)
		bs.gitOps.CommitChanges(ctx, repoPath, commitMsg)
	}
	
	return nil
}

func (bs *BenchmarkSuite) validateBuild(ctx context.Context, repoPath string) error {
	// Detect and run build
	buildSystem := bs.buildOps.DetectBuildSystem(repoPath)
	if buildSystem == "unknown" {
		fmt.Printf("Warning: Unknown build system for %s\n", repoPath)
		return nil // Don't fail on unknown build systems
	}
	
	fmt.Printf("Detected build system: %s\n", buildSystem)
	if err := bs.buildOps.ValidateBuild(ctx, repoPath, buildSystem); err != nil {
		return fmt.Errorf("build validation failed: %w", err)
	}
	
	return nil
}

func (bs *BenchmarkSuite) detectErrors(ctx context.Context, repoPath string) []ErrorCapture {
	// Try to run build and capture errors
	buildSystem := bs.buildOps.DetectBuildSystem(repoPath)
	
	// Create a context with timeout for build
	buildCtx, cancel := context.WithTimeout(ctx, bs.config.TimeoutPerIteration)
	defer cancel()
	
	// Run build and capture output
	err := bs.buildOps.ValidateBuild(buildCtx, repoPath, buildSystem)
	if err != nil {
		// Parse errors from build output
		if buildErr, ok := err.(*BuildError); ok {
			return []ErrorCapture{{
				Type:      buildErr.Type,
				Message:   buildErr.Message,
				Details:   buildErr.Details,
				Timestamp: time.Now(),
			}}
		}
		
		// Generic error
		return []ErrorCapture{{
			Type:      "build",
			Message:   err.Error(),
			Timestamp: time.Now(),
		}}
	}
	
	return []ErrorCapture{}
}

func (bs *BenchmarkSuite) performSelfHealing(ctx context.Context, repoPath string, errors []ErrorCapture) error {
	// Use LLM to generate fixes
	for _, err := range errors {
		request := RecipeGenerationRequest{
			ErrorContext: ErrorContext{
				ErrorType:    err.Type,
				ErrorMessage: err.Message,
				SourceFile:   "", // TODO: Extract from error
				CompilerOutput: err.Details,
			},
			CodebaseContext: CodebaseContext{
				Language:    bs.config.SourceLang,
				Framework:   bs.config.TargetSpec,
			},
			Language: bs.config.SourceLang,
		}
		
		recipe, llmErr := bs.llmGenerator.GenerateRecipe(ctx, request)
		if llmErr != nil {
			return fmt.Errorf("LLM generation failed: %w", llmErr)
		}
		
		// Apply the generated fix
		// TODO: Implement fix application
		_ = recipe
	}
	
	return nil
}

func (bs *BenchmarkSuite) captureDiffs(repoPath string) []DiffCapture {
	ctx := context.Background()
	diffs, err := bs.gitOps.GetDiff(ctx, repoPath)
	if err != nil {
		fmt.Printf("Warning: failed to capture diffs: %v\n", err)
		return []DiffCapture{}
	}
	return diffs
}

func (bs *BenchmarkSuite) runTests(ctx context.Context, repoPath string) error {
	// Detect build system and run tests
	buildSystem := bs.buildOps.DetectBuildSystem(repoPath)
	if buildSystem == "unknown" {
		fmt.Printf("Warning: Unknown build system, skipping tests\n")
		return nil
	}
	
	fmt.Printf("Running tests with %s\n", buildSystem)
	results, err := bs.buildOps.RunTests(ctx, repoPath, buildSystem)
	if err != nil {
		return fmt.Errorf("test execution failed: %w", err)
	}
	
	fmt.Printf("Test results: %d passed, %d failed (total: %d)\n", 
		results.Passed, results.Failed, results.Total)
	
	if !results.Success && results.Failed > 0 {
		return fmt.Errorf("tests failed: %d failures", results.Failed)
	}
	
	return nil
}

func (bs *BenchmarkSuite) collectMetrics(repoPath string) IterationMetrics {
	ctx := context.Background()
	metrics := IterationMetrics{}
	
	// Count changed files
	if count, err := bs.gitOps.CountChangedFiles(ctx, repoPath); err == nil {
		metrics.FilesModified = count
	}
	
	// Get line changes
	if added, removed, err := bs.gitOps.GetLineChanges(ctx, repoPath); err == nil {
		metrics.LinesAdded = added
		metrics.LinesRemoved = removed
	}
	
	// TODO: Get compile and test results from build/test execution
	// For now, these will be set by the validateBuild and runTests functions
	
	return metrics
}

func (bs *BenchmarkSuite) determineIterationStatus(iteration *BenchmarkIteration) string {
	// Check if all stages succeeded
	for _, stage := range iteration.Stages {
		if stage.Status != "success" {
			if len(iteration.Errors) > 0 {
				return "failed"
			}
			return "partial"
		}
	}
	return "success"
}

func (bs *BenchmarkSuite) saveIntermediateState(iterationNum int, repoPath string, iteration *BenchmarkIteration) {
	// Save iteration data
	filename := filepath.Join(bs.config.OutputDir, fmt.Sprintf("iteration_%d.json", iterationNum))
	data, _ := json.MarshalIndent(iteration, "", "  ")
	os.WriteFile(filename, data, 0644)
	
	// TODO: Save repository state (git commit or archive)
}

func (bs *BenchmarkSuite) generateSummary(result *BenchmarkResult) BenchmarkSummary {
	summary := BenchmarkSummary{}
	
	summary.TotalIterations = len(result.Iterations)
	
	var totalDuration time.Duration
	for _, iter := range result.Iterations {
		switch iter.Status {
		case "success":
			summary.SuccessfulIterations++
		case "partial":
			summary.PartialIterations++
		case "failed":
			summary.FailedIterations++
		}
		
		totalDuration += iter.Duration
		summary.TotalLLMCalls += len(iter.LLMCalls)
		
		for _, call := range iter.LLMCalls {
			summary.TotalLLMTokens += call.InputTokens + call.OutputTokens
			summary.TotalLLMCost += call.Cost
		}
		
		summary.TotalFilesModified += iter.Metrics.FilesModified
		summary.TotalLinesChanged += iter.Metrics.LinesAdded + iter.Metrics.LinesRemoved
	}
	
	if summary.TotalIterations > 0 {
		summary.AverageIterationTime = totalDuration / time.Duration(summary.TotalIterations)
		
		lastIter := result.Iterations[len(result.Iterations)-1]
		summary.FinalCompileStatus = lastIter.Metrics.CompileSuccess
		summary.FinalTestStatus = lastIter.Metrics.TestsPassed == lastIter.Metrics.TestsRun
	}
	
	return summary
}

func (bs *BenchmarkSuite) saveResult(result *BenchmarkResult) error {
	// Save comprehensive JSON result
	filename := filepath.Join(bs.config.OutputDir, fmt.Sprintf("benchmark_%s_%d.json", 
		bs.config.Name, time.Now().Unix()))
	
	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return err
	}
	
	if err := os.WriteFile(filename, data, 0644); err != nil {
		return err
	}
	
	// Generate HTML report
	htmlFile := strings.Replace(filename, ".json", ".html", 1)
	return bs.generateHTMLReport(result, htmlFile)
}

func (bs *BenchmarkSuite) generateHTMLReport(result *BenchmarkResult, filename string) error {
	// TODO: Implement HTML report generation with charts and diffs
	return nil
}

// testHealthEndpoint tests the /healthz endpoint of the deployed application
func (bs *BenchmarkSuite) testHealthEndpoint(ctx context.Context, healthURL string) error {
	client := &http.Client{Timeout: 30 * time.Second}
	
	req, err := http.NewRequestWithContext(ctx, "GET", healthURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create health check request: %w", err)
	}
	
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("health check request failed: %w", err)
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("health check failed with status %d: %s", resp.StatusCode, string(body))
	}
	
	return nil
}

// testApplicationEndpoints tests basic application functionality
func (bs *BenchmarkSuite) testApplicationEndpoints(ctx context.Context, appURL string) error {
	client := &http.Client{Timeout: 30 * time.Second}
	
	// Test root endpoint
	req, err := http.NewRequestWithContext(ctx, "GET", appURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create app test request: %w", err)
	}
	
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("app test request failed: %w", err)
	}
	defer resp.Body.Close()
	
	// Accept various success codes
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("app test failed with status %d: %s", resp.StatusCode, string(body))
	}
	
	return nil
}

// validatePerformance performs basic performance validation
func (bs *BenchmarkSuite) validatePerformance(ctx context.Context, appURL string) error {
	client := &http.Client{Timeout: 10 * time.Second}
	
	start := time.Now()
	req, err := http.NewRequestWithContext(ctx, "GET", appURL+"/healthz", nil)
	if err != nil {
		return fmt.Errorf("performance test request creation failed: %w", err)
	}
	
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("performance test failed: %w", err)
	}
	resp.Body.Close()
	
	responseTime := time.Since(start)
	
	// Warning if response time > 5 seconds
	if responseTime > 5*time.Second {
		return fmt.Errorf("slow response time: %v (warning only)", responseTime)
	}
	
	return nil
}

// parseDeploymentLogs analyzes deployment logs for common error patterns
func (bs *BenchmarkSuite) parseDeploymentLogs(logs string) []ErrorCapture {
	var errors []ErrorCapture
	
	lines := strings.Split(logs, "\n")
	for i, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		
		// Look for common error patterns
		if strings.Contains(strings.ToLower(line), "error") ||
		   strings.Contains(strings.ToLower(line), "failed") ||
		   strings.Contains(strings.ToLower(line), "exception") {
			
			errors = append(errors, ErrorCapture{
				Type:      "deployment",
				Message:   line,
				Details:   bs.getLogContext(lines, i),
				Timestamp: time.Now(),
			})
		}
	}
	
	return errors
}

// analyzeBuildErrors detects build system errors
func (bs *BenchmarkSuite) analyzeBuildErrors(repoPath string) []ErrorCapture {
	var errors []ErrorCapture
	
	buildSystem := bs.buildOps.DetectBuildSystem(repoPath)
	if buildSystem == "unknown" {
		errors = append(errors, ErrorCapture{
			Type:      "build_system",
			Message:   "Failed to detect build system",
			Details:   "No recognized build files found",
			Timestamp: time.Now(),
		})
		return errors
	}
	
	// Try to detect common build issues
	ctx := context.Background()
	if err := bs.buildOps.ValidateBuild(ctx, repoPath, buildSystem); err != nil {
		errors = append(errors, ErrorCapture{
			Type:      "build_validation",
			Message:   fmt.Sprintf("Build validation failed for %s", buildSystem),
			Details:   err.Error(),
			Timestamp: time.Now(),
		})
	}
	
	return errors
}

// detectConfigurationErrors analyzes configuration files for issues
func (bs *BenchmarkSuite) detectConfigurationErrors(repoPath string) []ErrorCapture {
	var errors []ErrorCapture
	
	// Check common configuration files
	configFiles := []string{
		"pom.xml", "build.gradle", "package.json", "go.mod", 
		"requirements.txt", "Dockerfile", "application.properties",
	}
	
	for _, configFile := range configFiles {
		configPath := filepath.Join(repoPath, configFile)
		if _, err := os.Stat(configPath); err == nil {
			// File exists, check for common issues
			if configErrors := bs.validateConfigFile(configPath, configFile); len(configErrors) > 0 {
				errors = append(errors, configErrors...)
			}
		}
	}
	
	return errors
}

// analyzeDependencies checks for dependency and compatibility issues
func (bs *BenchmarkSuite) analyzeDependencies(repoPath string) []ErrorCapture {
	var errors []ErrorCapture
	
	buildSystem := bs.buildOps.DetectBuildSystem(repoPath)
	if buildSystem == "unknown" {
		return errors // Skip if we can't detect build system
	}
	
	// Check for known dependency conflicts or version issues
	switch buildSystem {
	case "maven":
		if depErrors := bs.analyzeMavenDependencies(repoPath); len(depErrors) > 0 {
			errors = append(errors, depErrors...)
		}
	case "gradle":
		if depErrors := bs.analyzeGradleDependencies(repoPath); len(depErrors) > 0 {
			errors = append(errors, depErrors...)
		}
	case "npm", "yarn":
		if depErrors := bs.analyzeNodeDependencies(repoPath); len(depErrors) > 0 {
			errors = append(errors, depErrors...)
		}
	}
	
	return errors
}

// Helper methods for error analysis
func (bs *BenchmarkSuite) getLogContext(lines []string, errorIndex int) string {
	start := errorIndex - 2
	if start < 0 {
		start = 0
	}
	end := errorIndex + 3
	if end >= len(lines) {
		end = len(lines)
	}
	
	context := lines[start:end]
	return strings.Join(context, "\n")
}

func (bs *BenchmarkSuite) validateConfigFile(configPath, configType string) []ErrorCapture {
	var errors []ErrorCapture
	
	// Basic file readability check
	content, err := os.ReadFile(configPath)
	if err != nil {
		errors = append(errors, ErrorCapture{
			Type:      "config_read",
			Message:   fmt.Sprintf("Cannot read %s", configType),
			Details:   err.Error(),
			Timestamp: time.Now(),
		})
		return errors
	}
	
	// Check for common syntax issues (very basic)
	contentStr := string(content)
	if strings.Contains(configType, "xml") && !strings.Contains(contentStr, "<?xml") {
		errors = append(errors, ErrorCapture{
			Type:      "config_syntax",
			Message:   fmt.Sprintf("Invalid XML format in %s", configType),
			Details:   "Missing XML declaration",
			Timestamp: time.Now(),
		})
	}
	
	return errors
}

func (bs *BenchmarkSuite) analyzeMavenDependencies(repoPath string) []ErrorCapture {
	var errors []ErrorCapture
	
	pomPath := filepath.Join(repoPath, "pom.xml")
	if _, err := os.Stat(pomPath); err != nil {
		return errors
	}
	
	// TODO: Parse pom.xml and check for known version conflicts
	// For now, just check if file is readable
	if _, err := os.ReadFile(pomPath); err != nil {
		errors = append(errors, ErrorCapture{
			Type:      "maven_config",
			Message:   "Cannot read pom.xml",
			Details:   err.Error(),
			Timestamp: time.Now(),
		})
	}
	
	return errors
}

func (bs *BenchmarkSuite) analyzeGradleDependencies(repoPath string) []ErrorCapture {
	var errors []ErrorCapture
	
	gradlePath := filepath.Join(repoPath, "build.gradle")
	if _, err := os.Stat(gradlePath); err != nil {
		return errors
	}
	
	// TODO: Parse build.gradle and check for known issues
	if _, err := os.ReadFile(gradlePath); err != nil {
		errors = append(errors, ErrorCapture{
			Type:      "gradle_config",
			Message:   "Cannot read build.gradle",
			Details:   err.Error(),
			Timestamp: time.Now(),
		})
	}
	
	return errors
}

func (bs *BenchmarkSuite) analyzeNodeDependencies(repoPath string) []ErrorCapture {
	var errors []ErrorCapture
	
	packagePath := filepath.Join(repoPath, "package.json")
	if _, err := os.Stat(packagePath); err != nil {
		return errors
	}
	
	// TODO: Parse package.json and check for known security issues or conflicts
	if _, err := os.ReadFile(packagePath); err != nil {
		errors = append(errors, ErrorCapture{
			Type:      "npm_config",
			Message:   "Cannot read package.json",
			Details:   err.Error(),
			Timestamp: time.Now(),
		})
	}
	
	return errors
}

// CompareBenchmarks compares multiple benchmark results
func CompareBenchmarks(results []*BenchmarkResult) *ComparisonResult {
	if len(results) < 2 {
		return nil
	}
	
	comparison := &ComparisonResult{
		BaselineRun:  results[0].Config.Name,
		ComparedRuns: []string{},
		Metrics:      make(map[string]interface{}),
	}
	
	for i := 1; i < len(results); i++ {
		comparison.ComparedRuns = append(comparison.ComparedRuns, results[i].Config.Name)
	}
	
	// Compare key metrics
	comparison.Metrics["success_rate"] = compareSuccessRates(results)
	comparison.Metrics["average_time"] = compareAverageTimes(results)
	comparison.Metrics["llm_cost"] = compareLLMCosts(results)
	comparison.Metrics["total_iterations"] = compareTotalIterations(results)
	
	// Determine winner based on success rate and time
	comparison.Winner = determineWinner(results)
	comparison.Analysis = generateAnalysis(results)
	
	return comparison
}

// Helper comparison functions
func compareSuccessRates(results []*BenchmarkResult) map[string]float64 {
	rates := make(map[string]float64)
	for _, r := range results {
		if r.Summary.TotalIterations > 0 {
			rate := float64(r.Summary.SuccessfulIterations) / float64(r.Summary.TotalIterations)
			rates[r.Config.Name] = rate * 100
		}
	}
	return rates
}

func compareAverageTimes(results []*BenchmarkResult) map[string]string {
	times := make(map[string]string)
	for _, r := range results {
		times[r.Config.Name] = r.Summary.AverageIterationTime.String()
	}
	return times
}

func compareLLMCosts(results []*BenchmarkResult) map[string]float64 {
	costs := make(map[string]float64)
	for _, r := range results {
		costs[r.Config.Name] = r.Summary.TotalLLMCost
	}
	return costs
}

func compareTotalIterations(results []*BenchmarkResult) map[string]int {
	iterations := make(map[string]int)
	for _, r := range results {
		iterations[r.Config.Name] = r.Summary.TotalIterations
	}
	return iterations
}

func determineWinner(results []*BenchmarkResult) string {
	var bestName string
	var bestScore float64
	
	for _, r := range results {
		// Score based on success rate (70%) and speed (30%)
		successRate := float64(r.Summary.SuccessfulIterations) / float64(r.Summary.TotalIterations)
		speedScore := 1.0 / r.Summary.AverageIterationTime.Seconds()
		
		score := (successRate * 0.7) + (speedScore * 0.3)
		
		if score > bestScore {
			bestScore = score
			bestName = r.Config.Name
		}
	}
	
	return bestName
}

func generateAnalysis(results []*BenchmarkResult) string {
	// TODO: Generate detailed analysis text
	return "Comparative analysis of benchmark runs"
}