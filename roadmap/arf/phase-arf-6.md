# Phase ARF-6: Intelligent Dependency Resolution ⏳ PARTIALLY IMPLEMENTED

**Status**: Several components implemented via healing workflow (2025-09-02)
**Prerequisites**: Phases ARF-1 through ARF-4 completed (framework and LLM integration) ✅
**Dependencies**: Web search APIs, dependency repositories (Maven Central, npm, PyPI), knowledge base infrastructure

## Overview

Phase ARF-6 introduces intelligent dependency resolution capabilities to ARF, addressing one of the most time-consuming challenges in software modernization: resolving dependency conflicts during major version upgrades. This phase implements automated detection, analysis, and resolution of dependency issues that commonly occur during framework migrations (e.g., Java 8→11→17→21, Spring Boot 2→3, Node 14→18).

The system leverages minimal reproduction environments, web intelligence gathering, iterative resolution strategies, and machine learning to automatically resolve "Symbol not found", "ClassNotFoundException", and similar dependency-related errors that typically require hours of manual debugging.

## Problem Statement

Dependency resolution challenges include:
- **Version Conflicts**: Incompatible library versions after framework upgrades
- **Transitive Dependencies**: Diamond dependency problems and version mismatches
- **API Breaking Changes**: Methods/classes removed or signatures changed
- **Missing Dependencies**: Libraries no longer included or relocated
- **Build System Issues**: Plugin incompatibilities with new framework versions

Current manual resolution requires:
- Extensive web searching for compatible versions
- Trial-and-error version testing
- Full project rebuilds for each attempt
- Deep understanding of dependency graphs
- Knowledge of framework migration patterns

## Implementation Status

### Completed Components (via Healing Workflow)
- ✅ **Dependency Conflict Detection**: Implemented through healing workflow error analysis
- ✅ **Iterative Resolution**: Recursive healing attempts with child transformations
- ✅ **LLM-based Solution Generation**: OpenRewrite recipe generation via LLM integration
- ✅ **Error Pattern Recognition**: Healing workflow identifies and categorizes build/test failures
- ✅ **Parallel Testing**: Concurrent healing attempts via HealingCoordinator

### Pending Components
- ❌ **Minimal Reproduction Environment Generator**: Not yet implemented
- ❌ **Web Intelligence Integration**: Stack Overflow, GitHub search not integrated
- ❌ **Version Compatibility Matrix**: Automated version discovery pending
- ❌ **Knowledge Base Persistence**: Pattern learning exists but not persistent

## Technical Architecture

### Core Components

#### 1. Dependency Graph Analyzer
- **Comprehensive Graph Building**: Complete dependency tree analysis including transitive dependencies
- **Conflict Detection**: Identify version conflicts, diamond dependencies, excluded transitives ✅ (Partial via healing)
- **Impact Prediction**: Analyze cascading effects of version changes
- **Resolution Strategy Generation**: Create ordered resolution plans based on dependency relationships ✅ (Via healing)

#### 2. Minimal Reproduction Environment Generator
- **Code Extraction**: Isolate only code relevant to dependency issue (target: <5% of original)
- **Environment Creation**: Minimal build configuration with only problematic dependencies
- **Multi-Lane Support**: Use appropriate Ploy lane (including WASM/Lane G for speed)
- **Fast Iteration**: 90% reduction in test compilation time

#### 3. Web Intelligence Integration
- **Search Providers**: Stack Overflow, GitHub Issues, Maven Central, package registries
- **Compatibility Matrix Extraction**: Parse documentation for version requirements
- **Error Pattern Matching**: Map error messages to known solutions
- **Confidence Scoring**: Rate solutions based on source credibility and match quality

#### 4. Iterative Version Resolver
- **Binary Search Algorithm**: Efficiently find working version ranges
- **Parallel A/B Testing**: Test multiple strategies simultaneously
- **Constraint Solver**: Handle complex multi-dependency constraints
- **Rollback Safety**: Automatic restoration on validation failure

#### 5. Knowledge Base System
- **Pattern Storage**: Save successful resolution patterns
- **Similarity Matching**: Find similar issues from history
- **Recipe Generation**: Create OpenRewrite recipes for Java, custom formats for others
- **Cross-Project Learning**: Apply learnings across different projects

