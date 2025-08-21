# Phase ARF-3: LLM Integration & Hybrid Intelligence

**Duration**: AI-enhanced transformation capabilities phase
**Prerequisites**: Phase ARF-2 completed with error recovery and orchestration
**Dependencies**: LLM API access, hybrid processing pipeline, continuous learning system

## Overview

Phase ARF-3 represents the revolutionary integration of Large Language Models with OpenRewrite's static analysis capabilities, creating a hybrid intelligence system that combines deterministic transformations with AI-driven adaptability. This phase enables dynamic recipe generation, context-aware transformations, and continuous learning from transformation outcomes.

## Technical Architecture

### Core Components
- **LLM Integration Engine**: Dynamic recipe generation from error contexts
- **Hybrid Transformation Pipeline**: OpenRewrite + LLM enhancement workflows
- **Continuous Learning System**: Pattern extraction and strategy optimization
- **Context-Aware Strategy Selection**: Intelligent transformation approach selection

### Integration Points
- **ARF-2 Error Recovery**: Enhanced error analysis using LLM capabilities
- **OpenRewrite Recipe System**: LLM-generated recipes validated against existing catalog
- **Sandbox Validation**: AI-generated transformations tested in secure environments
- **Performance Analytics**: Learning system feeds back into strategy selection

## Implementation Tasks

### 1. LLM-Assisted Recipe Creation with Multi-Language Support

**Objective**: Enable dynamic generation of transformation recipes for multiple languages using LLM analysis and language-specific AST tools.

**Tasks**:
- Integrate LLM API for dynamic recipe generation from error contexts
- Implement LLM prompt engineering for multi-language recipe creation
- Add LLM response parsing into valid recipe formats (OpenRewrite YAML, tree-sitter queries)
- Create LLM-generated recipe validation and testing system
- Implement fallback handling when LLM generation fails
- Extend support beyond Java to Node.js, Python, Go, and Rust
- Integrate tree-sitter for universal AST parsing across languages
- Add WASM-specific transformation capabilities for Lane G

**Deliverables**:
```go
// controller/arf/llm_integration.go
type LLMRecipeGenerator interface {
    GenerateRecipe(ctx context.Context, request RecipeGenerationRequest) (*GeneratedRecipe, error)
    ValidateGenerated(ctx context.Context, recipe GeneratedRecipe) (*ValidationResult, error)
    OptimizeRecipe(ctx context.Context, recipe Recipe, feedback TransformationFeedback) (*Recipe, error)
}

type RecipeGenerationRequest struct {
    ErrorContext     ErrorContext             `json:"error_context"`
    CodebaseContext  CodebaseContext          `json:"codebase_context"`
    SimilarPatterns  []TransformationPattern  `json:"similar_patterns"`
    Constraints      []RecipeConstraint       `json:"constraints"`
    TargetFramework  string                   `json:"target_framework"`
    Language         string                   `json:"language"`
    ASTParser        string                   `json:"ast_parser"`
}

type GeneratedRecipe struct {
    Recipe       Recipe            `json:"recipe"`
    Confidence   float64           `json:"confidence"`
    Explanation  string            `json:"explanation"`
    LLMMetadata  LLMGenerationData `json:"llm_metadata"`
    Validation   ValidationResult  `json:"validation"`
}

type LLMGenerationData struct {
    Model           string            `json:"model"`
    PromptTokens    int               `json:"prompt_tokens"`
    ResponseTokens  int               `json:"response_tokens"`
    Temperature     float64           `json:"temperature"`
    RequestTime     time.Time         `json:"request_time"`
    ProcessingTime  time.Duration     `json:"processing_time"`
}
```

**Acceptance Criteria**:
- LLM generates syntactically valid recipes for Java, Node.js, Python, Go, Rust
- Generated recipes pass validation in sandbox environments
- Fallback to static recipes when LLM generation fails
- Recipe generation completes within 30 seconds
- 70%+ success rate for LLM-generated recipes on first attempt
- Tree-sitter integration enables cross-language AST analysis
- WASM transformations support optimization and polyfill injection

