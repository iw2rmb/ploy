# Phase ARF-5.1: Recipe Data Model & Storage

**Status**: ✅ **COMPLETED** (2025-08-25)  
**Dependencies**: Phase ARF-4 (Benchmark Infrastructure) ✅  
**Actual Effort**: 1 day (2025-08-25)  
**Priority**: CRITICAL  

## Overview

Phase ARF-5.1 establishes the foundational data structures, storage backend, and validation systems for the Generic Recipe Management System. This phase transforms ARF from hardcoded transformations into a flexible, user-controlled recipe execution platform.

## Objectives

1. ✅ **Recipe Data Structure Design**: Define comprehensive Recipe schema supporting multiple transformation types
2. ✅ **Storage Backend Implementation**: Integrate with existing SeaweedFS infrastructure for recipe persistence
3. ✅ **Validation Framework**: Ensure recipe integrity and security before execution
4. ✅ **YAML/JSON Format Specification**: Standardize human-readable recipe definitions
5. ✅ **Migration Path**: Transition existing hardcoded recipes to new data model

## Technical Specifications

### Core Recipe Data Model

```go
// Recipe represents a complete code transformation recipe
type Recipe struct {
    // Metadata for recipe identification and management
    Metadata    RecipeMetadata     `json:"metadata" yaml:"metadata"`
    
    // Sequential transformation steps
    Steps       []RecipeStep       `json:"steps" yaml:"steps"`
    
    // Execution configuration and constraints
    Execution   ExecutionConfig    `json:"execution" yaml:"execution"`
    
    // Validation rules for target codebases
    Validation  ValidationRules    `json:"validation" yaml:"validation"`
    
    // System fields (managed automatically)
    ID          string             `json:"id"`
    CreatedAt   time.Time          `json:"created_at"`
    UpdatedAt   time.Time          `json:"updated_at"`
    UploadedBy  string             `json:"uploaded_by"`
    Hash        string             `json:"hash"`        // Content hash for integrity
    Version     string             `json:"version"`     // Semantic version
}

// RecipeMetadata contains human-readable recipe information
type RecipeMetadata struct {
    Name         string            `json:"name" yaml:"name"`
    Description  string            `json:"description" yaml:"description"`
    Author       string            `json:"author" yaml:"author"`
    Version      string            `json:"version" yaml:"version"`
    License      string            `json:"license,omitempty" yaml:"license,omitempty"`
    Homepage     string            `json:"homepage,omitempty" yaml:"homepage,omitempty"`
    Repository   string            `json:"repository,omitempty" yaml:"repository,omitempty"`
    
    // Categorization and discovery
    Tags         []string          `json:"tags" yaml:"tags"`
    Categories   []string          `json:"categories" yaml:"categories"`
    Languages    []string          `json:"languages" yaml:"languages"`
    Frameworks   []string          `json:"frameworks,omitempty" yaml:"frameworks,omitempty"`
    
    // Compatibility and requirements
    MinPlatform  string            `json:"min_platform,omitempty" yaml:"min_platform,omitempty"`
    MaxPlatform  string            `json:"max_platform,omitempty" yaml:"max_platform,omitempty"`
    Requirements []string          `json:"requirements,omitempty" yaml:"requirements,omitempty"`
}

// RecipeStep defines a single transformation operation
type RecipeStep struct {
    Name        string                 `json:"name" yaml:"name"`
    Type        RecipeStepType         `json:"type" yaml:"type"`
    Config      map[string]interface{} `json:"config" yaml:"config"`
    Conditions  []ExecutionCondition   `json:"conditions,omitempty" yaml:"conditions,omitempty"`
    OnError     ErrorHandlingAction    `json:"on_error,omitempty" yaml:"on_error,omitempty"`
    Timeout     time.Duration          `json:"timeout,omitempty" yaml:"timeout,omitempty"`
}

// RecipeStepType defines supported transformation types
type RecipeStepType string

const (
    StepTypeOpenRewrite    RecipeStepType = "openrewrite"
    StepTypeShellScript    RecipeStepType = "shell"
    StepTypeFileOperation  RecipeStepType = "file_op"
    StepTypeRegexReplace   RecipeStepType = "regex"
    StepTypeASTTransform   RecipeStepType = "ast_transform"
    StepTypeComposite      RecipeStepType = "composite"
)

// ExecutionConfig controls recipe execution behavior
type ExecutionConfig struct {
    Parallelism    int               `json:"parallelism,omitempty" yaml:"parallelism,omitempty"`
    MaxDuration    time.Duration     `json:"max_duration,omitempty" yaml:"max_duration,omitempty"`
    RetryPolicy    RetryPolicy       `json:"retry_policy,omitempty" yaml:"retry_policy,omitempty"`
    Sandbox        SandboxConfig     `json:"sandbox,omitempty" yaml:"sandbox,omitempty"`
    Environment    map[string]string `json:"environment,omitempty" yaml:"environment,omitempty"`
    WorkingDir     string            `json:"working_dir,omitempty" yaml:"working_dir,omitempty"`
}

// ValidationRules define codebase compatibility checks
type ValidationRules struct {
    RequiredFiles     []string          `json:"required_files,omitempty" yaml:"required_files,omitempty"`
    ForbiddenFiles    []string          `json:"forbidden_files,omitempty" yaml:"forbidden_files,omitempty"`
    FilePatterns      []string          `json:"file_patterns,omitempty" yaml:"file_patterns,omitempty"`
    MinFileCount      int               `json:"min_file_count,omitempty" yaml:"min_file_count,omitempty"`
    MaxRepoSize       int64             `json:"max_repo_size,omitempty" yaml:"max_repo_size,omitempty"`
    LanguageDetection LanguageDetection `json:"language_detection,omitempty" yaml:"language_detection,omitempty"`
}
```

