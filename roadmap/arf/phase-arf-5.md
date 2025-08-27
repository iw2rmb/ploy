# Phase ARF-5: Generic Recipe Management System

**Duration**: 6-8 weeks for complete recipe management platform
**Prerequisites**: Phase ARF-4 completed with deployment integration and benchmark system
**Dependencies**: Recipe storage system, CLI integration, OpenRewrite tooling
**Status**: 📋 **PLANNED** - Universal code transformation platform (2025)

## Overview

Phase ARF-5 transforms ARF from a hardcoded Java migration tool into a **universal code transformation platform** with user-controlled recipe management. This phase replaces fixed transformation logic with a flexible, generic recipe system that allows users to upload, manage, and execute custom transformation recipes alongside built-in OpenRewrite recipes.

The generic recipe management system enables organizations to create, share, and maintain their own transformation logic while leveraging the full power of ARF's deployment integration, benchmarking, and error recovery capabilities.

## Technical Architecture

### Core Components
- **Recipe Management Engine**: CRUD operations for user-defined recipes
- **Generic Recipe Executor**: Plugin-based execution framework supporting multiple recipe types
- **Recipe Storage System**: Metadata indexing, search, and filtering capabilities
- **CLI Recipe Interface**: Complete command-line interface for recipe lifecycle management

### Integration Points
- **ARF Benchmark System**: Execute user recipes through existing benchmark infrastructure
- **OpenRewrite Integration**: Dynamic configuration generation for real OpenRewrite execution
- **Lane C Deployment**: Deploy transformed applications through existing OSv pipeline
- **Template Processing**: Recipe metadata integration with deployment templates

## Implementation Phases

### Phase ARF-5.1: Recipe Data Model & Storage (2 weeks)

**Objective**: Create foundational recipe management infrastructure with storage, validation, and metadata management.

#### Core Data Structures

```go
// api/arf/recipe.go
type Recipe struct {
    Metadata    RecipeMetadata     `json:"metadata" yaml:"metadata"`
    Recipes     []RecipeStep       `json:"recipes" yaml:"recipes"`
    Execution   ExecutionConfig    `json:"execution" yaml:"execution"`
    CreatedAt   time.Time          `json:"created_at"`
    UpdatedAt   time.Time          `json:"updated_at"`
}

type RecipeMetadata struct {
    ID          string            `json:"id" yaml:"id"`
    Name        string            `json:"name" yaml:"name"`
    Description string            `json:"description" yaml:"description"`
    Version     string            `json:"version" yaml:"version"`
    Author      string            `json:"author" yaml:"author"`
    Language    string            `json:"language" yaml:"language"`
    Tags        []string          `json:"tags" yaml:"tags"`
    Homepage    string            `json:"homepage,omitempty" yaml:"homepage,omitempty"`
    Repository  string            `json:"repository,omitempty" yaml:"repository,omitempty"`
}

type RecipeStep struct {
    Type        string                 `json:"type" yaml:"type"`           // openrewrite, script, composite
    ID          string                 `json:"id,omitempty" yaml:"id,omitempty"`
    Script      string                 `json:"script,omitempty" yaml:"script,omitempty"`
    Config      map[string]interface{} `json:"config,omitempty" yaml:"config,omitempty"`
    Condition   string                 `json:"condition,omitempty" yaml:"condition,omitempty"`
}

type ExecutionConfig struct {
    StopOnFailure   bool          `json:"stop_on_failure" yaml:"stop_on_failure"`
    MaxIterations   int           `json:"max_iterations" yaml:"max_iterations"`
    Timeout         time.Duration `json:"timeout" yaml:"timeout"`
    RetryOnFailure  bool          `json:"retry_on_failure" yaml:"retry_on_failure"`
    Environment     map[string]string `json:"environment,omitempty" yaml:"environment,omitempty"`
}
```

#### Recipe Storage Interface

```go
// api/arf/recipe_store.go
type RecipeStore interface {
    // CRUD Operations
    CreateRecipe(ctx context.Context, recipe *Recipe) error
    GetRecipe(ctx context.Context, id string) (*Recipe, error)
    UpdateRecipe(ctx context.Context, recipe *Recipe) error
    DeleteRecipe(ctx context.Context, id string) error
    
    // Query Operations
    ListRecipes(ctx context.Context, filters RecipeFilters) ([]*Recipe, error)
    SearchRecipes(ctx context.Context, query string) ([]*Recipe, error)
    GetRecipesByAuthor(ctx context.Context, author string) ([]*Recipe, error)
    GetRecipesByLanguage(ctx context.Context, language string) ([]*Recipe, error)
    GetRecipesByTags(ctx context.Context, tags []string) ([]*Recipe, error)
    
    // Metadata Operations
    ValidateRecipe(recipe *Recipe) error
    GetRecipeStats() (*RecipeStats, error)
}

type RecipeFilters struct {
    Language    string   `json:"language,omitempty"`
    Tags        []string `json:"tags,omitempty"`
    Author      string   `json:"author,omitempty"`
    MinVersion  string   `json:"min_version,omitempty"`
    MaxVersion  string   `json:"max_version,omitempty"`
    Limit       int      `json:"limit,omitempty"`
    Offset      int      `json:"offset,omitempty"`
}
```

