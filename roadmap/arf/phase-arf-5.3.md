# Phase ARF-5.3: Generic Recipe Execution Engine

**Status**: ✅ **IMPLEMENTED**  
**Dependencies**: Phase ARF-5.1 (Recipe Data Model) ✅, Phase ARF-5.2 (CLI Integration) ✅  
  
**Priority**: CRITICAL  

## Overview

Phase ARF-5.3 implements the core execution engine that transforms ARF from a mock transformation system into a production-ready code transformation platform. This phase replaces the current `BuiltinOpenRewriteEngine` with a generic, pluggable execution framework supporting real OpenRewrite integration, shell scripts, AST transformations, and custom transformation types.

## Objectives

1. ✅ **Generic Recipe Executor**: Plugin-based framework supporting multiple transformation types
2. ✅ **Real OpenRewrite Integration**: Replace mock implementations with actual Maven/Gradle OpenRewrite execution
3. ✅ **Execution Orchestration**: Manage multi-step recipe execution with error handling and rollback
4. ✅ **Sandbox Security**: Isolated execution environments for untrusted transformations
5. ✅ **Performance Optimization**: Parallel execution, caching, and resource management
6. ✅ **Extensibility Framework**: Plugin architecture for custom transformation engines

## Technical Specifications

### Core Execution Architecture

```go
// RecipeExecutor orchestrates transformation recipe execution
type RecipeExecutor struct {
    engines       map[RecipeStepType]ExecutionEngine
    sandbox       SandboxManager
    storage       RecipeStorage      // From Phase 5.1
    validator     *RecipeValidator   // From Phase 5.1
    metrics       MetricsCollector
    logger        *zap.Logger
}

// ExecutionEngine defines the interface for transformation engines
type ExecutionEngine interface {
    // Engine metadata
    GetType() RecipeStepType
    GetVersion() string
    GetCapabilities() EngineCapabilities
    
    // Execution methods
    Execute(ctx context.Context, step *RecipeStep, workspace *Workspace) (*StepResult, error)
    Validate(step *RecipeStep) error
    GetEstimatedDuration(step *RecipeStep) time.Duration
    
    // Lifecycle methods
    Initialize(ctx context.Context, config EngineConfig) error
    Cleanup(ctx context.Context) error
}

// Workspace represents an isolated execution environment
type Workspace struct {
    ID           string
    BasePath     string
    TempPath     string
    Metadata     WorkspaceMetadata
    Isolation    IsolationLevel
    Resources    ResourceLimits
    Environment  map[string]string
}

// ExecutionResult contains comprehensive execution information
type ExecutionResult struct {
    RecipeID        string                    `json:"recipe_id"`
    Success         bool                      `json:"success"`
    StartTime       time.Time                 `json:"start_time"`
    EndTime         time.Time                 `json:"end_time"`
    Duration        time.Duration             `json:"duration"`
    StepResults     []StepResult              `json:"step_results"`
    FilesModified   []string                  `json:"files_modified"`
    FilesCreated    []string                  `json:"files_created"`
    FilesDeleted    []string                  `json:"files_deleted"`
    Diff            string                    `json:"diff"`
    Error           string                    `json:"error,omitempty"`
    ResourceUsage   ResourceUsage             `json:"resource_usage"`
    Artifacts       []ExecutionArtifact       `json:"artifacts"`
}
```

### OpenRewrite Integration Engine