### Storage Backend Architecture

```go
// RecipeStorage handles persistent recipe management
type RecipeStorage interface {
    // CRUD Operations
    CreateRecipe(ctx context.Context, recipe *Recipe) error
    GetRecipe(ctx context.Context, id string) (*Recipe, error)
    UpdateRecipe(ctx context.Context, id string, recipe *Recipe) error
    DeleteRecipe(ctx context.Context, id string) error
    
    // Query Operations
    ListRecipes(ctx context.Context, filter RecipeFilter) ([]*Recipe, error)
    SearchRecipes(ctx context.Context, query string) ([]*Recipe, error)
    GetRecipeVersions(ctx context.Context, name string) ([]*Recipe, error)
    
    // Bulk Operations
    ImportRecipes(ctx context.Context, recipes []*Recipe) error
    ExportRecipes(ctx context.Context, filter RecipeFilter) ([]*Recipe, error)
    
    // Integrity Operations
    ValidateRecipe(ctx context.Context, recipe *Recipe) error
    CheckRecipeIntegrity(ctx context.Context, id string) error
}

// SeaweedFSRecipeStorage implements RecipeStorage using SeaweedFS
type SeaweedFSRecipeStorage struct {
    client      *seaweedfs.Client
    bucketName  string
    indexStore  RecipeIndexStore  // Secondary index for fast queries
    validator   *RecipeValidator
}
```

### Recipe Index System

```go
// RecipeIndexStore provides fast query capabilities
type RecipeIndexStore interface {
    // Index Management
    BuildIndex(ctx context.Context) error
    UpdateIndex(ctx context.Context, recipe *Recipe, action IndexAction) error
    
    // Query Interface
    QueryByTags(ctx context.Context, tags []string) ([]string, error)
    QueryByLanguage(ctx context.Context, language string) ([]string, error)
    QueryByCategory(ctx context.Context, category string) ([]string, error)
    FullTextSearch(ctx context.Context, query string) ([]string, error)
    
    // Statistics
    GetIndexStats(ctx context.Context) (*IndexStats, error)
}

// ConsulRecipeIndex implements RecipeIndexStore using Consul KV
type ConsulRecipeIndex struct {
    client   *consul.Client
    keyPath  string
}
```

### Recipe Validation Framework

```go
// RecipeValidator ensures recipe safety and correctness
type RecipeValidator struct {
    securityRules   SecurityRuleSet
    syntaxValidator SyntaxValidator
    schemaValidator SchemaValidator
}

// Validation methods
func (v *RecipeValidator) ValidateRecipe(recipe *Recipe) error {
    // 1. Schema validation
    if err := v.schemaValidator.ValidateSchema(recipe); err != nil {
        return fmt.Errorf("schema validation failed: %w", err)
    }
    
    // 2. Security validation
    if err := v.securityRules.ValidateSteps(recipe.Steps); err != nil {
        return fmt.Errorf("security validation failed: %w", err)
    }
    
    // 3. Syntax validation for embedded scripts/configs
    if err := v.syntaxValidator.ValidateSteps(recipe.Steps); err != nil {
        return fmt.Errorf("syntax validation failed: %w", err)
    }
    
    return nil
}

// SecurityRuleSet defines security constraints
type SecurityRuleSet struct {
    AllowedCommands     []string
    ForbiddenCommands   []string
    MaxExecutionTime    time.Duration
    AllowNetworkAccess  bool
    AllowFileSystemWrite bool
    SandboxRequired     bool
}
```