#### File-Based Storage Implementation

```go
// api/arf/file_recipe_store.go
type FileRecipeStore struct {
    basePath    string
    indexCache  map[string]*Recipe
    mutex       sync.RWMutex
}

func NewFileRecipeStore(basePath string) (*FileRecipeStore, error) {
    store := &FileRecipeStore{
        basePath:   basePath,
        indexCache: make(map[string]*Recipe),
    }
    
    // Create directory structure
    if err := os.MkdirAll(filepath.Join(basePath, "recipes"), 0755); err != nil {
        return nil, fmt.Errorf("failed to create recipes directory: %w", err)
    }
    
    // Build initial index
    if err := store.buildIndex(); err != nil {
        return nil, fmt.Errorf("failed to build recipe index: %w", err)
    }
    
    return store, nil
}
```

#### Recipe Validation System

```go
// api/arf/recipe_validator.go
type RecipeValidator struct {
    supportedTypes map[string]bool
    supportedLangs map[string]bool
}

func (v *RecipeValidator) ValidateRecipe(recipe *Recipe) error {
    // Validate metadata
    if err := v.validateMetadata(&recipe.Metadata); err != nil {
        return fmt.Errorf("metadata validation failed: %w", err)
    }
    
    // Validate recipe steps
    for i, step := range recipe.Recipes {
        if err := v.validateRecipeStep(&step, i); err != nil {
            return fmt.Errorf("recipe step %d validation failed: %w", i, err)
        }
    }
    
    // Validate execution config
    if err := v.validateExecutionConfig(&recipe.Execution); err != nil {
        return fmt.Errorf("execution config validation failed: %w", err)
    }
    
    return nil
}
```

### Phase ARF-5.2: CLI Integration & User Interface (1 week)

**Objective**: Implement comprehensive CLI commands for recipe management integrated with existing `ploy arf` command structure.

#### CLI Command Structure

```go
// internal/cli/arf/recipe.go
func NewRecipeCommand() *cli.Command {
    return &cli.Command{
        Name:  "recipe",
        Usage: "Manage ARF transformation recipes",
        Subcommands: []*cli.Command{
            &cli.Command{
                Name:   "list",
                Usage:  "List available recipes",
                Flags: []cli.Flag{
                    &cli.StringFlag{Name: "language", Usage: "Filter by programming language"},
                    &cli.StringSliceFlag{Name: "tags", Usage: "Filter by tags (comma-separated)"},
                    &cli.StringFlag{Name: "author", Usage: "Filter by recipe author"},
                    &cli.StringFlag{Name: "format", Value: "table", Usage: "Output format (table, json, yaml)"},
                },
                Action: listRecipes,
            },
            &cli.Command{
                Name:      "upload",
                Usage:     "Upload a new recipe from file",
                ArgsUsage: "<recipe.yaml>",
                Flags: []cli.Flag{
                    &cli.BoolFlag{Name: "validate-only", Usage: "Only validate recipe without uploading"},
                },
                Action: uploadRecipe,
            },
            &cli.Command{
                Name:      "update", 
                Usage:     "Update an existing recipe",
                ArgsUsage: "<recipe-id> <recipe.yaml>",
                Action: updateRecipe,
            },
            &cli.Command{
                Name:      "delete",
                Usage:     "Delete a recipe",
                ArgsUsage: "<recipe-id>",
                Flags: []cli.Flag{
                    &cli.BoolFlag{Name: "force", Usage: "Skip confirmation prompt"},
                },
                Action: deleteRecipe,
            },
            &cli.Command{
                Name:      "show",
                Usage:     "Show detailed recipe information",
                ArgsUsage: "<recipe-id>",
                Flags: []cli.Flag{
                    &cli.StringFlag{Name: "format", Value: "yaml", Usage: "Output format (yaml, json)"},
                },
                Action: showRecipe,
            },
            &cli.Command{
                Name:   "search",
                Usage:  "Search recipes by keyword",
                ArgsUsage: "<search-query>",
                Action: searchRecipes,
            },
            &cli.Command{
                Name:   "validate",
                Usage:  "Validate a recipe file",
                ArgsUsage: "<recipe.yaml>",
                Action: validateRecipe,
            },
        },
    }
}
```