```go
// OpenRewriteEngine implements real OpenRewrite transformations
type OpenRewriteEngine struct {
    mavenPath      string
    gradlePath     string
    javaHome       string
    rewriteVersion string
    pluginVersion  string
    tempDir        string
}

func (e *OpenRewriteEngine) Execute(ctx context.Context, step *RecipeStep, workspace *Workspace) (*StepResult, error) {
    config, err := e.parseStepConfig(step.Config)
    if err != nil {
        return nil, fmt.Errorf("invalid OpenRewrite config: %w", err)
    }
    
    // Detect build system (Maven vs Gradle)
    buildSystem, err := e.detectBuildSystem(workspace.BasePath)
    if err != nil {
        return nil, fmt.Errorf("unable to detect build system: %w", err)
    }
    
    // Execute based on build system
    switch buildSystem {
    case BuildSystemMaven:
        return e.executeMavenRewrite(ctx, config, workspace)
    case BuildSystemGradle:
        return e.executeGradleRewrite(ctx, config, workspace)
    default:
        return nil, fmt.Errorf("unsupported build system: %s", buildSystem)
    }
}

func (e *OpenRewriteEngine) executeMavenRewrite(ctx context.Context, config *OpenRewriteConfig, workspace *Workspace) (*StepResult, error) {
    // Create temporary pom.xml with OpenRewrite plugin configuration
    tempPom, err := e.createOpenRewriteMavenConfig(config, workspace)
    if err != nil {
        return nil, err
    }
    defer os.Remove(tempPom)
    
    // Execute OpenRewrite Maven goal
    cmd := exec.CommandContext(ctx, e.mavenPath,
        "-f", tempPom,
        "org.openrewrite.maven:rewrite-maven-plugin:" + e.pluginVersion + ":run",
        "-Drewrite.recipeArtifactCoordinates=" + config.RecipeArtifacts,
        "-Drewrite.activeRecipes=" + strings.Join(config.ActiveRecipes, ","),
    )
    
    cmd.Dir = workspace.BasePath
    cmd.Env = e.buildEnvironment(workspace)
    
    // Capture output for analysis
    var stdout, stderr bytes.Buffer
    cmd.Stdout = &stdout
    cmd.Stderr = &stderr
    
    startTime := time.Now()
    err = cmd.Run()
    duration := time.Since(startTime)
    
    // Parse execution results
    result := &StepResult{
        StepName:      config.Name,
        Success:       err == nil,
        Duration:      duration,
        ExecutionLog:  stdout.String(),
        ErrorLog:      stderr.String(),
    }
    
    if err == nil {
        // Analyze changes made by OpenRewrite
        result.FilesModified, result.Diff, err = e.analyzeChanges(workspace)
        if err != nil {
            result.Success = false
            result.Error = fmt.Sprintf("Failed to analyze changes: %v", err)
        }
    }
    
    return result, nil
}

func (e *OpenRewriteEngine) createOpenRewriteMavenConfig(config *OpenRewriteConfig, workspace *Workspace) (string, error) {
    // Read original pom.xml
    originalPom := filepath.Join(workspace.BasePath, "pom.xml")
    pomContent, err := ioutil.ReadFile(originalPom)
    if err != nil {
        return "", err
    }
    
    // Parse and modify POM to add OpenRewrite plugin
    modifiedPom, err := e.addOpenRewritePlugin(string(pomContent), config)
    if err != nil {
        return "", err
    }
    
    // Write temporary POM
    tempPomPath := filepath.Join(workspace.TempPath, "rewrite-pom.xml")
    err = ioutil.WriteFile(tempPomPath, []byte(modifiedPom), 0644)
    return tempPomPath, err
}
```

### Shell Script Engine

```go
// ShellScriptEngine executes shell commands and scripts
type ShellScriptEngine struct {
    allowedCommands   []string
    forbiddenCommands []string
    maxDuration       time.Duration
    shellPath         string
}

func (e *ShellScriptEngine) Execute(ctx context.Context, step *RecipeStep, workspace *Workspace) (*StepResult, error) {
    config, err := e.parseShellConfig(step.Config)
    if err != nil {
        return nil, err
    }
    
    // Security validation
    if err := e.validateScript(config.Script); err != nil {
        return nil, fmt.Errorf("script security validation failed: %w", err)
    }
    
    // Create script file in workspace
    scriptPath := filepath.Join(workspace.TempPath, "transform.sh")
    err = ioutil.WriteFile(scriptPath, []byte(config.Script), 0755)
    if err != nil {
        return nil, err
    }
    
    // Execute script with timeout
    ctx, cancel := context.WithTimeout(ctx, e.maxDuration)
    defer cancel()
    
    cmd := exec.CommandContext(ctx, e.shellPath, scriptPath)
    cmd.Dir = workspace.BasePath
    cmd.Env = e.buildScriptEnvironment(workspace, config.Environment)
    
    var stdout, stderr bytes.Buffer
    cmd.Stdout = &stdout
    cmd.Stderr = &stderr
    
    startTime := time.Now()
    err = cmd.Run()
    duration := time.Since(startTime)
    
    result := &StepResult{
        StepName:     step.Name,
        Success:      err == nil,
        Duration:     duration,
        ExecutionLog: stdout.String(),
        ErrorLog:     stderr.String(),
    }
    
    if err == nil {
        result.FilesModified, result.Diff, _ = e.detectChanges(workspace)
    }
    
    return result, nil
}

func (e *ShellScriptEngine) validateScript(script string) error {
    // Check for forbidden commands
    for _, forbidden := range e.forbiddenCommands {
        if strings.Contains(script, forbidden) {
            return fmt.Errorf("forbidden command detected: %s", forbidden)
        }
    }
    
    // Additional security checks
    if strings.Contains(script, "eval") || strings.Contains(script, "exec") {
        return errors.New("dynamic code execution not allowed")
    }
    
    return nil
}
```

