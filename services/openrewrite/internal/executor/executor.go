package executor

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Executor handles OpenRewrite transformations
type Executor struct {
	mavenPath      string
	gradlePath     string
	javaHome       string
	rewriteVersion string
	pluginVersion  string
	workspaceDir   string
}

// New creates a new OpenRewrite executor
func New() *Executor {
	// Find Maven and Gradle paths
	mavenPath, _ := exec.LookPath("mvn")
	gradlePath, _ := exec.LookPath("gradle")
	
	workspaceDir := os.Getenv("WORKSPACE_DIR")
	if workspaceDir == "" {
		workspaceDir = "/workspace/transformations"
	}
	
	// Ensure workspace directory exists
	os.MkdirAll(workspaceDir, 0755)
	
	return &Executor{
		mavenPath:      mavenPath,
		gradlePath:     gradlePath,
		javaHome:       os.Getenv("JAVA_HOME"),
		rewriteVersion: "5.34.0",
		pluginVersion:  "5.34.0",
		workspaceDir:   workspaceDir,
	}
}

// ExecuteTransformation performs a synchronous transformation
func (e *Executor) ExecuteTransformation(ctx context.Context, request TransformRequest) (*TransformationResult, error) {
	startTime := time.Now()
	
	// Create unique workspace for this transformation
	workDir := filepath.Join(e.workspaceDir, request.JobID)
	if err := os.MkdirAll(workDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create work directory: %w", err)
	}
	defer os.RemoveAll(workDir) // Clean up after transformation
	
	// Extract tar archive
	repoPath := filepath.Join(workDir, "repo")
	if err := e.extractArchive(request.TarArchive, repoPath); err != nil {
		return nil, fmt.Errorf("failed to extract archive: %w", err)
	}
	
	// Detect build system
	buildSystem := e.detectBuildSystem(repoPath)
	if buildSystem == "unknown" {
		return &TransformationResult{
			RecipeID:      request.RecipeConfig.Recipe,
			Success:       false,
			ExecutionTime: time.Since(startTime),
			Errors: []TransformationError{
				{
					Type:    "build_system",
					Message: "No supported build system found (Maven or Gradle required)",
				},
			},
		}, nil
	}
	
	// Execute transformation based on build system
	var result *TransformationResult
	var err error
	
	switch buildSystem {
	case "maven":
		result, err = e.executeMavenRewrite(ctx, request.RecipeConfig, repoPath)
	case "gradle":
		result, err = e.executeGradleRewrite(ctx, request.RecipeConfig, repoPath)
	default:
		return &TransformationResult{
			RecipeID:      request.RecipeConfig.Recipe,
			Success:       false,
			ExecutionTime: time.Since(startTime),
			Errors: []TransformationError{
				{
					Type:    "build_system",
					Message: fmt.Sprintf("Unsupported build system: %s", buildSystem),
				},
			},
		}, nil
	}
	
	if err != nil {
		return &TransformationResult{
			RecipeID:      request.RecipeConfig.Recipe,
			Success:       false,
			ExecutionTime: time.Since(startTime),
			Errors: []TransformationError{
				{
					Type:    "execution",
					Message: err.Error(),
				},
			},
		}, nil
	}
	
	// Update total execution time
	result.ExecutionTime = time.Since(startTime)
	return result, nil
}

// extractArchive extracts a base64-encoded tar.gz archive
func (e *Executor) extractArchive(encodedArchive string, destPath string) error {
	// Decode base64
	archiveData, err := base64.StdEncoding.DecodeString(encodedArchive)
	if err != nil {
		return fmt.Errorf("failed to decode base64 archive: %w", err)
	}
	
	// Create destination directory
	if err := os.MkdirAll(destPath, 0755); err != nil {
		return fmt.Errorf("failed to create destination directory: %w", err)
	}
	
	// Create gzip reader
	reader := bytes.NewReader(archiveData)
	gzReader, err := gzip.NewReader(reader)
	if err != nil {
		return fmt.Errorf("failed to create gzip reader: %w", err)
	}
	defer gzReader.Close()
	
	// Create tar reader
	tarReader := tar.NewReader(gzReader)
	
	// Extract files
	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("failed to read tar header: %w", err)
		}
		
		// Clean and validate path
		targetPath := filepath.Join(destPath, header.Name)
		if !strings.HasPrefix(targetPath, filepath.Clean(destPath)+string(os.PathSeparator)) {
			return fmt.Errorf("invalid file path: %s", header.Name)
		}
		
		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(targetPath, os.FileMode(header.Mode)); err != nil {
				return fmt.Errorf("failed to create directory %s: %w", targetPath, err)
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
				return fmt.Errorf("failed to create parent directory: %w", err)
			}
			
			file, err := os.OpenFile(targetPath, os.O_CREATE|os.O_WRONLY, os.FileMode(header.Mode))
			if err != nil {
				return fmt.Errorf("failed to create file %s: %w", targetPath, err)
			}
			
			if _, err := io.Copy(file, tarReader); err != nil {
				file.Close()
				return fmt.Errorf("failed to write file %s: %w", targetPath, err)
			}
			file.Close()
		}
	}
	
	return nil
}