#### Enhanced Benchmark Integration

```go
// Update existing benchmark command to support recipe arrays
func runBenchmark(c *cli.Context) error {
    // Parse recipes parameter
    recipesFlag := c.StringSlice("recipes")
    if len(recipesFlag) == 0 {
        return fmt.Errorf("--recipes parameter required")
    }
    
    // Validate recipes exist and are accessible
    recipeStore := getRecipeStore()
    for _, recipeID := range recipesFlag {
        if _, err := recipeStore.GetRecipe(c.Context, recipeID); err != nil {
            return fmt.Errorf("recipe '%s' not found: %w", recipeID, err)
        }
    }
    
    // Create benchmark config with user recipes
    config := &BenchmarkConfig{
        Name:         c.String("name"),
        Repository:   c.String("repository"),
        UserRecipes:  recipesFlag,  // New field for user-specified recipes
        // ... existing config
    }
    
    return executeBenchmark(c.Context, config)
}
```

### Phase ARF-5.3: Generic Recipe Execution Engine (2-3 weeks)

**Objective**: Replace hardcoded transformation logic with a flexible, plugin-based recipe execution system supporting multiple transformation types.

#### Generic Recipe Executor Interface

```go
// api/arf/recipe_executor.go
type RecipeExecutor interface {
    ExecuteRecipe(ctx context.Context, recipe *RecipeStep, repoPath string) (*TransformationResult, error)
    ValidateRecipe(recipe *RecipeStep) error
    GetSupportedType() string
    GetRequirements() ExecutorRequirements
}

type ExecutorRequirements struct {
    Tools        []string          `json:"tools"`         // Required external tools
    Environment  map[string]string `json:"environment"`   // Required environment variables
    Languages    []string          `json:"languages"`     // Supported programming languages
}

type GenericRecipeEngine struct {
    executors    map[string]RecipeExecutor
    recipeStore  RecipeStore
    logger       Logger
}

func NewGenericRecipeEngine(recipeStore RecipeStore) *GenericRecipeEngine {
    engine := &GenericRecipeEngine{
        executors:   make(map[string]RecipeExecutor),
        recipeStore: recipeStore,
    }
    
    // Register built-in executors
    engine.RegisterExecutor("openrewrite", NewOpenRewriteExecutor())
    engine.RegisterExecutor("script", NewScriptExecutor())
    engine.RegisterExecutor("composite", NewCompositeExecutor(engine))
    
    return engine
}

func (e *GenericRecipeEngine) ExecuteUserRecipes(ctx context.Context, recipeIDs []string, repoPath string) (*TransformationResult, error) {
    var aggregatedResult *TransformationResult
    
    for _, recipeID := range recipeIDs {
        recipe, err := e.recipeStore.GetRecipe(ctx, recipeID)
        if err != nil {
            return nil, fmt.Errorf("failed to load recipe %s: %w", recipeID, err)
        }
        
        result, err := e.executeRecipe(ctx, recipe, repoPath)
        if err != nil {
            if recipe.Execution.StopOnFailure {
                return nil, fmt.Errorf("recipe %s failed: %w", recipeID, err)
            }
            // Log error and continue
            e.logger.Error("Recipe execution failed", "recipe", recipeID, "error", err)
            continue
        }
        
        // Aggregate results
        aggregatedResult = e.mergeResults(aggregatedResult, result)
    }
    
    return aggregatedResult, nil
}
```

#### OpenRewrite Executor Implementation