### AST Transformation Engine

```go
// ASTTransformEngine provides language-specific AST transformations
type ASTTransformEngine struct {
    parsers map[string]ASTParser
}

type ASTParser interface {
    Parse(filePath string) (ASTNode, error)
    Transform(node ASTNode, rules TransformRules) (ASTNode, error)
    Generate(node ASTNode) (string, error)
}

func (e *ASTTransformEngine) Execute(ctx context.Context, step *RecipeStep, workspace *Workspace) (*StepResult, error) {
    config, err := e.parseASTConfig(step.Config)
    if err != nil {
        return nil, err
    }
    
    parser, exists := e.parsers[config.Language]
    if !exists {
        return nil, fmt.Errorf("unsupported language: %s", config.Language)
    }
    
    result := &StepResult{
        StepName: step.Name,
        Success:  true,
    }
    
    startTime := time.Now()
    
    // Find files matching pattern
    files, err := e.findTargetFiles(workspace.BasePath, config.FilePattern)
    if err != nil {
        return nil, err
    }
    
    // Transform each file
    for _, filePath := range files {
        if err := e.transformFile(filePath, parser, config.Rules); err != nil {
            result.Success = false
            result.Error = fmt.Sprintf("Transform failed for %s: %v", filePath, err)
            break
        }
        
        relPath, _ := filepath.Rel(workspace.BasePath, filePath)
        result.FilesModified = append(result.FilesModified, relPath)
    }
    
    result.Duration = time.Since(startTime)
    return result, nil
}
```

### Execution Orchestration

```go
// ExecutionOrchestrator manages multi-step recipe execution
type ExecutionOrchestrator struct {
    executor    *RecipeExecutor
    workspace   WorkspaceManager
    rollback    RollbackManager
}

func (o *ExecutionOrchestrator) ExecuteRecipe(ctx context.Context, recipe *Recipe, workspace *Workspace) (*ExecutionResult, error) {
    result := &ExecutionResult{
        RecipeID:    recipe.ID,
        StartTime:   time.Now(),
        StepResults: make([]StepResult, 0, len(recipe.Steps)),
    }
    
    // Create rollback checkpoint
    checkpoint, err := o.rollback.CreateCheckpoint(workspace)
    if err != nil {
        return nil, fmt.Errorf("failed to create rollback checkpoint: %w", err)
    }
    defer o.rollback.CleanupCheckpoint(checkpoint)
    
    // Execute steps sequentially or in parallel based on configuration
    if recipe.Execution.Parallelism > 1 {
        return o.executeParallel(ctx, recipe, workspace, result)
    } else {
        return o.executeSequential(ctx, recipe, workspace, result)
    }
}

func (o *ExecutionOrchestrator) executeSequential(ctx context.Context, recipe *Recipe, workspace *Workspace, result *ExecutionResult) (*ExecutionResult, error) {
    for i, step := range recipe.Steps {
        // Check conditions
        if !o.evaluateConditions(step.Conditions, workspace) {
            continue
        }
        
        // Execute step
        stepResult, err := o.executor.ExecuteStep(ctx, &step, workspace)
        if err != nil {
            result.Success = false
            result.Error = fmt.Sprintf("Step %d failed: %v", i+1, err)
            
            // Handle rollback based on error policy
            if step.OnError == ErrorActionRollback {
                o.rollback.RestoreCheckpoint(workspace, checkpoint)
            }
            break
        }
        
        result.StepResults = append(result.StepResults, *stepResult)
        
        // Aggregate file changes
        result.FilesModified = append(result.FilesModified, stepResult.FilesModified...)
        result.FilesCreated = append(result.FilesCreated, stepResult.FilesCreated...)
        result.FilesDeleted = append(result.FilesDeleted, stepResult.FilesDeleted...)
    }
    
    result.EndTime = time.Now()
    result.Duration = result.EndTime.Sub(result.StartTime)
    result.Success = result.Error == ""
    
    // Generate unified diff
    if result.Success {
        result.Diff, _ = o.generateUnifiedDiff(workspace)
    }
    
    return result, nil
}
```

### Sandbox & Security Framework