### 2. Multi-Language Transformation Engine

**Objective**: Extend ARF capabilities beyond Java to support transformations across Ploy's supported languages.

**Tasks**:
- Integrate tree-sitter for universal AST parsing
- Create language-specific transformation recipes for Node.js, Python, Go
- Implement WASM-specific optimizations for Lane G
- Add cross-language dependency analysis
- Create polyglot transformation capabilities

**Deliverables**:
```go
// controller/arf/multi_language.go
type MultiLanguageEngine interface {
    ParseAST(ctx context.Context, code string, language string) (*UniversalAST, error)
    GenerateTransformation(ctx context.Context, ast *UniversalAST, recipe Recipe) (*Transformation, error)
    ValidateLanguageSupport(language string) (bool, error)
    GetLanguageCapabilities(language string) (*LanguageCapabilities, error)
}

type UniversalAST struct {
    Language    string              `json:"language"`
    Parser      string              `json:"parser"`
    RootNode    *ASTNode            `json:"root_node"`
    Symbols     []Symbol            `json:"symbols"`
    Imports     []Import            `json:"imports"`
    Metadata    map[string]interface{} `json:"metadata"`
}

type LanguageCapabilities struct {
    Language        string              `json:"language"`
    Parsers         []string            `json:"parsers"`
    Transformations []TransformationType `json:"transformations"`
    Frameworks      []string            `json:"frameworks"`
    LaneSupport     []string            `json:"lane_support"`
}

// Language-specific recipe types
type NodeJSRecipe struct {
    Recipe
    PackageUpdates  map[string]string   `json:"package_updates"`
    ESLintRules     map[string]interface{} `json:"eslint_rules"`
    TypeScript      bool                `json:"typescript"`
}

type PythonRecipe struct {
    Recipe
    PipUpdates      map[string]string   `json:"pip_updates"`
    PyUpgrade       string              `json:"pyupgrade_target"`
    BlackConfig     map[string]interface{} `json:"black_config"`
}

type GoRecipe struct {
    Recipe
    GoModUpdates    map[string]string   `json:"go_mod_updates"`
    GofmtOptions    []string            `json:"gofmt_options"`
    StaticCheck     []string            `json:"staticcheck_rules"`
}

type WASMRecipe struct {
    Recipe
    OptimizationLevel   int             `json:"optimization_level"`
    TargetFeatures      []string        `json:"target_features"`
    PolyfillsRequired   []string        `json:"polyfills_required"`
    MemoryConfiguration MemoryConfig    `json:"memory_config"`
}
```

**Acceptance Criteria**:
- Support for 5+ languages (Java, JavaScript, Python, Go, Rust)
- Tree-sitter parses 95% of real-world code successfully
- Language-specific recipes achieve 80%+ success rate
- WASM optimizations reduce module size by 20%+
- Cross-language dependency tracking prevents breaking changes

### 3. Hybrid Transformation Pipeline

**Objective**: Create sophisticated workflows that combine deterministic transformations with LLM enhancement for complex scenarios across multiple languages.

**Tasks**:
- Create hybrid execution workflow: OpenRewrite → LLM enhancement → validation
- Implement confidence scoring system (token confidence + build success + test coverage)
- Add intelligent strategy selection based on transformation complexity
- Create context-aware prompting with surrounding code and build logs
- Implement solution confidence ranking and selection