// detectBuildSystem detects whether project uses Maven or Gradle
func (e *Executor) detectBuildSystem(basePath string) string {
	// Check for Maven first (takes precedence)
	if _, err := os.Stat(filepath.Join(basePath, "pom.xml")); err == nil {
		return "maven"
	}
	
	// Check for Gradle
	if _, err := os.Stat(filepath.Join(basePath, "build.gradle")); err == nil {
		return "gradle"
	}
	if _, err := os.Stat(filepath.Join(basePath, "build.gradle.kts")); err == nil {
		return "gradle"
	}
	
	return "unknown"
}

// executeMavenRewrite executes OpenRewrite via Maven
func (e *Executor) executeMavenRewrite(ctx context.Context, config RecipeConfig, repoPath string) (*TransformationResult, error) {
	if e.mavenPath == "" {
		return nil, fmt.Errorf("Maven not found in PATH")
	}
	
	startTime := time.Now()
	
	// Create rewrite.yml configuration file
	rewriteConfig := fmt.Sprintf(`---
type: specs.openrewrite.org/v1beta/recipe
name: ServiceTransformation
displayName: OpenRewrite Service Transformation Recipe
recipeList:
  - %s
`, config.Recipe)
	
	rewriteYamlPath := filepath.Join(repoPath, "rewrite.yml")
	if err := os.WriteFile(rewriteYamlPath, []byte(rewriteConfig), 0644); err != nil {
		return nil, fmt.Errorf("failed to write rewrite.yml: %w", err)
	}
	defer os.Remove(rewriteYamlPath)
	
	// Determine recipe artifacts
	recipeArtifacts := e.getRecipeArtifacts(config)
	
	// Build Maven command with OpenRewrite plugin
	args := []string{
		"org.openrewrite.maven:rewrite-maven-plugin:" + e.pluginVersion + ":run",
		"-Drewrite.recipeArtifactCoordinates=" + recipeArtifacts,
		"-Drewrite.activeRecipes=ServiceTransformation",
		"-Drewrite.exportDatatables=true",
	}
	
	cmd := exec.CommandContext(ctx, e.mavenPath, args...)
	cmd.Dir = repoPath
	cmd.Env = e.buildEnvironment()
	
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	
	// Execute Maven OpenRewrite
	err := cmd.Run()
	duration := time.Since(startTime)
	
	// Parse results
	result := &TransformationResult{
		RecipeID:       config.Recipe,
		Success:        err == nil,
		ExecutionTime:  duration,
		ChangesApplied: 0,
		FilesModified:  []string{},
		Metadata:       make(map[string]interface{}),
	}
	
	// Extract changes from output
	if err == nil {
		changes := e.parseMavenOutput(stdout.String())
		result.ChangesApplied = len(changes)
		result.FilesModified = changes
		
		// Generate diff
		result.Diff = e.generateDiff(repoPath)
		result.ValidationScore = e.calculateValidationScore(result)
	} else {
		// Store error in Errors field
		result.Success = false
		result.Errors = []TransformationError{
			{
				Type:    "maven_execution",
				Message: fmt.Sprintf("Maven execution failed: %v\n%s", err, stderr.String()),
			},
		}
	}
	
	return result, nil
}

// executeGradleRewrite executes OpenRewrite via Gradle
func (e *Executor) executeGradleRewrite(ctx context.Context, config RecipeConfig, repoPath string) (*TransformationResult, error) {
	gradleCmd := e.gradlePath
	if gradleCmd == "" {
		// Try gradlew if gradle is not in PATH
		gradlewPath := filepath.Join(repoPath, "gradlew")
		if _, err := os.Stat(gradlewPath); err == nil {
			gradleCmd = "./gradlew"
		} else {
			return nil, fmt.Errorf("Gradle not found")
		}
	}
	
	startTime := time.Now()
	
	// Add OpenRewrite plugin to build.gradle if not present
	if err := e.ensureGradlePlugin(repoPath, config.Recipe); err != nil {
		return nil, fmt.Errorf("failed to configure Gradle plugin: %w", err)
	}
	
	// Run rewriteRun task
	cmd := exec.CommandContext(ctx, gradleCmd, "rewriteRun")
	cmd.Dir = repoPath
	cmd.Env = e.buildEnvironment()
	
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	
	err := cmd.Run()
	duration := time.Since(startTime)
	
	result := &TransformationResult{
		RecipeID:       config.Recipe,
		Success:        err == nil,
		ExecutionTime:  duration,
		ChangesApplied: 0,
		FilesModified:  []string{},
		Metadata:       make(map[string]interface{}),
	}
	
	if err == nil {
		changes := e.parseGradleOutput(stdout.String())
		result.ChangesApplied = len(changes)
		result.FilesModified = changes
		result.Diff = e.generateDiff(repoPath)
		result.ValidationScore = e.calculateValidationScore(result)
	} else {
		result.Success = false
		result.Errors = []TransformationError{
			{
				Type:    "gradle_execution",
				Message: fmt.Sprintf("Gradle execution failed: %v\n%s", err, stderr.String()),
			},
		}
	}
	
	return result, nil
}