```go
// api/arf/openrewrite_executor.go
type OpenRewriteExecutor struct {
    tempDir     string
    javaHome    string
    mavenHome   string
    gradleHome  string
}

func (e *OpenReWriteExecutor) ExecuteRecipe(ctx context.Context, recipe *RecipeStep, repoPath string) (*TransformationResult, error) {
    // Detect build system
    buildSystem := e.detectBuildSystem(repoPath)
    
    switch buildSystem {
    case "maven":
        return e.executeMavenRecipe(ctx, recipe, repoPath)
    case "gradle":
        return e.executeGradleRecipe(ctx, recipe, repoPath)
    default:
        return nil, fmt.Errorf("unsupported build system: %s", buildSystem)
    }
}

func (e *OpenRewriteExecutor) executeMavenRecipe(ctx context.Context, recipe *RecipeStep, repoPath string) (*TransformationResult, error) {
    // Generate temporary rewrite.yml configuration
    configPath := filepath.Join(e.tempDir, "rewrite.yml")
    if err := e.generateRewriteConfig(recipe, configPath); err != nil {
        return nil, fmt.Errorf("failed to generate rewrite config: %w", err)
    }
    
    // Execute maven rewrite:run
    cmd := exec.CommandContext(ctx, "mvn", 
        "org.openrewrite.maven:rewrite-maven-plugin:run",
        "-Drewrite.configLocation=" + configPath,
        "-f", filepath.Join(repoPath, "pom.xml"))
    
    cmd.Dir = repoPath
    output, err := cmd.CombinedOutput()
    
    if err != nil {
        return nil, fmt.Errorf("maven rewrite execution failed: %w, output: %s", err, output)
    }
    
    // Parse results and generate diff
    return e.parseTransformationResults(ctx, repoPath, string(output))
}
```

#### Script Executor Implementation

```go
// api/arf/script_executor.go
type ScriptExecutor struct {
    sandboxManager SandboxManager
    allowedShells  map[string]bool
}

func (e *ScriptExecutor) ExecuteRecipe(ctx context.Context, recipe *RecipeStep, repoPath string) (*TransformationResult, error) {
    // Validate script safety
    if err := e.validateScript(recipe.Script); err != nil {
        return nil, fmt.Errorf("script validation failed: %w", err)
    }
    
    // Create secure execution environment
    sandbox, err := e.sandboxManager.CreateSandbox(ctx, SandboxConfig{
        LocalPath:     repoPath,
        TTL:          30 * time.Minute,
        NetworkAccess: false, // Scripts run offline for security
        CPULimit:     "1",
        MemoryLimit:  "512M",
    })
    if err != nil {
        return nil, fmt.Errorf("failed to create script sandbox: %w", err)
    }
    defer e.sandboxManager.DestroySandbox(ctx, sandbox.ID)
    
    // Execute script in sandbox
    result, err := e.executeScriptInSandbox(ctx, sandbox, recipe.Script)
    if err != nil {
        return nil, fmt.Errorf("script execution failed: %w", err)
    }
    
    return result, nil
}
```

### Phase ARF-5.4: Recipe Discovery & Management Features (1 week)

**Objective**: Implement advanced search, filtering, and recipe ecosystem features for comprehensive recipe management.

#### Advanced Search Implementation

```go
// api/arf/recipe_search.go
type RecipeSearchEngine struct {
    store       RecipeStore
    indexer     *SearchIndexer
}

type SearchQuery struct {
    Query       string            `json:"query"`
    Language    string            `json:"language,omitempty"`
    Tags        []string          `json:"tags,omitempty"`
    Author      string            `json:"author,omitempty"`
    MinVersion  string            `json:"min_version,omitempty"`
    MaxVersion  string            `json:"max_version,omitempty"`
    Limit       int               `json:"limit,omitempty"`
    Offset      int               `json:"offset,omitempty"`
    SortBy      string            `json:"sort_by,omitempty"`    // name, author, created_at, updated_at
    SortOrder   string            `json:"sort_order,omitempty"` // asc, desc
}

func (s *RecipeSearchEngine) Search(ctx context.Context, query SearchQuery) (*SearchResult, error) {
    // Full-text search across recipe metadata
    matches, err := s.indexer.Search(query.Query)
    if err != nil {
        return nil, fmt.Errorf("search index query failed: %w", err)
    }
    
    // Apply filters
    filtered := s.applyFilters(matches, query)
    
    // Sort results
    sorted := s.sortResults(filtered, query.SortBy, query.SortOrder)
    
    // Apply pagination
    paginated := s.paginate(sorted, query.Limit, query.Offset)
    
    return &SearchResult{
        Recipes:    paginated,
        Total:      len(filtered),
        Query:      query,
        ExecutionTime: time.Since(start),
    }, nil
}
```

#### Built-in Recipe Catalog