### Integration Points
- **ARF Engine**: Leverage existing transformation infrastructure
- **LLM Integration**: Use Phase 3 LLM for understanding error messages
- **Learning System**: Extend Phase 3 pattern learning for dependency patterns
- **Sandbox Manager**: Use existing sandbox infrastructure for testing

## Implementation Tasks

### Phase 6A: Core Infrastructure (Month 1)

#### 1. Dependency Graph Analysis System

```go
// api/arf/dependency_resolver.go
type DependencyResolver interface {
    AnalyzeDependencyGraph(ctx context.Context, project Project) (*DependencyGraph, error)
    DetectConflicts(ctx context.Context, graph *DependencyGraph) ([]DependencyConflict, error)
    PredictImpact(ctx context.Context, change DependencyChange) (*ImpactAnalysis, error)
    GenerateResolutionPlan(ctx context.Context, conflicts []DependencyConflict) (*ResolutionPlan, error)
}

type DependencyGraph struct {
    RootDependencies    []Dependency              `json:"root_dependencies"`
    TransitiveDeps      map[string][]Dependency   `json:"transitive_deps"`
    ConflictClusters    []ConflictCluster         `json:"conflict_clusters"`
    ResolutionOrder     []string                  `json:"resolution_order"`
}

type DependencyConflict struct {
    Type            ConflictType    `json:"type"` // version_mismatch, missing_symbol, api_change
    Dependencies    []Dependency    `json:"dependencies"`
    ErrorMessage    string          `json:"error_message"`
    StackTrace      []string        `json:"stack_trace"`
    Severity        string          `json:"severity"`
}
```

#### 2. Minimal Reproduction Generator

```go
// api/arf/minimal_repro.go
type MinimalReproGenerator interface {
    ExtractRelevantCode(ctx context.Context, issue DependencyIssue) (*CodeSubset, error)
    CreateMinimalProject(ctx context.Context, subset *CodeSubset) (*MinimalProject, error)
    GenerateTestHarness(ctx context.Context, issue DependencyIssue) (*TestHarness, error)
    CompileInLane(ctx context.Context, project *MinimalProject, lane string) (*CompilationResult, error)
}

type MinimalProject struct {
    SourceFiles     []string                `json:"source_files"`
    BuildConfig     map[string]interface{}  `json:"build_config"`
    Dependencies    []Dependency            `json:"dependencies"`
    TestCommand     string                  `json:"test_command"`
    SizeReduction   float64                 `json:"size_reduction"` // % reduction from original
}
```

**Deliverables:**
- Dependency graph builder for Maven, Gradle, npm, pip
- AST-based code extraction for minimal reproduction
- Build configuration minimizer
- Test harness generator

### Phase 6B: Intelligence Layer (Month 2)

#### 1. Web Search Integration

```go
// api/arf/web_intelligence.go
type WebIntelligence interface {
    SearchStackOverflow(ctx context.Context, error DependencyError) ([]Solution, error)
    SearchGitHubIssues(ctx context.Context, error DependencyError) ([]IssueResolution, error)
    QueryMavenCentral(ctx context.Context, artifact string) (*VersionInfo, error)
    ExtractCompatibilityMatrix(ctx context.Context, docs string) (*CompatibilityMatrix, error)
}

type Solution struct {
    Source          string          `json:"source"`
    URL             string          `json:"url"`
    Description     string          `json:"description"`
    VersionChanges  []VersionChange `json:"version_changes"`
    Confidence      float64         `json:"confidence"`
    Votes           int             `json:"votes"`
}
```

#### 2. Knowledge Base Implementation

```go
// api/arf/dependency_knowledge_base.go
type DependencyKnowledgeBase interface {
    StoreResolution(ctx context.Context, resolution ResolutionRecord) error
    FindSimilarIssues(ctx context.Context, issue DependencyIssue) ([]ResolutionRecord, error)
    GenerateRecipe(ctx context.Context, resolution ResolutionRecord) (*Recipe, error)
    GetStatistics(ctx context.Context) (*KnowledgeBaseStats, error)
}

type ResolutionRecord struct {
    ID              string                  `json:"id"`
    Issue           DependencyIssue         `json:"issue"`
    Solution        Solution                `json:"solution"`
    SuccessRate     float64                 `json:"success_rate"`
    TimeToResolve   time.Duration           `json:"time_to_resolve"`
    ProjectContext  map[string]interface{}  `json:"project_context"`
}
```

