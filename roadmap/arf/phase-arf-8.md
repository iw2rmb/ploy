# Phase ARF-8: Benchmark Test Suite & Multi-LLM Provider Support

**Duration**: 2-3 months for comprehensive benchmark system
**Prerequisites**: Phases ARF-1 through ARF-3 completed (core engine and LLM integration)
**Dependencies**: Multiple LLM provider APIs, benchmark infrastructure, reporting systems

## Overview

Phase ARF-8 introduces a comprehensive benchmark test suite for ARF, enabling systematic testing and evaluation of code transformations across different repositories, recipes, and LLM providers. This phase expands LLM support beyond OpenAI to include Ollama (for local models), Anthropic, Azure OpenAI, and other providers, while implementing detailed iteration tracking with diffs, stage-wise time measurements, and comprehensive reporting.

The benchmark suite addresses the critical need for measuring ARF's effectiveness, comparing different LLM providers, and providing detailed insights into the transformation process including every iteration of self-healing attempts.

## Problem Statement

Current limitations in ARF testing and evaluation:
- **Single LLM Provider**: Only OpenAI is currently supported, limiting flexibility and cost optimization
- **Limited Iteration Visibility**: No comprehensive tracking of self-healing iterations with diffs
- **Insufficient Metrics**: Missing stage-wise timing and detailed error analysis
- **No Benchmark Framework**: Cannot systematically test transformations across repositories
- **Lack of Comparative Analysis**: Cannot compare effectiveness of different LLM providers

Manual testing requires:
- Setting up individual test scenarios
- Manually tracking transformation attempts
- Collecting metrics across multiple runs
- Comparing results without standardization
- Estimating costs without systematic tracking

## Technical Architecture

### Core Components

#### 1. Benchmark Test Suite Engine
- **Repository Test Execution**: Run specific recipes on target repositories
- **Iteration Tracking**: Capture every self-healing attempt with full context
- **Stage Timing**: Measure time for each transformation stage
- **Error Collection**: Comprehensive error tracking and categorization
- **Resource Monitoring**: Track memory, CPU, and API usage

#### 2. Multi-LLM Provider System
- **Provider Interface**: Abstract interface for all LLM providers
- **Ollama Integration**: Support for local models via Ollama
- **Provider Factory**: Dynamic provider selection and configuration
- **Cost Tracking**: Token usage and cost estimation per provider
- **Streaming Support**: Handle streaming responses from compatible providers
- **Auto Model Management**: Automatic download and installation of required models

#### 3. Iteration & Diff Tracking
- **Comprehensive Diffs**: Capture code changes for each iteration
- **State Management**: Track transformation state between iterations
- **Rollback Capability**: Restore to any iteration point
- **Change Analysis**: Categorize and analyze changes per iteration

#### 4. Performance Profiling
- **Stage Breakdown**: Time measurement for each transformation stage
- **Memory Profiling**: Track memory usage throughout execution
- **API Latency**: Measure LLM API response times
- **Bottleneck Analysis**: Identify performance bottlenecks

#### 5. Report Generation System
- **Multiple Formats**: JSON, HTML, Markdown, PDF reports
- **Comparative Analysis**: Side-by-side provider comparisons
- **Visual Dashboards**: Charts and graphs for metrics
- **Executive Summaries**: High-level insights and recommendations

## Implementation Tasks

### Phase 8A: Core Benchmark Infrastructure (Month 1)

#### 1. Benchmark Suite Engine

