# Stream A: Core Transformation Pipeline

## Overview
**Goal**: Get basic Java 11→17 migration working ASAP  
**Dependencies**: None (can work independently)  
**Deliverable**: Working OpenRewrite transformation service with HTTP API

## Phase A1: Git & OpenRewrite Executor

### Objectives
- [x] Implement Git repository management ✅ 2025-08-26
- [x] Create OpenRewrite executor for Maven/Gradle ✅ 2025-08-26
- [x] Generate diffs from transformations ✅ 2025-08-26
- [x] Test with real Java projects locally ✅ 2025-08-26

### A1.1: Git Repository Management

#### Implementation
```go
// internal/git/manager.go
package git

type Manager struct {
    workDir   string
    gitBinary string
}

// InitializeRepo creates a Git repo from tar archive
func (m *Manager) InitializeRepo(ctx context.Context, jobID string, tarData []byte) (string, error) {
    repoPath := filepath.Join(m.workDir, jobID)
    
    // Extract tar to directory
    if err := m.extractTar(repoPath, tarData); err != nil {
        return "", fmt.Errorf("tar extraction failed: %w", err)
    }
    
    // Initialize git repo
    if err := m.runGitCommand(ctx, repoPath, "init"); err != nil {
        return "", fmt.Errorf("git init failed: %w", err)
    }
    
    // Configure git user
    commands := [][]string{
        {"config", "user.email", "openrewrite@ploy.local"},
        {"config", "user.name", "OpenRewrite Service"},
    }
    
    for _, cmd := range commands {
        if err := m.runGitCommand(ctx, repoPath, cmd...); err != nil {
            return "", fmt.Errorf("git config failed: %w", err)
        }
    }
    
    // Initial commit
    if err := m.runGitCommand(ctx, repoPath, "add", "."); err != nil {
        return "", err
    }
    if err := m.runGitCommand(ctx, repoPath, "commit", "-m", "Initial state"); err != nil {
        return "", err
    }
    if err := m.runGitCommand(ctx, repoPath, "tag", "before-transform"); err != nil {
        return "", err
    }
    
    return repoPath, nil
}

// GenerateDiff creates a unified diff after transformation
func (m *Manager) GenerateDiff(ctx context.Context, repoPath string) ([]byte, error) {
    // Stage all changes
    if err := m.runGitCommand(ctx, repoPath, "add", "."); err != nil {
        return nil, err
    }
    
    // Commit changes
    if err := m.runGitCommand(ctx, repoPath, "commit", "-m", "After transformation"); err != nil {
        return nil, err
    }
    
    // Generate diff
    return m.runGitCommandOutput(ctx, repoPath, 
        "diff", "before-transform", "HEAD", "--unified=3")
}
```

### A1.2: OpenRewrite Executor

#### Implementation
```go
// internal/openrewrite/executor.go
package openrewrite

type Executor struct {
    gitManager *git.Manager
    mavenPath  string
    gradlePath string
    javaHome   string
}

func (e *Executor) Execute(ctx context.Context, jobID string, tarData []byte, recipe RecipeConfig) (*Result, error) {
    // Initialize Git repo
    repoPath, err := e.gitManager.InitializeRepo(ctx, jobID, tarData)
    if err != nil {
        return nil, err
    }
    defer e.cleanup(repoPath)
    
    // Detect build system
    buildSystem := e.detectBuildSystem(repoPath)
    if buildSystem == "" {
        return nil, fmt.Errorf("no supported build system found")
    }
    
    // Execute transformation
    var execErr error
    switch buildSystem {
    case "maven":
        execErr = e.executeMaven(ctx, repoPath, recipe)
    case "gradle":
        execErr = e.executeGradle(ctx, repoPath, recipe)
    }
    
    if execErr != nil {
        return nil, execErr
    }
    
    // Generate diff
    diff, err := e.gitManager.GenerateDiff(ctx, repoPath)
    if err != nil {
        return nil, err
    }
    
    return &Result{
        Success: true,
        Diff:    diff,
    }, nil
}

func (e *Executor) executeMaven(ctx context.Context, repoPath string, recipe RecipeConfig) error {
    // Create rewrite.yml
    rewriteYaml := fmt.Sprintf(`---