## Implementation Plan

### Core Data Structures
- ✅ Define Recipe struct hierarchy and supporting types
- ✅ Implement JSON/YAML serialization with comprehensive test suite
- ✅ Create recipe example templates for common transformation types

### Storage Backend Integration
- ✅ Implement SeaweedFSRecipeStorage with CRUD operations
- ✅ Develop ConsulRecipeIndex for fast query capabilities
- ✅ Integration testing with existing SeaweedFS infrastructure

### Validation Framework
- ✅ Build RecipeValidator with security rule enforcement
- ✅ Implement schema validation using JSON Schema
- ✅ Create validation test suite with edge cases and attack scenarios

### Migration and Integration
- ✅ Convert existing BuiltinOpenRewriteEngine recipes to new format
- ✅ Update ARF benchmark system to use new Recipe data model
- ✅ Performance testing and optimization

## File Structure Changes

```
api/arf/
├── storage/
│   ├── recipe_storage.go           # RecipeStorage interface and implementations
│   ├── seaweedfs_storage.go        # SeaweedFS-based recipe storage
│   ├── consul_index.go             # Consul-based recipe indexing
│   └── storage_test.go             # Storage layer tests
├── validation/
│   ├── recipe_validator.go         # Recipe validation framework
│   ├── security_rules.go           # Security constraint definitions
│   ├── schema_validator.go         # JSON Schema validation
│   └── validation_test.go          # Validation tests
├── models/
│   ├── recipe.go                   # Core Recipe data structures
│   ├── recipe_metadata.go          # Metadata and categorization
│   ├── execution_config.go         # Execution configuration
│   └── validation_rules.go         # Validation rule definitions
└── examples/
    ├── java11to17.yaml             # Java migration recipe example
    ├── spring-boot3-upgrade.yaml   # Spring Boot upgrade recipe
    └── generic-cleanup.yaml        # Generic code cleanup recipes
```

## Configuration Schema

### Recipe YAML Format Specification

```yaml
# Recipe metadata
metadata:
  name: "java11-to-java17-migration"
  description: "Migrates Java projects from version 11 to 17 with modern language features"
  author: "Ploy Platform"
  version: "1.2.0"
  license: "MIT"
  tags: ["java", "migration", "java17"]
  categories: ["language-upgrade"]
  languages: ["java"]
  frameworks: ["spring-boot", "maven", "gradle"]

# Transformation steps
steps:
  - name: "Update Maven Java Version"
    type: "openrewrite"
    config:
      recipe: "org.openrewrite.java.migrate.Java11toJava17"
      options:
        target_version: "17"
        update_compiler_plugin: true
    conditions:
      - file_exists: "pom.xml"
    on_error: "continue"
    timeout: "5m"

  - name: "Update Gradle Java Version"
    type: "openrewrite"
    config:
      recipe: "org.openrewrite.java.migrate.JavaVersion11to17"
    conditions:
      - file_exists: "build.gradle"
    on_error: "continue"

  - name: "Apply Text Blocks Transformation"
    type: "openrewrite"
    config:
      recipe: "org.openrewrite.java.migrate.lang.StringLiteralsToTextBlocks"
    conditions:
      - language: "java"
      - min_java_version: "15"

# Execution configuration
execution:
  parallelism: 2
  max_duration: "15m"
  sandbox:
    enabled: true
    allow_network: false
    max_memory: "2GB"
  environment:
    JAVA_HOME: "/usr/lib/jvm/java-17-openjdk"

# Validation rules
validation:
  required_files: ["pom.xml", "build.gradle"]
  file_patterns: ["**/*.java"]
  min_file_count: 1
  max_repo_size: 1073741824  # 1GB
  language_detection:
    primary: "java"
    min_confidence: 0.7
```

### Storage Configuration

```yaml
# configs/arf-storage.yaml
recipe_storage:
  backend: "seaweedfs"
  seaweedfs:
    master_url: "http://localhost:9333"
    bucket: "ploy-recipes"
    volume_type: "replicated"
    replication: "001"
  
  index:
    backend: "consul"
    consul:
      address: "localhost:8500"
      key_prefix: "ploy/arf/recipes/"
      datacenter: "dc1"
  
  validation:
    security_rules:
      max_execution_time: "30m"
      allow_network_access: false
      allow_filesystem_write: true
      sandbox_required: true
      forbidden_commands: ["rm", "sudo", "curl", "wget"]
    
    schema_validation: true
    syntax_validation: true
```