```go
// api/arf/benchmark_suite.go
package arf

import (
    "context"
    "time"
)

// BenchmarkSuite provides comprehensive testing capabilities for ARF
type BenchmarkSuite interface {
    RunBenchmark(ctx context.Context, config BenchmarkConfig) (*BenchmarkReport, error)
    RunRepositoryTest(ctx context.Context, test RepositoryTest) (*TestResult, error)
    CompareProviders(ctx context.Context, comparison ProviderComparison) (*ComparisonReport, error)
    RunTestSuite(ctx context.Context, suite TestSuite) (*SuiteReport, error)
}

// BenchmarkConfig defines a benchmark test configuration
type BenchmarkConfig struct {
    TestID          string                 `json:"test_id"`
    Repository      Repository             `json:"repository"`
    Recipe          Recipe                 `json:"recipe"`
    Transformation  TransformationSpec     `json:"transformation"`
    LLMConfig       LLMConfiguration       `json:"llm_config"`
    Tracking        TrackingConfig         `json:"tracking"`
    Limits          BenchmarkLimits        `json:"limits"`
    OutputConfig    OutputConfiguration    `json:"output_config"`
}

// TrackingConfig specifies what to track during benchmarks
type TrackingConfig struct {
    TrackIterations     bool `json:"track_iterations"`
    CollectDiffs        bool `json:"collect_diffs"`
    TimeByStage         bool `json:"time_by_stage"`
    TrackMemory         bool `json:"track_memory"`
    TrackAPIUsage       bool `json:"track_api_usage"`
    SaveIntermediates   bool `json:"save_intermediates"`
}

// IterationMetrics captures metrics for a single iteration
type IterationMetrics struct {
    IterationNumber int                    `json:"iteration_number"`
    StartTime       time.Time              `json:"start_time"`
    EndTime         time.Time              `json:"end_time"`
    Duration        time.Duration          `json:"duration"`
    Stages          []StageMetrics         `json:"stages"`
    Diff            string                 `json:"diff"`
    Changes         []CodeChange           `json:"changes"`
    Errors          []TransformationError  `json:"errors"`
    LLMUsage        LLMUsageMetrics        `json:"llm_usage"`
    Success         bool                   `json:"success"`
    StateSnapshot   string                 `json:"state_snapshot"`
}

// StageMetrics captures metrics for a transformation stage
type StageMetrics struct {
    Name            string        `json:"name"`
    Type            StageType     `json:"type"`
    StartTime       time.Time     `json:"start_time"`
    Duration        time.Duration `json:"duration"`
    MemoryUsed      int64         `json:"memory_used_bytes"`
    MemoryDelta     int64         `json:"memory_delta_bytes"`
    CPUPercent      float64       `json:"cpu_percent"`
    Success         bool          `json:"success"`
    Error           string        `json:"error,omitempty"`
    Metadata        map[string]interface{} `json:"metadata"`
}

// TestResult contains results from a single repository test
type TestResult struct {
    TestID              string              `json:"test_id"`
    Repository          Repository          `json:"repository"`
    Recipe              Recipe              `json:"recipe"`
    StartTime           time.Time           `json:"start_time"`
    EndTime             time.Time           `json:"end_time"`
    TotalDuration       time.Duration       `json:"total_duration"`
    Iterations          []IterationMetrics  `json:"iterations"`
    FinalState          TransformationState `json:"final_state"`
    Success             bool                `json:"success"`
    ErrorSummary        ErrorSummary        `json:"error_summary"`
    PerformanceSummary  PerformanceSummary  `json:"performance_summary"`
}
```

#### 2. Self-Healing Iteration Tracker

```go
// api/arf/iteration_tracker.go
type IterationTracker interface {
    StartIteration(ctx context.Context, iteration int) (*IterationContext, error)
    RecordStage(ctx context.Context, stage StageMetrics) error
    RecordDiff(ctx context.Context, diff string) error
    RecordError(ctx context.Context, err TransformationError) error
    CompleteIteration(ctx context.Context, success bool) (*IterationMetrics, error)
    GetIterationHistory(ctx context.Context) ([]IterationMetrics, error)
}

type SelfHealingContext struct {
    CurrentIteration    int                 `json:"current_iteration"`
    MaxIterations       int                 `json:"max_iterations"`
    Iterations          []IterationMetrics  `json:"iterations"`
    CurrentState        TransformationState `json:"current_state"`
    AccumulatedChanges  []CodeChange        `json:"accumulated_changes"`
    LearningContext     map[string]interface{} `json:"learning_context"`
}
```

### Phase 8B: Multi-LLM Provider Support (Month 2)

#### 1. LLM Provider Interface