**Deliverables**:
```go
// controller/arf/hybrid_pipeline.go
type HybridPipeline interface {
    ExecuteHybridTransformation(ctx context.Context, request HybridRequest) (*HybridResult, error)
    SelectOptimalStrategy(ctx context.Context, analysis ComplexityAnalysis) (*TransformationStrategy, error)
    EnhanceWithLLM(ctx context.Context, baseResult TransformationResult) (*EnhancedResult, error)
}

type HybridRequest struct {
    Repository      Repository              `json:"repository"`
    PrimaryRecipe   Recipe                  `json:"primary_recipe"`
    Context         TransformationContext   `json:"context"`
    EnhancementMode EnhancementMode         `json:"enhancement_mode"`
    Confidence      ConfidenceThresholds    `json:"confidence"`
}

type TransformationStrategy struct {
    Primary     StrategyType    `json:"primary"`
    Enhancement StrategyType    `json:"enhancement"`
    Confidence  float64         `json:"confidence"`
    Reasoning   string          `json:"reasoning"`
    Fallbacks   []StrategyType  `json:"fallbacks"`
}

type EnhancementMode int
const (
    NoEnhancement EnhancementMode = iota
    PostProcessing
    ParallelValidation
    FullHybrid
)

type ConfidenceThresholds struct {
    MinOpenRewrite float64 `json:"min_openrewrite"`
    MinLLM         float64 `json:"min_llm"`
    MinHybrid      float64 `json:"min_hybrid"`
    RequiredBuild  float64 `json:"required_build"`
}
```

**Acceptance Criteria**:
- Hybrid pipeline achieves 85%+ success rate vs 70% for OpenRewrite alone
- Confidence scoring accurately predicts transformation success
- Strategy selection optimizes for both accuracy and resource efficiency
- Context-aware prompting improves LLM relevance by 40%
- Solution ranking correctly identifies best transformations 90% of the time

### 4. Continuous Learning System with Schema Design

**Objective**: Implement machine learning capabilities that extract patterns from transformation outcomes to improve future strategy selection and recipe generation.

**Tasks**:
- Add success pattern extraction from completed transformations
- Implement failure pattern analysis and cataloging
- Create recipe performance tracking by repository type and complexity
- Add pattern generalization for new recipe template creation
- Implement model retraining for strategy selection algorithms
- Design comprehensive learning database schema
- Implement A/B testing framework for recipe variations
- Create feedback loop for continuous improvement

**Deliverables**:
```go
// controller/arf/learning_system.go
type LearningSystem interface {
    RecordTransformationOutcome(ctx context.Context, outcome TransformationOutcome) error
    ExtractPatterns(ctx context.Context, timeWindow time.Duration) (*PatternAnalysis, error)
    UpdateStrategyWeights(ctx context.Context, patterns PatternAnalysis) error
    GenerateRecipeTemplate(ctx context.Context, pattern SuccessPattern) (*RecipeTemplate, error)
}

type TransformationOutcome struct {
    TransformationID  string                    `json:"transformation_id"`
    Repository        RepositoryMetadata        `json:"repository"`
    Strategy          TransformationStrategy    `json:"strategy"`
    Result            TransformationResult      `json:"result"`
    Metrics           PerformanceMetrics        `json:"metrics"`
    Context           EnvironmentContext        `json:"context"`
    Timestamp         time.Time                 `json:"timestamp"`
}

type PatternAnalysis struct {
    SuccessPatterns   []SuccessPattern   `json:"success_patterns"`
    FailurePatterns   []FailurePattern   `json:"failure_patterns"`
    StrategyEffectiveness map[string]float64 `json:"strategy_effectiveness"`
    RecommendedUpdates []StrategyUpdate   `json:"recommended_updates"`
    Confidence        float64            `json:"confidence"`
}

type SuccessPattern struct {
    Signature         string                 `json:"signature"`
    Frequency         int                    `json:"frequency"`
    SuccessRate       float64                `json:"success_rate"`
    OptimalStrategy   TransformationStrategy `json:"optimal_strategy"`
    ContextFactors    []string               `json:"context_factors"`
    Generalization    PatternGeneralization  `json:"generalization"`
}
```