```go
// SandboxManager provides isolated execution environments
type SandboxManager interface {
    CreateSandbox(ctx context.Context, config SandboxConfig) (*Sandbox, error)
    DestroySandbox(ctx context.Context, sandbox *Sandbox) error
    ExecuteInSandbox(ctx context.Context, sandbox *Sandbox, cmd *exec.Cmd) error
}

// DockerSandboxManager implements sandboxing using Docker containers
type DockerSandboxManager struct {
    client     *docker.Client
    baseImages map[string]string
    network    string
}

func (sm *DockerSandboxManager) CreateSandbox(ctx context.Context, config SandboxConfig) (*Sandbox, error) {
    // Create Docker container with resource limits
    containerConfig := &container.Config{
        Image: sm.baseImages[config.Runtime],
        Cmd:   []string{"sleep", "infinity"},
        Env:   config.Environment,
        WorkingDir: "/workspace",
    }
    
    hostConfig := &container.HostConfig{
        Memory:      config.MaxMemory,
        CPUQuota:    config.CPUQuota,
        NetworkMode: container.NetworkMode(sm.network),
        ReadonlyRootfs: true,
        Tmpfs: map[string]string{
            "/tmp": "rw,noexec,nosuid,size=100m",
            "/workspace": "rw,size=1g",
        },
    }
    
    resp, err := sm.client.ContainerCreate(ctx, containerConfig, hostConfig, nil, nil, "")
    if err != nil {
        return nil, err
    }
    
    // Start container
    if err := sm.client.ContainerStart(ctx, resp.ID, types.ContainerStartOptions{}); err != nil {
        sm.client.ContainerRemove(ctx, resp.ID, types.ContainerRemoveOptions{})
        return nil, err
    }
    
    return &Sandbox{
        ID:          resp.ID,
        Type:        SandboxTypeDocker,
        Runtime:     config.Runtime,
        WorkspacePath: "/workspace",
    }, nil
}
```

## Implementation Plan

### Core Engine Framework
- ✅ Implement RecipeExecutor interface and plugin architecture
- ✅ Build basic execution orchestration and error handling
- ✅ Create workspace management and isolation framework

### OpenRewrite Integration
- ✅ Implement real OpenRewrite engine for Maven projects
- ✅ Add Gradle support and build system detection

### Additional Engines & Security
- ✅ Implement ShellScriptEngine with security validation
- ✅ Build ASTTransformEngine for basic language support
- ✅ Implement Docker-based sandboxing framework

### Orchestration & Performance
- ✅ Build parallel execution capabilities
- ✅ Implement rollback and error recovery mechanisms
- ✅ Add comprehensive logging and metrics collection

### Integration & Optimization
- ✅ Integration testing with Phase 5.1 storage and Phase 5.2 CLI
- ✅ Performance optimization and resource management
- ✅ Documentation and example recipe conversions

## Testing Strategy

### Unit Tests
- Individual execution engine functionality
- Workspace isolation and cleanup
- Error handling and rollback mechanisms
- Security validation and sandbox escaping

### Integration Tests
- End-to-end recipe execution with real projects
- Multi-step recipe orchestration
- Cross-engine recipe composition
- Resource limitation and timeout enforcement

### Security Tests
- Sandbox escape attempts
- Malicious script execution prevention
- Resource exhaustion attacks
- Privilege escalation detection

### Performance Tests
- Large repository transformation performance
- Parallel execution scaling
- Memory and CPU resource usage
- Network isolation effectiveness

## Real OpenRewrite Configuration

### Maven Plugin Integration

```xml
<!-- Temporary POM modification for OpenRewrite execution -->
<plugin>
    <groupId>org.openrewrite.maven</groupId>
    <artifactId>rewrite-maven-plugin</artifactId>
    <version>${rewrite.version}</version>
    <configuration>
        <activeRecipes>
            <recipe>org.openrewrite.java.migrate.Java11toJava17</recipe>
        </activeRecipes>
        <failOnDryRunResults>false</failOnDryRunResults>
        <plainTextMasks>
            <mask>**/application*.yml</mask>
            <mask>**/application*.yaml</mask>
            <mask>**/application*.properties</mask>
        </plainTextMasks>
    </configuration>
    <dependencies>
        <dependency>
            <groupId>org.openrewrite.recipe</groupId>
            <artifactId>rewrite-migrate-java</artifactId>
            <version>${rewrite-migrate-java.version}</version>
        </dependency>
    </dependencies>
</plugin>
```

### Gradle Plugin Integration

