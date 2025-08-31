# Phase 4: ARF Module Consolidation

## Objective

Consolidate all duplicate ARF (Automated Refactoring Framework) implementations, establish clear module boundaries, and create a single, well-architected ARF system with proper dependency injection and clear separation of concerns.

## Current State Analysis

### Major Duplication Issues

1. **Duplicate ARF Directories**:
   - `api/arf/` - 50+ files
   - References to `controller/arf/` throughout codebase
   - Identical implementations in both locations

2. **Duplicate Core Components**:
   ```
   Duplicated in both api/arf/ and controller/arf/:
   - complexity_analyzer.go (identical 505 lines)
   - pattern_learning.go (identical 400+ lines)
   - ab_testing.go (identical structures)
   - multi_language.go (identical CodeChange struct)
   - hybrid_pipeline.go (identical implementations)
   - factory.go (duplicate Phase3Config)
   - engine.go (duplicate Codebase struct)
   - monitoring.go (duplicate Alert struct)
   ```

3. **Scattered ARF Logic**:
   - OpenRewrite integration in multiple places
   - LLM dispatchers duplicated
   - Recipe execution spread across files
   - Storage adapters reimplemented

## Proposed Architecture

```
internal/arf/
├── README.md                          # ARF documentation
├── core/
│   ├── engine.go                     # Core ARF engine
│   ├── types.go                      # Shared types and interfaces
│   ├── context.go                    # ARF execution context
│   └── errors.go                     # ARF-specific errors
├── analysis/
│   ├── complexity.go                 # Code complexity analyzer
│   ├── patterns.go                   # Pattern detection
│   ├── metrics.go                    # Code metrics
│   └── multi_language.go             # Multi-language support
├── learning/
│   ├── patterns.go                   # Pattern learning service
│   ├── database.go                   # Learning database
│   ├── similarity.go                 # Similarity calculations
│   └── recommendations.go            # Fix recommendations
├── transformation/
│   ├── pipeline.go                   # Transformation pipeline
│   ├── strategies.go                 # Transformation strategies
│   ├── validator.go                  # Change validation
│   └── rollback.go                   # Rollback mechanisms
├── recipes/
│   ├── registry.go                   # Recipe registry
│   ├── executor.go                   # Recipe execution
│   ├── evolution.go                  # Recipe evolution
│   ├── catalog.go                    # Recipe catalog
│   └── models/                       # Recipe data models
├── integration/
│   ├── openrewrite/
│   │   ├── client.go                # OpenRewrite client
│   │   ├── dispatcher.go            # Job dispatcher
│   │   └── remediator.go            # Error remediation
│   ├── llm/
│   │   ├── dispatcher.go            # LLM dispatcher
│   │   ├── providers.go             # LLM providers
│   │   └── prompts.go               # Prompt templates
│   └── git/
│       ├── operations.go            # Git operations
│       └── validation.go            # Git validation
├── testing/
│   ├── framework.go                  # A/B testing framework
│   ├── experiments.go                # Experiment management
│   ├── analysis.go                   # Statistical analysis
│   └── sandbox.go                    # Testing sandbox
├── monitoring/
│   ├── metrics.go                    # ARF metrics
│   ├── alerts.go                     # Alert management
│   ├── dashboard.go                  # Monitoring dashboard
│   └── production.go                 # Production optimizer
├── storage/
│   ├── adapter.go                    # Storage adapter interface
│   ├── seaweedfs.go                  # SeaweedFS implementation
│   ├── consul.go                     # Consul index
│   └── cache.go                      # Caching layer
└── api/
    ├── handlers.go                    # HTTP handlers
    ├── models.go                      # API models
    └── validation.go                  # Request validation
```

## Core Interfaces