**Learning Database Schema**:
```sql
-- Transformation outcomes table
CREATE TABLE transformation_outcomes (
    id UUID PRIMARY KEY,
    transformation_id UUID NOT NULL,
    repository_id UUID NOT NULL,
    language VARCHAR(50),
    framework VARCHAR(100),
    recipe_id UUID,
    strategy_type VARCHAR(50),
    success BOOLEAN,
    confidence_score FLOAT,
    execution_time_ms INT,
    error_classification VARCHAR(100),
    created_at TIMESTAMP DEFAULT NOW()
);

-- Pattern templates table
CREATE TABLE pattern_templates (
    id UUID PRIMARY KEY,
    pattern_signature TEXT,
    language VARCHAR(50),
    success_rate FLOAT,
    usage_count INT,
    template_recipe JSONB,
    confidence_threshold FLOAT,
    created_at TIMESTAMP DEFAULT NOW(),
    last_used TIMESTAMP
);

-- A/B test experiments
CREATE TABLE ab_experiments (
    id UUID PRIMARY KEY,
    experiment_name VARCHAR(200),
    variant_a_recipe JSONB,
    variant_b_recipe JSONB,
    variant_a_count INT DEFAULT 0,
    variant_b_count INT DEFAULT 0,
    variant_a_success_rate FLOAT,
    variant_b_success_rate FLOAT,
    statistical_significance FLOAT,
    status VARCHAR(50),
    created_at TIMESTAMP DEFAULT NOW()
);

-- Feature extraction for ML
CREATE TABLE transformation_features (
    transformation_id UUID PRIMARY KEY,
    repo_size_kb INT,
    file_count INT,
    complexity_score FLOAT,
    dependency_count INT,
    test_coverage FLOAT,
    language_features JSONB,
    framework_features JSONB,
    outcome_label VARCHAR(50)
);
```

**A/B Testing Framework**:
```go
type ABTestFramework interface {
    CreateExperiment(ctx context.Context, experiment ABExperiment) error
    SelectVariant(ctx context.Context, experimentID string) (*Variant, error)
    RecordOutcome(ctx context.Context, variantID string, success bool) error
    AnalyzeResults(ctx context.Context, experimentID string) (*ABTestResults, error)
    GraduateWinner(ctx context.Context, experimentID string) error
}

type ABExperiment struct {
    ID              string          `json:"id"`
    Name            string          `json:"name"`
    VariantA        Recipe          `json:"variant_a"`
    VariantB        Recipe          `json:"variant_b"`
    TrafficSplit    float64         `json:"traffic_split"`
    MinSampleSize   int             `json:"min_sample_size"`
    ConfidenceLevel float64         `json:"confidence_level"`
}
```

**Acceptance Criteria**:
- Learning system improves strategy selection accuracy by 25% over 100 transformations
- Pattern extraction identifies actionable insights from transformation history
- Recipe template generation creates reusable patterns from successful outcomes
- Model retraining prevents degradation in strategy selection performance
- Continuous improvement demonstrates measurable enhancement over time
- A/B testing achieves statistical significance with 95% confidence
- Learning database handles 1M+ transformation records efficiently

### 5. Transformation Strategy Selection

**Objective**: Create an intelligent system that selects optimal transformation approaches based on historical performance, repository characteristics, and resource constraints.

**Tasks**:
- Create strategy selection matrix based on issue type and complexity
- Implement historical performance analysis for confidence scoring
- Add resource availability assessment for strategy decisions
- Create strategy escalation logic (recipe → LLM → human intervention)
- Implement strategy performance monitoring and optimization

**Deliverables**:
```go
// controller/arf/strategy_selector.go
type StrategySelector interface {
    SelectStrategy(ctx context.Context, request StrategyRequest) (*SelectedStrategy, error)
    EvaluateComplexity(ctx context.Context, repository Repository) (*ComplexityAnalysis, error)
    PredictResourceRequirements(ctx context.Context, strategy TransformationStrategy) (*ResourcePrediction, error)
    RecommendEscalation(ctx context.Context, failures []TransformationFailure) (*EscalationRecommendation, error)
}

type StrategyRequest struct {
    Repository         Repository              `json:"repository"`
    TransformationType TransformationType      `json:"transformation_type"`
    ErrorContext       ErrorContext           `json:"error_context"`
    ResourceConstraints ResourceConstraints    `json:"resource_constraints"`
    TimeConstraints    TimeConstraints        `json:"time_constraints"`
    QualityRequirements QualityRequirements   `json:"quality_requirements"`
}

type SelectedStrategy struct {
    Primary           TransformationStrategy  `json:"primary"`
    Alternatives      []TransformationStrategy `json:"alternatives"`
    Confidence        float64                 `json:"confidence"`
    Reasoning         StrategyReasoning       `json:"reasoning"`
    ResourceEstimate  ResourcePrediction      `json:"resource_estimate"`
    TimeEstimate      time.Duration           `json:"time_estimate"`
    RiskAssessment    RiskAssessment          `json:"risk_assessment"`
}

type ComplexityAnalysis struct {
    OverallComplexity    float64                    `json:"overall_complexity"`
    FactorBreakdown     map[string]float64         `json:"factor_breakdown"`
    PredictedChallenges []PredictedChallenge       `json:"predicted_challenges"`
    RecommendedApproach RecommendedApproach        `json:"recommended_approach"`
}
```