```go
// api/arf/builtin_recipes.go
type BuiltinRecipeCatalog struct {
    recipes map[string]*Recipe
}

func NewBuiltinRecipeCatalog() *BuiltinRecipeCatalog {
    catalog := &BuiltinRecipeCatalog{
        recipes: make(map[string]*Recipe),
    }
    
    // Load built-in recipes
    catalog.loadJavaRecipes()
    catalog.loadSpringRecipes()
    catalog.loadCleanupRecipes()
    
    return catalog
}

func (c *BuiltinRecipeCatalog) loadJavaRecipes() {
    // Java 8 to 11 migration
    c.recipes["java8to11"] = &Recipe{
        Metadata: RecipeMetadata{
            ID:          "java8to11",
            Name:        "Java 8 to 11 Migration",
            Description: "Complete Java 8 to 11 migration with API updates",
            Version:     "1.0.0",
            Author:      "ploy-builtin",
            Language:    "java",
            Tags:        []string{"java", "migration", "jvm"},
        },
        Recipes: []RecipeStep{
            {Type: "openrewrite", ID: "org.openrewrite.java.migrate.JavaVersion8to11"},
            {Type: "openrewrite", ID: "org.openrewrite.java.migrate.javax.HttpClientMigration"},
        },
        Execution: ExecutionConfig{
            StopOnFailure: true,
            MaxIterations: 1,
            Timeout:       30 * time.Minute,
        },
    }
    
    // Similar patterns for java11to17, java17to21, etc.
}
```

#### Recipe Recommendation System

```go
// api/arf/recipe_recommender.go
type RecipeRecommender struct {
    catalog         *BuiltinRecipeCatalog
    projectAnalyzer *ProjectAnalyzer
}

func (r *RecipeRecommender) RecommendRecipes(ctx context.Context, projectPath string) ([]*Recipe, error) {
    // Analyze project structure
    analysis, err := r.projectAnalyzer.AnalyzeProject(projectPath)
    if err != nil {
        return nil, fmt.Errorf("project analysis failed: %w", err)
    }
    
    var recommendations []*Recipe
    
    // Language-specific recommendations
    switch analysis.Language {
    case "java":
        recommendations = append(recommendations, r.recommendJavaRecipes(analysis)...)
    case "python":
        recommendations = append(recommendations, r.recommendPythonRecipes(analysis)...)
    }
    
    // Framework-specific recommendations  
    if analysis.HasFramework("spring-boot") {
        recommendations = append(recommendations, r.recommendSpringRecipes(analysis)...)
    }
    
    return recommendations, nil
}
```

## API Integration

### REST API Endpoints

```go
// Recipe Management API
POST   /v1/arf/recipes                    // Upload new recipe
GET    /v1/arf/recipes                    // List recipes with filtering
GET    /v1/arf/recipes/{id}               // Get specific recipe
PUT    /v1/arf/recipes/{id}               // Update recipe
DELETE /v1/arf/recipes/{id}               // Delete recipe

// Recipe Discovery API  
GET    /v1/arf/recipes/search             // Search recipes
GET    /v1/arf/recipes/recommend          // Get recipe recommendations
GET    /v1/arf/recipes/builtin            // List built-in recipes
GET    /v1/arf/recipes/stats              // Get recipe statistics

// Recipe Execution API
POST   /v1/arf/recipes/validate           // Validate recipe without storing
POST   /v1/arf/benchmark/run              // Run benchmark with custom recipes
```

### Benchmark Integration

```go
// Enhanced BenchmarkConfig with recipe support
type BenchmarkConfig struct {
    // Existing fields...
    UserRecipes    []string `json:"user_recipes,omitempty"`    // User recipe IDs
    BuiltinRecipes []string `json:"builtin_recipes,omitempty"` // Built-in recipe IDs
    RecipeConfig   map[string]interface{} `json:"recipe_config,omitempty"` // Per-recipe configuration
}
```

## Success Metrics

### Functional Requirements
- ✅ Users can upload, update, delete custom recipes via CLI and API
- ✅ Recipe arrays support complex multi-step transformations  
- ✅ OpenRewrite recipes execute with real Maven/Gradle integration
- ✅ Custom script recipes execute securely in sandboxed environments
- ✅ Recipe search finds relevant recipes in <2 seconds
- ✅ Built-in recipe catalog provides 20+ common migration patterns

### Performance Requirements
- Recipe upload and validation completes in <30 seconds
- Recipe search handles 1000+ recipes with <2 second response time
- Recipe execution integrates with existing benchmark system
- Support 10+ concurrent recipe executions without conflicts

### User Experience  
- Intuitive CLI following existing `ploy arf` patterns
- Clear validation error messages with specific line numbers
- Recipe recommendations based on project analysis
- Comprehensive recipe documentation with examples

## Next Phase Dependencies

Phase ARF-5 enables:
- **Phase ARF-6**: Enterprise recipe governance with approval workflows
- **Phase ARF-7**: Recipe marketplace and sharing ecosystem  
- **Phase ARF-8**: Advanced analytics and recipe performance optimization

The generic recipe management foundation transforms ARF into a universal code transformation platform where any migration logic can be expressed, shared, and executed through the existing ARF infrastructure.