```go
// internal/arf/core/types.go
package core

import (
    "context"
    "time"
)

// Engine is the main ARF engine interface
type Engine interface {
    // Analysis
    Analyze(ctx context.Context, codebase Codebase) (*AnalysisResult, error)
    
    // Transformation
    Transform(ctx context.Context, req TransformRequest) (*TransformResult, error)
    
    // Learning
    Learn(ctx context.Context, outcome LearningOutcome) error
    FindSimilar(ctx context.Context, error ErrorContext) ([]Pattern, error)
    
    // Recipes
    ExecuteRecipe(ctx context.Context, recipe Recipe, target Codebase) (*ExecutionResult, error)
    EvolveRecipe(ctx context.Context, recipe Recipe, feedback Feedback) (*Recipe, error)
}

// Codebase represents a code repository
type Codebase struct {
    Repository  string            `json:"repository"`
    Branch      string            `json:"branch"`
    Path        string            `json:"path"`
    Language    string            `json:"language"`
    BuildTool   string            `json:"build_tool"`
    Metadata    map[string]string `json:"metadata"`
}

// AnalysisResult contains code analysis findings
type AnalysisResult struct {
    ID          string                   `json:"id"`
    Timestamp   time.Time                `json:"timestamp"`
    Complexity  *ComplexityMetrics       `json:"complexity"`
    Patterns    []PatternMatch           `json:"patterns"`
    Issues      []Issue                  `json:"issues"`
    Suggestions []Suggestion             `json:"suggestions"`
    Metadata    map[string]interface{}   `json:"metadata"`
}

// TransformRequest defines a transformation operation
type TransformRequest struct {
    Codebase    Codebase                 `json:"codebase"`
    Type        TransformationType       `json:"type"`
    Strategy    string                   `json:"strategy"`
    Options     map[string]interface{}   `json:"options"`
    DryRun      bool                     `json:"dry_run"`
}

// Recipe defines a reusable transformation recipe
type Recipe struct {
    ID          string                   `json:"id"`
    Name        string                   `json:"name"`
    Version     string                   `json:"version"`
    Description string                   `json:"description"`
    Steps       []RecipeStep             `json:"steps"`
    Metadata    map[string]interface{}   `json:"metadata"`
}
```

## Dependency Injection Architecture

```go
// internal/arf/core/engine.go
package core

import (
    "github.com/ploy/internal/arf/analysis"
    "github.com/ploy/internal/arf/learning"
    "github.com/ploy/internal/arf/transformation"
    "github.com/ploy/internal/arf/recipes"
)

// EngineConfig configures the ARF engine
type EngineConfig struct {
    Storage        StorageAdapter
    LLMProvider    LLMProvider
    OpenRewrite    OpenRewriteClient
    GitOperations  GitOperations
    MetricsCollector MetricsCollector
}

// DefaultEngine implements the Engine interface
type DefaultEngine struct {
    config       EngineConfig
    analyzer     *analysis.Analyzer
    learner      *learning.Service
    transformer  *transformation.Pipeline
    recipeEngine *recipes.Engine
}

// NewEngine creates a new ARF engine with dependency injection
func NewEngine(cfg EngineConfig) (*DefaultEngine, error) {
    // Create components with injected dependencies
    analyzer := analysis.NewAnalyzer(
        analysis.WithComplexityAnalyzer(cfg.Storage),
        analysis.WithPatternMatcher(cfg.Storage),
    )
    
    learner := learning.NewService(
        learning.WithStorage(cfg.Storage),
        learning.WithLLM(cfg.LLMProvider),
    )
    
    transformer := transformation.NewPipeline(
        transformation.WithOpenRewrite(cfg.OpenRewrite),
        transformation.WithGit(cfg.GitOperations),
    )
    
    recipeEngine := recipes.NewEngine(
        recipes.WithRegistry(cfg.Storage),
        recipes.WithExecutor(transformer),
    )
    
    return &DefaultEngine{
        config:       cfg,
        analyzer:     analyzer,
        learner:      learner,
        transformer:  transformer,
        recipeEngine: recipeEngine,
    }, nil
}
```

## Migration Strategy

### Phase 4.1: Extract Core Components

1. **Create new internal/arf structure**:
```bash
mkdir -p internal/arf/{core,analysis,learning,transformation,recipes,integration,testing,monitoring,storage,api}
```

2. **Move and consolidate core types**:
```go
// internal/arf/core/types.go
// Merge types from:
// - api/arf/types.go
// - api/arf/engine.go
// - controller/arf/types.go (if exists)
```

### Phase 4.2: Consolidate Analysis Components

```go
// internal/arf/analysis/complexity.go
package analysis

import (
    "context"
    "github.com/ploy/internal/arf/core"
)

// ComplexityAnalyzer analyzes code complexity
type ComplexityAnalyzer struct {
    multiLangEngine MultiLanguageEngine
}

// Merge implementations from:
// - api/arf/complexity_analyzer.go
// - controller/arf/complexity_analyzer.go

func (ca *ComplexityAnalyzer) Analyze(ctx context.Context, code string, language string) (*core.ComplexityMetrics, error) {
    // Consolidated implementation
}
```

### Phase 4.3: Unify Learning System

```go
// internal/arf/learning/patterns.go
package learning

// Consolidate from:
// - api/arf/pattern_learning.go
// - api/arf/learning_system.go
// - controller/arf/pattern_learning.go

type PatternLearningService struct {
    storage     core.StorageAdapter
    llmProvider core.LLMProvider
    patterns    map[string]Pattern
    index       map[string][]string
}

func (pls *PatternLearningService) Learn(ctx context.Context, outcome core.LearningOutcome) error {
    // Unified learning implementation
}
```