```go
// api/arf/llm_provider.go
type LLMProvider interface {
    GenerateRecipe(ctx context.Context, request RecipeGenerationRequest) (*GeneratedRecipe, error)
    ValidateGenerated(ctx context.Context, recipe GeneratedRecipe) (*ValidationResult, error)
    OptimizeRecipe(ctx context.Context, recipe Recipe, feedback TransformationFeedback) (*Recipe, error)
    StreamGenerate(ctx context.Context, request StreamRequest) (<-chan StreamResponse, error)
    GetCapabilities() ProviderCapabilities
    EstimateCost(tokens TokenUsage) CostEstimate
}

type ProviderCapabilities struct {
    SupportsStreaming   bool     `json:"supports_streaming"`
    MaxContextLength    int      `json:"max_context_length"`
    SupportedLanguages  []string `json:"supported_languages"`
    SupportsFineTuning  bool     `json:"supports_fine_tuning"`
    LocalExecution      bool     `json:"local_execution"`
}

type TokenUsage struct {
    InputTokens     int `json:"input_tokens"`
    OutputTokens    int `json:"output_tokens"`
    TotalTokens     int `json:"total_tokens"`
}

type CostEstimate struct {
    Provider        string  `json:"provider"`
    Model           string  `json:"model"`
    InputCost       float64 `json:"input_cost"`
    OutputCost      float64 `json:"output_cost"`
    TotalCost       float64 `json:"total_cost"`
    Currency        string  `json:"currency"`
}
```

#### 2. Ollama Provider Implementation

```go
// api/arf/ollama_provider.go
type OllamaProvider struct {
    baseURL         string
    model           string
    temperature     float64
    httpClient      *http.Client
    streamingMode   bool
    contextLength   int
}

func NewOllamaProvider(config OllamaConfig) (*OllamaProvider, error) {
    return &OllamaProvider{
        baseURL:       config.BaseURL,
        model:         config.Model,
        temperature:   config.Temperature,
        httpClient:    &http.Client{Timeout: config.Timeout},
        streamingMode: config.StreamingEnabled,
        contextLength: config.ContextLength,
    }, nil
}

func (o *OllamaProvider) GenerateRecipe(ctx context.Context, request RecipeGenerationRequest) (*GeneratedRecipe, error) {
    // Prepare Ollama-specific request
    ollamaReq := OllamaRequest{
        Model:       o.model,
        Prompt:      o.buildPrompt(request),
        Temperature: o.temperature,
        Stream:      false,
        Options: OllamaOptions{
            NumCtx:     o.contextLength,
            NumPredict: 2048,
        },
    }
    
    // Call Ollama API
    response, err := o.callOllama(ctx, ollamaReq)
    if err != nil {
        return nil, fmt.Errorf("Ollama API call failed: %w", err)
    }
    
    // Parse response and create recipe
    return o.parseResponse(response, request), nil
}

func (o *OllamaProvider) GetCapabilities() ProviderCapabilities {
    return ProviderCapabilities{
        SupportsStreaming:  true,
        MaxContextLength:   o.contextLength,
        SupportedLanguages: []string{"java", "python", "go", "javascript", "rust"},
        SupportsFineTuning: false,
        LocalExecution:     true,
    }
}

func (o *OllamaProvider) EstimateCost(tokens TokenUsage) CostEstimate {
    // Ollama is free for local execution
    return CostEstimate{
        Provider:   "ollama",
        Model:      o.model,
        InputCost:  0,
        OutputCost: 0,
        TotalCost:  0,
        Currency:   "USD",
    }
}
```

#### 3. Provider Factory

```go
// api/arf/llm_provider_factory.go
type LLMProviderFactory struct {
    configs map[string]ProviderConfig
}

func (f *LLMProviderFactory) CreateProvider(providerType string, config map[string]interface{}) (LLMProvider, error) {
    switch providerType {
    case "openai":
        return NewOpenAIProvider(config)
    case "ollama":
        return NewOllamaProvider(config)
    case "anthropic":
        return NewAnthropicProvider(config)
    case "azure":
        return NewAzureOpenAIProvider(config)
    case "cohere":
        return NewCohereProvider(config)
    default:
        return nil, fmt.Errorf("unsupported provider: %s", providerType)
    }
}
```