type: specs.openrewrite.org/v1beta/recipe
name: CustomTransformation
recipeList:
  - %s
`, recipe.Recipe)
    
    yamlPath := filepath.Join(repoPath, "rewrite.yml")
    if err := os.WriteFile(yamlPath, []byte(rewriteYaml), 0644); err != nil {
        return err
    }
    
    // Run Maven
    args := []string{
        "org.openrewrite.maven:rewrite-maven-plugin:5.34.0:run",
        fmt.Sprintf("-Drewrite.recipeArtifactCoordinates=%s", recipe.Artifacts),
        "-Drewrite.activeRecipes=CustomTransformation",
    }
    
    cmd := exec.CommandContext(ctx, e.mavenPath, args...)
    cmd.Dir = repoPath
    
    return cmd.Run()
}
```

### A1.3: Testing Checklist
- [x] Test with Java 8 Tutorial repository ✅ 2025-08-26
- [x] Verify Java 11→17 migration works ✅ 2025-08-26
- [x] Ensure diff generation is correct ✅ 2025-08-26
- [x] Measure transformation time ✅ 2025-08-26

## Phase A2: HTTP API

### Objectives
- [x] Create REST API with Fiber framework ✅ 2025-08-26
- [x] Implement synchronous /transform endpoint ✅ 2025-08-26
- [x] Add basic health check ✅ 2025-08-26
- [x] Enable local testing ✅ 2025-08-26

### A2.1: API Server

#### Implementation
```go
// cmd/server/main.go
package main

import (
    "github.com/gofiber/fiber/v2"
    "github.com/gofiber/fiber/v2/middleware/logger"
)

func main() {
    app := fiber.New()
    app.Use(logger.New())
    
    executor := openrewrite.NewExecutor()
    handler := api.NewHandler(executor)
    
    app.Post("/transform", handler.Transform)
    app.Get("/health", handler.Health)
    
    app.Listen(":8090")
}
```

#### Transform Handler
```go
// internal/api/handler.go
package api

type Handler struct {
    executor *openrewrite.Executor
}

func (h *Handler) Transform(c *fiber.Ctx) error {
    var req TransformRequest
    if err := c.BodyParser(&req); err != nil {
        return c.Status(400).JSON(fiber.Map{
            "error": "Invalid request",
        })
    }
    
    // Decode base64 tar
    tarData, err := base64.StdEncoding.DecodeString(req.TarArchive)
    if err != nil {
        return c.Status(400).JSON(fiber.Map{
            "error": "Invalid tar archive",
        })
    }
    
    // Execute transformation (synchronous for MVP)
    result, err := h.executor.Execute(c.Context(), req.JobID, tarData, req.RecipeConfig)
    if err != nil {
        return c.Status(500).JSON(fiber.Map{
            "error": err.Error(),
        })
    }
    
    // Return diff
    return c.JSON(fiber.Map{
        "success": true,
        "diff":    base64.StdEncoding.EncodeToString(result.Diff),
    })
}

func (h *Handler) Health(c *fiber.Ctx) error {
    return c.JSON(fiber.Map{
        "status": "healthy",
        "version": "1.0.0",
    })
}
```

### A2.2: Request/Response Format
```json
// Request
{
  "job_id": "test-123",
  "recipe_config": {
    "recipe": "org.openrewrite.java.migrate.UpgradeToJava17",
    "artifacts": "org.openrewrite.recipe:rewrite-migrate-java:3.15.0"
  },
  "tar_archive": "<base64-encoded-tar>"
}

// Response
{
  "success": true,
  "diff": "<base64-encoded-diff>"
}
```