### Phase 4.4: Consolidate Transformation Pipeline

```go
// internal/arf/transformation/pipeline.go
package transformation

// Merge from:
// - api/arf/hybrid_pipeline.go
// - api/arf/transformation_workflow.go
// - controller/arf/hybrid_pipeline.go

type Pipeline struct {
    stages      []Stage
    validators  []Validator
    rollback    RollbackManager
}

func (p *Pipeline) Execute(ctx context.Context, req core.TransformRequest) (*core.TransformResult, error) {
    // Consolidated pipeline execution
}
```

### Phase 4.5: Update API Handlers

```go
// internal/arf/api/handlers.go
package api

import (
    "github.com/ploy/internal/arf/core"
    "github.com/gin-gonic/gin"
)

type Handlers struct {
    engine core.Engine
}

func NewHandlers(engine core.Engine) *Handlers {
    return &Handlers{engine: engine}
}

func (h *Handlers) RegisterRoutes(r *gin.RouterGroup) {
    arf := r.Group("/arf")
    {
        arf.POST("/analyze", h.Analyze)
        arf.POST("/transform", h.Transform)
        arf.POST("/recipes/execute", h.ExecuteRecipe)
        arf.GET("/patterns/similar", h.FindSimilarPatterns)
    }
}
```

## Testing Strategy

```go
// internal/arf/core/engine_test.go
package core_test

import (
    "testing"
    "github.com/ploy/internal/arf/core"
    "github.com/ploy/internal/testing/mocks"
)

func TestEngine(t *testing.T) {
    // Create mocked dependencies
    storage := mocks.NewStorageClient()
    llm := mocks.NewLLMProvider()
    openrewrite := mocks.NewOpenRewriteClient()
    
    // Create engine with mocks
    engine, err := core.NewEngine(core.EngineConfig{
        Storage:     storage,
        LLMProvider: llm,
        OpenRewrite: openrewrite,
    })
    require.NoError(t, err)
    
    // Test analysis
    t.Run("analysis", func(t *testing.T) {
        result, err := engine.Analyze(ctx, testCodebase)
        assert.NoError(t, err)
        assert.NotNil(t, result.Complexity)
    })
}
```

## Backwards Compatibility

```go
// api/arf/compatibility.go
// Temporary compatibility layer during migration

package arf

import "github.com/ploy/internal/arf/core"

// Type aliases for backwards compatibility
type (
    Codebase              = core.Codebase
    AnalysisResult        = core.AnalysisResult
    TransformRequest      = core.TransformRequest
    Recipe                = core.Recipe
)

// Deprecated: Use internal/arf/core.NewEngine
func NewEngine(config Config) (*Engine, error) {
    return core.NewEngine(config.ToEngineConfig())
}
```

## Removal Plan

After consolidation:

1. **Remove duplicate files**:
```bash
# Remove old ARF implementations
rm -rf api/arf/complexity_analyzer.go
rm -rf api/arf/pattern_learning.go
rm -rf controller/arf/  # If entire directory is duplicate
```

2. **Update imports**:
```bash
# Script to update imports
find . -name "*.go" -exec sed -i '' \
    -e 's|"github.com/ploy/api/arf|"github.com/ploy/internal/arf/api|g' \
    -e 's|"github.com/ploy/controller/arf|"github.com/ploy/internal/arf/core|g' \
    {} +
```

## Validation Checklist

- [ ] All ARF functionality consolidated in internal/arf
- [ ] No duplicate implementations remain
- [ ] Clear module boundaries established
- [ ] Dependency injection implemented
- [ ] All tests passing
- [ ] API backwards compatibility maintained
- [ ] Documentation updated
- [ ] Performance benchmarks show improvement

## Risk Mitigation

1. **Gradual Migration**: Keep old code during transition
2. **Feature Flags**: Toggle between old and new implementations
3. **Extensive Testing**: Full regression test suite
4. **Monitoring**: Track ARF performance metrics
5. **Rollback Plan**: Git tags at each migration step

## Implementation Sequence

- Create new structure and core components
- Consolidate analysis and complexity modules
- Unify learning system
- Merge transformation pipelines
- Consolidate recipes and integrations
- Update API handlers and compatibility layer
- Testing, validation, and cleanup

## Expected Outcomes

### Before
- ARF files: 100+ across multiple directories
- Duplicate implementations: 20+ files
- ARF-related LOC: ~15,000

### After
- ARF files: ~50 in organized structure
- Duplicate implementations: 0
- ARF-related LOC: ~7,000 (53% reduction)
- Architecture: Clean, maintainable, testable
- Performance: 20% improvement in ARF operations