#### 4. Auto Model Management

```go
// api/arf/model_manager.go
type ModelManager interface {
    EnsureModelAvailable(ctx context.Context, provider, model string) error
    DownloadModel(ctx context.Context, provider, model string) error
    ListAvailableModels(ctx context.Context, provider string) ([]ModelInfo, error)
    GetModelStatus(ctx context.Context, provider, model string) ModelStatus
    GetModelSize(provider, model string) (int64, error)
    PurgeUnusedModels(ctx context.Context, provider string, keepCount int) error
}

type ModelInfo struct {
    Name         string            `json:"name"`
    Provider     string            `json:"provider"`
    Size         int64             `json:"size_bytes"`
    Downloaded   bool              `json:"downloaded"`
    LastUsed     time.Time         `json:"last_used"`
    Capabilities map[string]string `json:"capabilities"`
    Tags         []string          `json:"tags"`
}

type ModelStatus struct {
    Available     bool      `json:"available"`
    Downloading   bool      `json:"downloading"`
    Progress      float64   `json:"progress"`
    Downloaded    bool      `json:"downloaded"`
    LastChecked   time.Time `json:"last_checked"`
    Error         string    `json:"error,omitempty"`
}

type OllamaModelManager struct {
    client     *http.Client
    baseURL    string
    timeout    time.Duration
}

func (m *OllamaModelManager) EnsureModelAvailable(ctx context.Context, provider, model string) error {
    if provider != "ollama" {
        return fmt.Errorf("unsupported provider for auto-download: %s", provider)
    }
    
    // Check if model is already available
    status := m.GetModelStatus(ctx, provider, model)
    if status.Available {
        return nil
    }
    
    // Auto-download if not available
    log.Printf("Model %s not found, downloading automatically...", model)
    return m.DownloadModel(ctx, provider, model)
}

func (m *OllamaModelManager) DownloadModel(ctx context.Context, provider, model string) error {
    req := map[string]string{
        "name": model,
    }
    
    data, err := json.Marshal(req)
    if err != nil {
        return fmt.Errorf("failed to marshal request: %w", err)
    }
    
    httpReq, err := http.NewRequestWithContext(ctx, "POST", m.baseURL+"/api/pull", bytes.NewBuffer(data))
    if err != nil {
        return fmt.Errorf("failed to create request: %w", err)
    }
    httpReq.Header.Set("Content-Type", "application/json")
    
    resp, err := m.client.Do(httpReq)
    if err != nil {
        return fmt.Errorf("failed to download model: %w", err)
    }
    defer resp.Body.Close()
    
    if resp.StatusCode != http.StatusOK {
        return fmt.Errorf("model download failed with status: %d", resp.StatusCode)
    }
    
    // Stream progress (optional)
    scanner := bufio.NewScanner(resp.Body)
    for scanner.Scan() {
        var progress map[string]interface{}
        if err := json.Unmarshal(scanner.Bytes(), &progress); err == nil {
            if status, ok := progress["status"].(string); ok && status == "success" {
                log.Printf("Model %s downloaded successfully", model)
                return nil
            }
        }
    }
    
    return scanner.Err()
}
```

**Auto Model Management Features:**

1. **Automatic Model Detection**: Automatically detect when required models are missing
2. **Smart Downloading**: Download models on-demand when first needed for benchmarks
3. **Progress Tracking**: Show download progress for large models
4. **Model Caching**: Cache downloaded models and reuse across benchmark runs
5. **Storage Management**: Automatic cleanup of unused models to save disk space
6. **Provider-Agnostic**: Support auto-download for different LLM providers
7. **Error Recovery**: Graceful handling of failed downloads with retry logic
8. **Size Estimation**: Show estimated download size before starting
9. **Background Downloads**: Non-blocking downloads with status monitoring
10. **Model Validation**: Verify model integrity after download

**Integration with Benchmark Suite:**