**Deliverables:**
- Stack Overflow API integration
- GitHub search integration
- Maven Central/npm/PyPI API clients
- Knowledge base schema (SQL) — deferred (no SQL database in use)
- Pattern matching algorithms

### Phase 6C: Advanced Resolution (Month 3)

#### 1. Iterative Version Resolver

```go
// api/arf/version_resolver.go
type VersionResolver interface {
    BinarySearchVersion(ctx context.Context, dep Dependency, validator Validator) (*Version, error)
    ParallelTestStrategies(ctx context.Context, strategies []ResolutionStrategy) (*BestStrategy, error)
    SolveConstraints(ctx context.Context, constraints []Constraint) (*Solution, error)
    ValidateResolution(ctx context.Context, resolution *Resolution) (*ValidationResult, error)
}

type ResolutionStrategy struct {
    ID              string                  `json:"id"`
    Name            string                  `json:"name"`
    Actions         []ResolutionAction      `json:"actions"`
    Parallelizable  bool                    `json:"parallelizable"`
    Confidence      float64                 `json:"confidence"`
    EstimatedTime   time.Duration           `json:"estimated_time"`
}
```

#### 2. A/B Testing Framework

```go
// api/arf/dependency_ab_testing.go
type DependencyABTester interface {
    CreateTestVariants(ctx context.Context, base MinimalProject, strategies []ResolutionStrategy) ([]TestVariant, error)
    ExecuteParallelTests(ctx context.Context, variants []TestVariant) ([]TestResult, error)
    SelectBestResult(ctx context.Context, results []TestResult) (*TestResult, error)
    GenerateReport(ctx context.Context, results []TestResult) (*ABTestReport, error)
}
```

**Deliverables:**
- Binary search version finder
- Parallel strategy executor
- Constraint solver implementation
- A/B testing coordinator
- Result analysis and selection

### Phase 6D: Production Hardening (Month 4)

#### 1. Performance Optimization
- Caching layer for web search results
- Dependency graph caching
- Parallel resolution execution
- Resource usage optimization

#### 2. OpenRewrite Recipe Generation
- Convert successful resolutions to OpenRewrite recipes
- Custom recipe format for non-Java languages
- Recipe validation and testing
- Recipe catalog integration

#### 3. Production Features
- Progress tracking and reporting
- Rollback capabilities
- Audit logging
- Metrics and monitoring

## Configuration

### Dependency Resolution Configuration

```yaml
# configs/arf-dependency-resolution.yaml
dependency_resolution:
  graph_analysis:
    max_depth: 10
    include_optional: false
    include_test: true
    
  minimal_repro:
    target_size_reduction: 0.95  # 95% reduction
    max_files: 20
    timeout: 5m
    preferred_lane: "G"  # Use WASM for speed
    
  web_intelligence:
    providers:
      - stack_overflow
      - github_issues
      - maven_central
    max_results_per_provider: 20
    confidence_threshold: 0.7
    
  version_resolution:
    max_parallel_tests: 10
    binary_search_iterations: 10
    timeout_per_test: 2m
    
  knowledge_base:
    similarity_threshold: 0.85
    min_success_rate: 0.8
    retention_days: 365
```

### Web Search Configuration

```yaml
# configs/arf-web-sources.yaml
web_sources:
  stack_overflow:
    api_key: "${STACK_OVERFLOW_API_KEY}"
    rate_limit: 100
    tags:
      - java
      - maven
      - gradle
      - spring-boot
      
  github:
    token: "${GITHUB_TOKEN}"
    search_repos:
      - spring-projects/spring-boot
      - spring-projects/spring-framework
      
  maven_central:
    base_url: "https://search.maven.org"
    timeout: 10s
```

## Issue Categories