### A2.3: Testing Checklist
- [x] API responds to health check ✅ 2025-08-26
- [x] Transform endpoint accepts tar and returns diff ✅ 2025-08-26
- [x] Error handling works correctly ✅ 2025-08-26
- [x] Response time < 5 minutes for Java 8 Tutorial ✅ 2025-08-26

## Phase A3: Docker Container

### Objectives
- [x] Create optimized Docker image ✅ 2025-08-26
- [x] Pre-cache Java 11→17 migration artifacts ✅ 2025-08-26
- [x] Ensure container runs locally ✅ 2025-08-26
- [x] Keep container size under 1GB ✅ 2025-08-26

### A3.1: Dockerfile

```dockerfile
FROM golang:1.21-alpine AS go-builder
WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o openrewrite-service cmd/server/main.go

FROM maven:3.9-eclipse-temurin-17 AS maven-cache
# Pre-download OpenRewrite artifacts
RUN mvn dependency:get \
    -DgroupId=org.openrewrite.recipe \
    -DartifactId=rewrite-migrate-java \
    -Dversion=3.15.0 \
    -Dtransitive=true

FROM eclipse-temurin:17-jdk-alpine
RUN apk add --no-cache git maven gradle

WORKDIR /app
COPY --from=go-builder /build/openrewrite-service .
COPY --from=maven-cache /root/.m2 /root/.m2

EXPOSE 8090
CMD ["./openrewrite-service"]
```

### A3.2: Build & Test
```bash
# Build
docker build -t openrewrite-service:mvp .

# Test locally
docker run -p 8090:8090 openrewrite-service:mvp

# Test transformation
curl -X POST http://localhost:8090/transform \
  -H "Content-Type: application/json" \
  -d @test-request.json
```

### A3.3: Optimization Checklist
- [x] Multi-stage build implemented ✅ 2025-08-26
- [x] Java 11→17 artifacts cached ✅ 2025-08-26
- [x] Container size < 1GB ✅ 2025-08-26
- [x] Starts in < 30 seconds ✅ 2025-08-26
- [x] Health check passes ✅ 2025-08-26

## MVP Success Criteria

### Functional Requirements
- [x] Git initialization and diff generation working ✅ 2025-08-26
- [x] OpenRewrite Maven execution successful ✅ 2025-08-26
- [x] HTTP API accepting requests ✅ 2025-08-26
- [x] Docker container running ✅ 2025-08-26
- [x] Java 11→17 migration tested successfully ✅ 2025-08-26

### Performance Requirements
- [x] Transformation completes in < 5 minutes ✅ 2025-08-26
- [x] API responds in < 1 second ✅ 2025-08-26
- [x] Container starts in < 30 seconds ✅ 2025-08-26
- [ ] Memory usage < 2GB

### Testing Requirements
- [x] Java 8 Tutorial migrated successfully ✅ 2025-08-26
- [x] Diff is valid and applies cleanly ✅ 2025-08-26
- [ ] No memory leaks during transformation
- [x] Error cases handled gracefully ✅ 2025-08-26

## Integration Points

### With Stream B (Optional for MVP)
- Can add Consul for job status later
- Can add SeaweedFS for diff storage later
- Can add job queue for async processing later

### With Stream C (After MVP)
- Add monitoring endpoints
- Add metrics collection
- Add production logging

## Troubleshooting Guide

### Common Issues

1. **Maven fails to download artifacts**
   - Check internet connectivity
   - Verify Maven settings
   - Use local repository mirror

2. **Git commands fail**
   - Ensure git is installed in container
   - Check file permissions
   - Verify working directory exists

3. **Container too large**
   - Use Alpine base images
   - Clean Maven cache after download
   - Remove unnecessary tools

4. **Transformation timeout**
   - Increase context timeout
   - Check for infinite loops in recipes
   - Monitor memory usage

## Next Steps After MVP
1. Add async processing (Stream B integration)
2. Implement persistent storage (Consul/SeaweedFS)
3. Add monitoring and metrics (Stream C)
4. Deploy to Nomad (Stream B completion)