**Acceptance Criteria**:
- Strategy selection optimizes success probability while minimizing resource usage
- Complexity analysis accurately predicts transformation difficulty
- Resource predictions are within 20% of actual usage
- Escalation recommendations prevent unnecessary human intervention
- Performance monitoring enables continuous strategy optimization

### 6. Developer Experience Tooling

**Objective**: Create comprehensive developer tools and IDE integration for recipe development, testing, and debugging.

**Tasks**:
- Create VS Code extension for ARF recipe development
- Implement local transformation preview capabilities
- Add dry-run mode for transformation testing
- Create recipe development SDK with examples
- Build interactive debugging tools for transformations

**Deliverables**:
```go
// controller/arf/developer_tools.go
type DeveloperTools interface {
    PreviewTransformation(ctx context.Context, code string, recipe Recipe) (*PreviewResult, error)
    DryRun(ctx context.Context, repository Repository, recipe Recipe) (*DryRunResult, error)
    DebugTransformation(ctx context.Context, transformationID string) (*DebugSession, error)
    ValidateRecipeLocally(ctx context.Context, recipe Recipe) (*ValidationResult, error)
    GenerateRecipeFromExample(ctx context.Context, before, after string) (*Recipe, error)
}

type PreviewResult struct {
    OriginalCode    string              `json:"original_code"`
    TransformedCode string              `json:"transformed_code"`
    Diff            string              `json:"diff"`
    Warnings        []Warning           `json:"warnings"`
    Confidence      float64             `json:"confidence"`
}

type DryRunResult struct {
    AffectedFiles   []FileChange        `json:"affected_files"`
    EstimatedTime   time.Duration       `json:"estimated_time"`
    RiskAssessment  RiskAssessment      `json:"risk_assessment"`
    Prerequisites   []Prerequisite      `json:"prerequisites"`
    Rollback        RollbackPlan        `json:"rollback"`
}

type DebugSession struct {
    SessionID       string              `json:"session_id"`
    Breakpoints     []Breakpoint        `json:"breakpoints"`
    StepHistory     []TransformStep     `json:"step_history"`
    Variables       map[string]interface{} `json:"variables"`
    ASTSnapshot     *UniversalAST       `json:"ast_snapshot"`
}
```

**VS Code Extension Features**:
```typescript
// vscode-arf-extension/src/features.ts
interface ARFExtension {
    // Recipe development
    recipeEditor: {
        syntax: SyntaxHighlighting;
        validation: RealTimeValidation;
        autoComplete: IntelliSense;
        snippets: RecipeSnippets;
    };
    
    // Testing capabilities
    testing: {
        preview: TransformationPreview;
        dryRun: LocalDryRun;
        unittest: RecipeUnitTests;
        coverage: TransformationCoverage;
    };
    
    // Debugging tools
    debugging: {
        breakpoints: ASTBreakpoints;
        stepThrough: TransformationStepper;
        variables: VariableInspector;
        history: TransformationHistory;
    };
    
    // Integration
    integration: {
        ploy: PloyControllerConnection;
        git: GitIntegration;
        ci: CIPipelineIntegration;
    };
}
```