### Version Conflicts
- **Symbol Not Found**: Class/method no longer exists
- **Method Signature Changed**: Parameters or return type modified
- **Class Not Found**: Package relocated or removed
- **Package Removed**: Entire package deprecated

### Transitive Conflicts
- **Diamond Dependency**: Multiple versions via different paths
- **Version Mismatch**: Incompatible transitive versions
- **Excluded Transitive**: Required transitive excluded

### API Breaking Changes
- **Removed API**: Method/class completely removed
- **Changed Semantics**: Same signature, different behavior
- **Deprecated Usage**: Using deprecated APIs

### Build System Issues
- **Plugin Incompatibility**: Build plugins incompatible with new version
- **Lifecycle Changes**: Build lifecycle modifications
- **Configuration Format**: Build config format changes

### Runtime Issues
- **Classloader Conflicts**: Multiple versions loaded
- **Module System Issues**: Java 9+ module conflicts
- **Reflection Failures**: Reflection-based code failures

## API Endpoints

```yaml
# Dependency Analysis
POST   /v1/arf/dependencies/analyze       # Analyze project dependencies
GET    /v1/arf/dependencies/graph         # Get dependency graph
POST   /v1/arf/dependencies/conflicts     # Detect conflicts

# Resolution
POST   /v1/arf/dependencies/resolve       # Start resolution process
GET    /v1/arf/dependencies/resolve/{id}  # Get resolution status
POST   /v1/arf/dependencies/test          # Test resolution strategy

# Knowledge Base
GET    /v1/arf/dependencies/knowledge     # Search knowledge base
POST   /v1/arf/dependencies/knowledge     # Add resolution to KB
GET    /v1/arf/dependencies/recipes       # Get generated recipes

# Web Intelligence
POST   /v1/arf/dependencies/search        # Search web for solutions
GET    /v1/arf/dependencies/compatibility # Get compatibility matrix
```

## CLI Commands

```bash
# Analyze dependencies
ploy arf deps analyze --project ./my-app

# Resolve conflicts
ploy arf deps resolve --issue "Symbol not found: javax.annotation.PostConstruct"

# Test resolution
ploy arf deps test --strategy upgrade-javax-to-jakarta

# Search knowledge base
ploy arf deps search --error "ClassNotFoundException"

# Generate recipe
ploy arf deps recipe --resolution res-123
```

## Success Metrics

- **Resolution Success Rate**: >85% for common dependency issues
- **Time to Resolution**: <5 minutes average (vs hours manual)
- **Knowledge Base Hit Rate**: >60% after 6 months
- **Minimal Reproduction Size**: <5% of original codebase
- **False Positive Rate**: <5%
- **Parallel Test Efficiency**: 10x faster than sequential

## Testing Strategy

### Unit Tests
- Dependency graph builder tests
- Minimal reproduction generator tests
- Version resolver algorithm tests
- Knowledge base operations tests

### Integration Tests
- End-to-end resolution workflow
- Web search integration tests
- Multi-language support tests
- A/B testing framework tests

### Performance Tests
- Large dependency graph analysis
- Parallel resolution execution
- Cache effectiveness
- Resource usage under load

## Risk Mitigation

### Technical Risks
- **Web API Limits**: Implement caching and rate limiting
- **False Positives**: Multi-stage validation
- **Version Availability**: Check artifact repositories
- **Network Dependencies**: Offline fallback mode

### Operational Risks
- **Resource Usage**: Limit parallel executions
- **Long-Running Resolutions**: Implement timeouts
- **Knowledge Base Growth**: Implement retention policies
- **Breaking Changes**: Maintain rollback capability

## Future Enhancements

- Machine learning for resolution prediction
- Dependency vulnerability correlation
- Performance impact analysis
- Cross-language dependency resolution
- Automated dependency updates
- Integration with Dependabot/Renovate

## Dependencies on Other Phases

- **Phase 1**: Uses sandbox infrastructure for testing
- **Phase 2**: Leverages error recovery mechanisms
- **Phase 3**: Uses LLM for error understanding
- **Phase 4**: Integrates with security scanning
- **Phase 5**: Scales to multi-repository campaigns

Phase ARF-6 transforms dependency resolution from a manual, time-consuming process into an intelligent, automated system that learns from each resolution to become more effective over time.