```go
// Automatic model management in benchmark execution
func (s *BenchmarkSuite) runIteration(ctx context.Context, config *BenchmarkConfig, iteration int) (*IterationResult, error) {
    // Ensure required model is available before starting
    if err := s.modelManager.EnsureModelAvailable(ctx, config.LLMProvider, config.LLMModel); err != nil {
        return nil, fmt.Errorf("failed to ensure model availability: %w", err)
    }
    
    // Continue with benchmark execution
    return s.executeTransformation(ctx, config, iteration)
}
```

### Phase 8C: Comprehensive Reporting (Month 3)

#### 1. Benchmark Report Generator

```go
// api/arf/benchmark_report.go
type BenchmarkReportGenerator interface {
    GenerateReport(ctx context.Context, result TestResult, format ReportFormat) (*Report, error)
    GenerateComparison(ctx context.Context, results []TestResult, format ReportFormat) (*ComparisonReport, error)
    GenerateSummary(ctx context.Context, suite SuiteReport) (*ExecutiveSummary, error)
}

type BenchmarkReport struct {
    TestID              string                  `json:"test_id"`
    ExecutionDate       time.Time               `json:"execution_date"`
    Repository          RepositoryInfo          `json:"repository"`
    Recipe              RecipeInfo              `json:"recipe"`
    LLMProvider         LLMProviderInfo         `json:"llm_provider"`
    ExecutionSummary    ExecutionSummary        `json:"execution_summary"`
    Iterations          []IterationReport       `json:"iterations"`
    StageAnalysis       StageAnalysis           `json:"stage_analysis"`
    ErrorAnalysis       ErrorAnalysis           `json:"error_analysis"`
    DiffAnalysis        DiffAnalysis            `json:"diff_analysis"`
    PerformanceMetrics  PerformanceMetrics      `json:"performance_metrics"`
    CostAnalysis        CostAnalysis            `json:"cost_analysis"`
    Recommendations     []Recommendation        `json:"recommendations"`
}

type IterationReport struct {
    Number              int                     `json:"number"`
    Duration            time.Duration           `json:"duration"`
    StageBreakdown      map[string]time.Duration `json:"stage_breakdown"`
    DiffSummary         DiffSummary             `json:"diff_summary"`
    FullDiff            string                  `json:"full_diff,omitempty"`
    ErrorsEncountered   []ErrorDetail           `json:"errors_encountered"`
    Success             bool                    `json:"success"`
    ImprovementFromPrev float64                 `json:"improvement_from_previous"`
}

type DiffAnalysis struct {
    TotalLinesAdded     int                     `json:"total_lines_added"`
    TotalLinesRemoved   int                     `json:"total_lines_removed"`
    FilesModified       int                     `json:"files_modified"`
    ChangesByCategory   map[string]int          `json:"changes_by_category"`
    SignificantChanges  []SignificantChange     `json:"significant_changes"`
    DiffEvolution       []DiffEvolution         `json:"diff_evolution"`
}

type PerformanceMetrics struct {
    TotalDuration           time.Duration           `json:"total_duration"`
    AverageIterationTime    time.Duration           `json:"average_iteration_time"`
    FastestIteration        int                     `json:"fastest_iteration"`
    SlowestIteration        int                     `json:"slowest_iteration"`
    StageTimings            map[string]StageStats   `json:"stage_timings"`
    MemoryPeakUsage         int64                   `json:"memory_peak_usage_bytes"`
    AverageMemoryUsage      int64                   `json:"average_memory_usage_bytes"`
    CPUUtilization          float64                 `json:"cpu_utilization_percent"`
}
```

#### 2. HTML Report Template

```go
// api/arf/report_templates.go
const htmlReportTemplate = `
<!DOCTYPE html>
<html>
<head>
    <title>ARF Benchmark Report - {{.TestID}}</title>
    <style>
        /* Professional styling for reports */
        .iteration-timeline { /* Visual timeline of iterations */ }
        .diff-viewer { /* Side-by-side diff display */ }
        .performance-chart { /* Performance metrics charts */ }
        .cost-breakdown { /* Cost analysis visualization */ }
    </style>