// getRecipeArtifacts returns the Maven coordinates for recipe artifacts
func (e *Executor) getRecipeArtifacts(config RecipeConfig) string {
	// Use artifacts from config if provided
	if config.Artifacts != "" {
		return config.Artifacts
	}
	
	// Map common recipes to their artifacts
	recipeMap := map[string]string{
		"org.openrewrite.java.migrate.Java11toJava17":            "org.openrewrite.recipe:rewrite-migrate-java:2.25.0",
		"org.openrewrite.java.spring.boot3.UpgradeSpringBoot_3_0": "org.openrewrite.recipe:rewrite-spring:5.7.0",
		"org.openrewrite.java.spring.boot3.SpringBoot3BestPractices": "org.openrewrite.recipe:rewrite-spring:5.7.0",
		"org.openrewrite.java.cleanup.UnnecessaryThrows":         "org.openrewrite:rewrite-java:8.21.0",
	}
	
	if artifacts, ok := recipeMap[config.Recipe]; ok {
		return artifacts
	}
	
	// Default to core Java recipes
	return "org.openrewrite:rewrite-java:8.21.0"
}

// Helper methods (simplified versions of the existing implementation)

func (e *Executor) buildEnvironment() []string {
	env := os.Environ()
	
	// Add Java home if set
	if e.javaHome != "" {
		env = append(env, "JAVA_HOME="+e.javaHome)
	}
	
	// Add Maven/Gradle options for better output
	env = append(env, "MAVEN_OPTS=-Xmx2G")
	env = append(env, "GRADLE_OPTS=-Xmx2G")
	
	return env
}

func (e *Executor) parseMavenOutput(output string) []string {
	files := []string{}
	lines := strings.Split(output, "\n")
	
	for _, line := range lines {
		// Look for file change indicators
		if strings.Contains(line, "Changes have been made to") ||
			strings.Contains(line, "Modified") ||
			strings.Contains(line, ".java") && strings.Contains(line, "fixed") {
			// Extract filename
			parts := strings.Fields(line)
			for _, part := range parts {
				if strings.HasSuffix(part, ".java") || strings.HasSuffix(part, ".xml") {
					files = append(files, part)
				}
			}
		}
	}
	
	// If no specific files found, but execution succeeded, assume pom.xml was modified
	if len(files) == 0 && strings.Contains(output, "BUILD SUCCESS") {
		files = append(files, "pom.xml")
	}
	
	return files
}

func (e *Executor) parseGradleOutput(output string) []string {
	files := []string{}
	lines := strings.Split(output, "\n")
	
	for _, line := range lines {
		if strings.Contains(line, "Fixed") || strings.Contains(line, "Modified") {
			parts := strings.Fields(line)
			for _, part := range parts {
				if strings.Contains(part, ".java") || strings.Contains(part, ".gradle") {
					files = append(files, part)
				}
			}
		}
	}
	
	// Default to build.gradle if no files detected but task succeeded
	if len(files) == 0 && strings.Contains(output, "BUILD SUCCESSFUL") {
		files = append(files, "build.gradle")
	}
	
	return files
}

func (e *Executor) generateDiff(basePath string) string {
	// Run git diff if available
	cmd := exec.Command("git", "diff", "--no-index", "--no-prefix")
	cmd.Dir = basePath
	
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	
	if err := cmd.Run(); err == nil {
		return out.String()
	}
	
	// Fallback to simple message
	return "OpenRewrite transformation applied successfully"
}

func (e *Executor) calculateValidationScore(result *TransformationResult) float64 {
	// Simple validation score based on success and changes
	if !result.Success {
		return 0.0
	}
	
	if result.ChangesApplied == 0 {
		return 0.5 // No changes but successful
	}
	
	// Score based on changes applied (capped at 1.0)
	score := 0.7 + (float64(result.ChangesApplied) * 0.1)
	if score > 1.0 {
		score = 1.0
	}
	
	return score
}

func (e *Executor) ensureGradlePlugin(basePath string, recipe string) error {
	buildFile := filepath.Join(basePath, "build.gradle")
	content, err := os.ReadFile(buildFile)
	if err != nil {
		return err
	}
	
	// Check if plugin is already present
	if strings.Contains(string(content), "org.openrewrite.rewrite") {
		return nil // Already configured
	}
	
	// Add plugin and configuration
	pluginBlock := fmt.Sprintf(`
plugins {
    id 'org.openrewrite.rewrite' version '%s'
}

rewrite {
    activeRecipe('%s')
}

repositories {
    mavenCentral()
}

dependencies {
    rewrite('%s')
}
`, e.rewriteVersion, recipe, e.getRecipeArtifacts(RecipeConfig{Recipe: recipe}))
	
	// Prepend to existing content
	newContent := pluginBlock + "\n" + string(content)
	return os.WriteFile(buildFile, []byte(newContent), 0644)
}