**Recipe Development SDK**:
```go
// sdk/arf/recipe_sdk.go
type RecipeSDK struct {
    // Recipe creation helpers
    CreateFromTemplate(template string) *Recipe
    AddPrecondition(condition Condition) *Recipe
    AddTransformation(transformation Transformation) *Recipe
    AddPostValidation(validation Validation) *Recipe
    
    // Testing utilities
    TestWithFixture(fixture TestFixture) TestResult
    BenchmarkPerformance(dataset Dataset) BenchmarkResult
    ValidateSafety(recipe Recipe) SafetyReport
    
    // Example recipes
    Examples map[string]Recipe
}
```

**Acceptance Criteria**:
- VS Code extension provides real-time recipe validation
- Local preview shows transformation results in <2 seconds
- Dry-run mode prevents accidental production changes
- SDK includes 50+ example recipes for common patterns
- Debugging tools enable step-by-step transformation analysis
- Extension marketplace rating >4.5 stars

## Configuration Examples

### LLM Integration Configuration
```yaml
# configs/arf-llm-config.yaml
llm_integration:
  provider: "openai"  # openai, anthropic, azure
  model: "gpt-4"
  api_key_secret: "llm-api-key"
  
  generation:
    temperature: 0.1
    max_tokens: 2048
    timeout: "30s"
    retry_attempts: 3
  
  prompting:
    context_window: 8192
    include_surrounding_code: true
    include_build_logs: true
    include_test_results: true
  
  validation:
    syntax_check: true
    sandbox_test: true
    confidence_threshold: 0.7
```

### Hybrid Pipeline Configuration
```yaml
# configs/arf-hybrid-pipeline.yaml
hybrid_pipeline:
  strategy_selection:
    complexity_threshold: 0.8
    resource_weight: 0.3
    time_weight: 0.4
    success_weight: 0.3
  
  confidence_thresholds:
    min_openrewrite: 0.6
    min_llm: 0.7
    min_hybrid: 0.8
    required_build: 0.9
  
  enhancement_modes:
    simple_transformations: "NoEnhancement"
    moderate_complexity: "PostProcessing"
    high_complexity: "FullHybrid"
```

### Learning System Configuration
```yaml
# configs/arf-learning-config.yaml
learning_system:
  pattern_extraction:
    minimum_samples: 10
    time_window: "30d"
    significance_threshold: 0.05
  
  model_retraining:
    frequency: "weekly"
    minimum_data_points: 100
    validation_split: 0.2
  
  strategy_updates:
    weight_adjustment_rate: 0.1
    stability_period: "7d"
    rollback_threshold: 0.95
```

## Nomad Job Templates

### LLM-Enhanced Transformation Job
```hcl
# platform/nomad/templates/arf-llm-transformation.hcl.j2
job "arf-llm-transform-{{ transformation_id }}" {
  datacenters = ["{{ datacenter }}"]
  type = "batch"
  
  constraint {
    attribute = "${attr.kernel.name}"
    value     = "freebsd"
  }
  
  group "hybrid-transformation" {
    task "primary-transform" {
      driver = "jail"
      
      config {
        path = "/zroot/jails/arf-primary-{{ transformation_id }}"
        command = "/usr/local/bin/arf-openrewrite-executor"
        args = [
          "--recipe", "/local/recipe.yaml",
          "--repository", "/input/repository.tar.gz",
          "--output", "/shared/primary-result.tar.gz"
        ]
      }
      
      resources {
        cpu    = 2000
        memory = 4096
        disk   = 10240
      }
    }
    
    task "llm-enhancement" {
      driver = "exec"
      
      config {
        command = "/usr/local/bin/arf-llm-enhancer"
        args = [
          "--primary-result", "/shared/primary-result.tar.gz",
          "--context", "/local/context.json",
          "--output", "/shared/enhanced-result.tar.gz"
        ]
      }
      
      env {
        LLM_API_KEY = "{{ llm_api_key }}"
        LLM_MODEL = "{{ llm_model }}"
        CONFIDENCE_THRESHOLD = "{{ confidence_threshold }}"
      }
      
      resources {
        cpu    = 1000
        memory = 2048
        disk   = 5120
      }
    }
    
    task "validator" {
      driver = "jail"
      
      config {
        path = "/zroot/jails/arf-validator-{{ transformation_id }}"
        command = "/usr/local/bin/arf-validator"
        args = [
          "--result", "/shared/enhanced-result.tar.gz",
          "--validation-suite", "/local/validation.yaml",
          "--output", "/output/final-result.tar.gz"
        ]
      }
      
      resources {
        cpu    = 1000
        memory = 2048
        disk   = 5120
      }
    }
  }
}
```