</head>
<body>
    <h1>ARF Benchmark Report</h1>
    <div class="summary">
        <h2>Executive Summary</h2>
        <p>Repository: {{.Repository.URL}}</p>
        <p>Recipe: {{.Recipe.Name}}</p>
        <p>LLM Provider: {{.LLMProvider.Name}} ({{.LLMProvider.Model}})</p>
        <p>Total Duration: {{.ExecutionSummary.TotalDuration}}</p>
        <p>Success: {{.ExecutionSummary.Success}}</p>
    </div>
    
    <div class="iterations">
        <h2>Iteration Timeline</h2>
        {{range .Iterations}}
        <div class="iteration">
            <h3>Iteration {{.Number}}</h3>
            <div class="diff-viewer">{{.FullDiff}}</div>
            <div class="metrics">{{.StageBreakdown}}</div>
        </div>
        {{end}}
    </div>
    
    <div class="performance">
        <h2>Performance Analysis</h2>
        <canvas id="performanceChart"></canvas>
    </div>
</body>
</html>
`
```

## Configuration

### Benchmark Configuration

```yaml
# configs/arf-benchmark.yaml
benchmark:
  # LLM Provider Configurations
  providers:
    ollama:
      base_url: "${OLLAMA_URL:-http://localhost:11434}"
      models:
        - name: codellama
          context_length: 16384
          temperature: 0.1
        - name: mistral
          context_length: 8192
          temperature: 0.2
        - name: llama2
          context_length: 4096
          temperature: 0.1
      timeout: 120s
      retry_attempts: 3
      
    openai:
      api_key: "${OPENAI_API_KEY}"
      organization: "${OPENAI_ORG}"
      models:
        - name: gpt-4
          max_tokens: 8192
          temperature: 0.1
        - name: gpt-3.5-turbo
          max_tokens: 4096
          temperature: 0.2
      timeout: 60s
      
    anthropic:
      api_key: "${ANTHROPIC_API_KEY}"
      models:
        - name: claude-3-opus
          max_tokens: 4096
        - name: claude-3-sonnet
          max_tokens: 4096
      timeout: 60s
      
  # Test Scenarios
  test_scenarios:
    java_11_to_17:
      name: "Java 11 to 17 Migration"
      description: "Migrate Java application from version 11 to 17"
      recipe: "org.openrewrite.java.migrate.Java11toJava17"
      max_iterations: 10
      timeout: 30m
      success_criteria:
        compilation_success: true
        tests_pass: true
        no_critical_warnings: true
        
    spring_boot_3:
      name: "Spring Boot 3.0 Migration"
      description: "Upgrade Spring Boot 2.x to 3.0"
      recipe: "org.openrewrite.java.spring.boot3.UpgradeSpringBoot_3_0"
      max_iterations: 15
      timeout: 45m
      
    security_fixes:
      name: "Security Vulnerability Remediation"
      description: "Fix known security vulnerabilities"
      recipe: "org.openrewrite.java.security.SecureTempFileCreation"
      max_iterations: 5
      timeout: 15m
      
  # Tracking Configuration
  tracking:
    collect_diffs: true
    diff_format: unified  # unified, split, or inline
    track_memory: true
    memory_sampling_interval: 100ms
    track_stages: true
    track_api_calls: true
    save_intermediates: true
    intermediate_format: json
    
  # Performance Thresholds
  performance:
    max_iteration_time: 5m
    max_total_time: 60m
    max_memory_usage: 4GB
    warn_on_slow_stages: true
    slow_stage_threshold: 30s
    
  # Reporting Configuration
  reporting:
    formats:
      - json
      - html
      - markdown
      - pdf
    include_full_diffs: true
    include_stage_timings: true
    include_memory_profiles: true
    include_cost_analysis: true
    chart_library: chartjs  # chartjs or d3
    
  # Cost Tracking
  cost_tracking:
    track_llm_costs: true
    track_compute_costs: true
    currency: USD
    alert_on_high_cost: true
    cost_threshold: 10.00
```

## API Endpoints