## Testing Strategy

### Unit Tests
- Recipe struct serialization/deserialization
- Storage backend CRUD operations
- Validation framework with malicious input scenarios
- Index query performance and accuracy

### Integration Tests
- End-to-end recipe creation, storage, and retrieval
- Recipe execution within sandbox environments
- Multi-step recipe transformation validation
- Storage backend failover scenarios

### Security Tests
- Recipe validation against injection attacks
- Sandbox escape attempt detection
- Resource limitation enforcement
- Privilege escalation prevention

## Migration Strategy

### Phase 1: Parallel Implementation
- Implement new Recipe system alongside existing BuiltinOpenRewriteEngine
- Create adapter layer to execute legacy recipes through new system
- Gradual migration of hardcoded recipes to YAML format

### Phase 2: Recipe Conversion
- Convert existing Java migration recipes to new format
- Create migration tool for user-defined recipes
- Validate converted recipes maintain functionality

### Phase 3: Legacy Deprecation
- Mark BuiltinOpenRewriteEngine as deprecated
- Provide migration path for users with custom integrations
- Remove legacy implementation after deprecation period

## Success Metrics

### Functionality Metrics
- **Recipe CRUD Success Rate**: >99.9% for all storage operations
- **Validation Accuracy**: 100% detection of security violations
- **Query Performance**: Sub-100ms for recipe search operations
- **Storage Efficiency**: <10% overhead compared to raw file storage

### Security Metrics
- **Zero Security Breaches**: No successful sandbox escapes or privilege escalations
- **Validation Coverage**: 100% of security rules tested with edge cases
- **Attack Simulation**: Successful defense against common injection patterns

### Performance Metrics
- **Recipe Load Time**: <500ms for large recipe files (>1MB)
- **Concurrent Operations**: Support for 100+ concurrent recipe operations
- **Index Update Speed**: <1s for recipe index updates
- **Storage Scalability**: Support for 10,000+ recipes without performance degradation

## ✅ Implementation Results (2025-08-25)

### Completed Deliverables
- ✅ **Recipe Data Model**: Complete models.Recipe with metadata, steps, validation rules
- ✅ **SeaweedFS Storage Backend**: Production-ready with retry logic, caching, deletion markers  
- ✅ **Consul Index Backend**: Enhanced search with relevance scoring and performance optimization
- ✅ **Recipe Validation System**: Security rules enforcement with resource limits and command filtering
- ✅ **Configuration Management**: Environment-driven backend selection (production/development)
- ✅ **API Handler Integration**: All endpoints updated to use storage backend instead of catalog
- ✅ **Comprehensive Testing**: Four complete test suites with VPS-ready runtime validation
- ✅ **Nomad Template Updates**: Production and development deployment configurations

### Performance Achievements
- ✅ **Recipe Operations**: All CRUD operations with proper error handling and retries
- ✅ **Search Performance**: Full-text search with metadata filtering and relevance ranking  
- ✅ **Storage Efficiency**: Caching layer with TTL-based invalidation
- ✅ **Graceful Fallbacks**: Seamless failover between storage and catalog interfaces
- ✅ **Security Validation**: Complete security rule framework with sandbox requirements

## Next Phase Dependencies

✅ **Phase ARF-5.1 COMPLETE** - All deliverables implemented and tested, enabling:
- **Phase ARF-5.2**: CLI Integration can now use complete Recipe storage system
- **Phase ARF-5.3**: Generic Execution Engine has full Recipe data model and storage
- **Phase ARF-5.4**: Discovery Features can leverage implemented indexing and search

## Risk Assessment

### Technical Risks
- **SeaweedFS Integration Complexity**: Mitigated by extensive testing and fallback mechanisms
- **Recipe Validation Performance**: Addressed through caching and parallel validation
- **Storage Consistency**: Handled via transactional operations and integrity checks

### Security Risks
- **Recipe Injection Attacks**: Prevented through comprehensive validation framework
- **Sandbox Escape**: Mitigated by strict execution environment controls
- **Resource Exhaustion**: Protected by timeout and resource limitation enforcement

### Operational Risks
- **Migration Complexity**: Reduced through phased approach and backward compatibility
- **Storage Backend Failure**: Addressed by redundancy and backup strategies
- **Recipe Corruption**: Prevented through content hashing and integrity validation