```gradle
// Temporary build.gradle modification for OpenRewrite execution
plugins {
    id 'org.openrewrite.rewrite' version "${rewriteVersion}"
}

rewrite {
    activeRecipe('org.openrewrite.java.migrate.Java11toJava17')
    failOnDryRunResults = false
    plainTextMasks = ['**/application*.yml', '**/application*.yaml', '**/application*.properties']
}

dependencies {
    rewrite('org.openrewrite.recipe:rewrite-migrate-java:latest.release')
}
```

## File Structure Changes

```
controller/arf/
├── execution/
│   ├── executor.go                 # Main RecipeExecutor implementation
│   ├── orchestrator.go             # Multi-step execution orchestration
│   ├── workspace.go                # Workspace management
│   └── rollback.go                 # Rollback and recovery mechanisms
├── engines/
│   ├── engine.go                   # ExecutionEngine interface
│   ├── openrewrite/
│   │   ├── openrewrite_engine.go   # Real OpenRewrite integration
│   │   ├── maven_executor.go       # Maven-based OpenRewrite execution
│   │   ├── gradle_executor.go      # Gradle-based OpenRewrite execution
│   │   └── build_detector.go       # Build system detection
│   ├── shell/
│   │   ├── shell_engine.go         # Shell script execution engine
│   │   └── security_validator.go   # Script security validation
│   ├── ast/
│   │   ├── ast_engine.go           # AST transformation engine
│   │   ├── java_parser.go          # Java AST parser
│   │   └── go_parser.go            # Go AST parser
│   └── composite/
│       └── composite_engine.go     # Multi-engine recipe composition
├── sandbox/
│   ├── sandbox.go                  # Sandbox interface and types
│   ├── docker_sandbox.go           # Docker-based sandboxing
│   └── process_sandbox.go          # Process-based sandboxing
└── metrics/
    ├── collector.go                # Metrics collection interface
    └── prometheus_collector.go     # Prometheus metrics integration
```

## Success Metrics

### Functionality Metrics
- **OpenRewrite Integration**: 100% compatibility with OpenRewrite Maven/Gradle plugins
- **Execution Success Rate**: >95% for well-formed recipes
- **Multi-Engine Support**: Support for 4+ execution engine types
- **Sandbox Isolation**: 100% containment of untrusted code execution

### Performance Metrics
- **Execution Speed**: <2x overhead compared to direct tool execution
- **Parallel Scaling**: Linear scaling up to 4 parallel steps
- **Resource Efficiency**: <20% overhead for sandbox isolation
- **Memory Usage**: <500MB baseline + recipe-dependent scaling

### Security Metrics
- **Zero Sandbox Escapes**: No successful breakouts during testing
- **Script Validation**: 100% detection of known malicious patterns
- **Resource Exhaustion Prevention**: Automatic termination of resource abuse
- **Network Isolation**: Complete prevention of unauthorized network access

## Migration from BuiltinOpenRewriteEngine

### Phase 1: Parallel Implementation
- Keep existing BuiltinOpenRewriteEngine operational
- Implement new RecipeExecutor alongside
- Create adapter to execute legacy recipes through new system

### Phase 2: Recipe Conversion
- Convert existing hardcoded recipes to YAML format
- Test converted recipes against known repositories
- Validate output matches legacy implementation

### Phase 3: Cutover and Deprecation
- Switch benchmark system to use new RecipeExecutor
- Mark BuiltinOpenRewriteEngine as deprecated
- Remove legacy implementation after validation period

## Integration Points

### With Phase 5.1 (Storage)
- Load recipes from RecipeStorage
- Validate recipes using RecipeValidator
- Store execution results and artifacts

### With Phase 5.2 (CLI)
- Execute recipes through `ploy arf recipe run` command
- Provide real-time execution progress updates
- Generate comprehensive execution reports

### With Existing Benchmark System
- Replace mock transformations with real executions
- Maintain compatibility with existing benchmark workflows
- Enhance reporting with actual transformation metrics

## Risk Assessment

### Technical Risks
- **OpenRewrite Version Compatibility**: Mitigated through version pinning and compatibility testing
- **Sandbox Performance Overhead**: Addressed through lightweight isolation and optimization
- **Multi-Engine Complexity**: Managed through clean interfaces and comprehensive testing

### Security Risks  
- **Untrusted Recipe Execution**: Prevented through mandatory sandboxing and validation
- **Resource Exhaustion**: Protected by strict resource limits and timeouts
- **Privilege Escalation**: Blocked by minimal privilege execution contexts

### Operational Risks
- **Migration Complexity**: Reduced through phased approach and backward compatibility
- **Performance Regression**: Monitored through comprehensive benchmarking
- **Recipe Ecosystem Fragmentation**: Addressed through standardization and validation tools