```yaml
# Benchmark Management
POST   /v1/arf/benchmark/run              # Run single benchmark
POST   /v1/arf/benchmark/suite            # Run benchmark suite
GET    /v1/arf/benchmark/status/{id}      # Get benchmark status
GET    /v1/arf/benchmark/results/{id}     # Get benchmark results
DELETE /v1/arf/benchmark/cancel/{id}      # Cancel running benchmark

# Provider Comparison
POST   /v1/arf/benchmark/compare          # Compare providers
GET    /v1/arf/benchmark/comparison/{id}  # Get comparison results

# Report Generation
GET    /v1/arf/benchmark/report/{id}      # Get benchmark report
POST   /v1/arf/benchmark/report/generate  # Generate custom report
GET    /v1/arf/benchmark/reports          # List all reports

# LLM Provider Management
GET    /v1/arf/providers                  # List available providers
GET    /v1/arf/providers/{name}/models    # List provider models
POST   /v1/arf/providers/test             # Test provider connectivity
```

## CLI Commands

```bash
# Run benchmark on repository
ploy arf benchmark run \
  --repo https://github.com/example/myapp \
  --recipe "org.openrewrite.java.migrate.Java11toJava17" \
  --provider ollama \
  --model codellama \
  --max-iterations 10 \
  --track-all \
  --output report.html

# Compare multiple providers
ploy arf benchmark compare \
  --repo ./local-java-app \
  --recipe spring-boot-3 \
  --providers "ollama:codellama,openai:gpt-4,anthropic:claude-3-opus" \
  --parallel \
  --output comparison.html

# Run predefined test suite
ploy arf benchmark suite \
  --config benchmark-suite.yaml \
  --repos repos.txt \
  --parallel 4 \
  --output-dir results/

# Analyze existing results
ploy arf benchmark analyze \
  --results results/*.json \
  --generate-summary \
  --format markdown

# Test provider connectivity
ploy arf providers test \
  --provider ollama \
  --model codellama \
  --verbose
```

## Success Metrics

- **LLM Provider Support**: 5+ providers (OpenAI, Ollama, Anthropic, Azure, Cohere)
- **Iteration Tracking**: 100% capture of all self-healing iterations
- **Diff Collection**: Complete diff for every iteration
- **Stage Timing**: Microsecond precision for stage measurements
- **Memory Tracking**: <5% overhead for memory profiling
- **Report Generation**: <10 seconds for HTML report generation
- **Cost Accuracy**: ±1% accuracy for LLM cost estimation
- **Parallel Execution**: Support for 10+ parallel benchmarks

## Testing Strategy

### Unit Tests
- Provider interface implementations
- Iteration tracking accuracy
- Diff generation and parsing
- Stage timing precision
- Cost calculation correctness

### Integration Tests
- End-to-end benchmark execution
- Multi-provider comparison
- Report generation validation
- API endpoint functionality
- CLI command execution

### Performance Tests
- Benchmark overhead measurement
- Memory profiling accuracy
- Parallel execution scaling
- Report generation speed
- Large diff handling

## Risk Mitigation

### Technical Risks
- **Provider API Changes**: Version pinning and compatibility checks
- **Memory Overhead**: Configurable tracking granularity
- **Large Diffs**: Streaming and pagination for large changes
- **Provider Failures**: Retry logic and fallback providers

### Operational Risks
- **Cost Overruns**: Budget limits and alerts
- **Long-Running Tests**: Timeouts and checkpointing
- **Storage Requirements**: Configurable retention policies
- **Network Dependencies**: Offline mode for local providers

## Future Enhancements

- Machine learning for optimal provider selection
- Automated benchmark suite generation
- Performance regression detection
- Cost optimization recommendations
- Integration with CI/CD pipelines
- Real-time benchmark monitoring dashboard
- Distributed benchmark execution
- Custom metric definitions
- Benchmark result sharing platform

## Dependencies

### External Services
- Ollama server for local models
- OpenAI API access
- Anthropic API access
- Azure OpenAI subscription
- Report generation libraries

### Infrastructure
- Sufficient memory for tracking
- Storage for intermediate results
- Network connectivity for API calls
- Git for diff generation

Phase ARF-8 provides the comprehensive benchmark testing infrastructure needed to systematically evaluate ARF's effectiveness, compare different approaches, and optimize transformation strategies across various scenarios and LLM providers.