## API Endpoints

### LLM Recipe Generation
```yaml
# API: POST /v1/arf/recipes/generate
request:
  error_context:
    error_type: "compilation_failure"
    error_message: "Cannot resolve symbol 'HttpServletRequest'"
    source_file: "src/main/java/Controller.java"
    line_number: 15
  codebase_context:
    language: "java"
    framework: "spring-boot"
    version: "2.7.0"
    dependencies: ["spring-web", "spring-data-jpa"]
  constraints:
    - "maintain_existing_functionality"
    - "prefer_spring_6_patterns"

response:
  recipe:
    id: "generated-servlet-migration-001"
    yaml: "..."
  confidence: 0.85
  explanation: "Generated recipe to migrate javax.servlet to jakarta.servlet namespace"
```

### Strategy Selection
```yaml
# API: POST /v1/arf/strategies/select
request:
  repository:
    url: "https://github.com/company/legacy-app"
    language: "java"
    size_kb: 15420
  transformation_type: "framework_migration"
  resource_constraints:
    max_cpu: "4000m"
    max_memory: "8Gi"
    max_duration: "2h"

response:
  primary:
    type: "hybrid"
    openrewrite_recipe: "SpringBoot3Migration"
    llm_enhancement: true
  confidence: 0.92
  resource_estimate:
    cpu: "2500m"
    memory: "6Gi"
    duration: "1h 15m"
```

## Testing Strategy

### Unit Tests
- LLM API integration and error handling
- Recipe generation and validation logic
- Strategy selection algorithms and scoring
- Learning system pattern extraction

### Integration Tests
- End-to-end hybrid transformation workflows
- LLM recipe generation with validation pipeline
- Strategy selection with real repository analysis
- Learning system feedback loop functionality

### Performance Tests
- LLM API response times and rate limiting
- Hybrid pipeline throughput and resource usage
- Learning system processing of large datasets
- Strategy selection performance under load

### AI/ML Tests
- LLM-generated recipe quality assessment
- Strategy selection accuracy measurement
- Learning system improvement validation
- Confidence scoring calibration tests

## Success Metrics

- **Recipe Generation**: 70%+ success rate for LLM-generated recipes
- **Hybrid Performance**: 85%+ success rate vs 70% for deterministic transformations alone
- **Learning Improvement**: 25% strategy selection accuracy improvement over 100 transformations
- **Resource Optimization**: 30% reduction in average transformation time
- **Confidence Calibration**: Confidence scores predict success within 10% accuracy
- **Pattern Recognition**: 90% accuracy in identifying similar transformation patterns
- **Multi-Language Support**: 5+ languages with 80%+ transformation success rate
- **Developer Adoption**: 1000+ VS Code extension installations within 6 months
- **A/B Testing**: 95% statistical confidence in recipe improvements
- **WASM Optimization**: 20%+ size reduction for Lane G modules

## Risk Mitigation

### Technical Risks
- **LLM Availability**: Implement fallback to static recipes and multiple provider support
- **Token Costs**: Monitor usage and implement request optimization and caching
- **Recipe Quality**: Comprehensive validation pipeline with sandbox testing

### Operational Risks
- **Strategy Drift**: Regular validation of strategy selection against benchmarks
- **Learning Bias**: Balanced training data and bias detection in pattern extraction
- **Performance Regression**: Continuous monitoring and rollback capabilities

## Next Phase Dependencies

Phase ARF-3 enables:
- **Phase ARF-4**: Security-focused transformations with AI-enhanced vulnerability analysis
- **Phase ARF-5**: Enterprise analytics with AI-powered insights and reporting

The hybrid intelligence capabilities developed in ARF-3 provide the foundation for sophisticated security analysis and enterprise-scale transformation campaigns.