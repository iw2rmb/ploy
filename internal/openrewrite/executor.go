package openrewrite

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// ExecutorImpl implements the Executor interface
type ExecutorImpl struct {
	config     *Config
	gitManager GitManager
}

// NewExecutor creates a new Executor instance
func NewExecutor(config *Config) Executor {
	return &ExecutorImpl{
		config:     config,
		gitManager: NewGitManager(config),
	}
}

// Execute runs an OpenRewrite transformation on the provided source code
func (e *ExecutorImpl) Execute(ctx context.Context, jobID string, tarData []byte, recipe RecipeConfig) (*TransformResult, error) {
	startTime := time.Now()
	result := &TransformResult{
		Success: false,
	}

	// Initialize Git repository
	repoPath, err := e.gitManager.InitializeRepo(ctx, jobID, tarData)
	if err != nil {
		result.Error = fmt.Sprintf("failed to initialize repository: %v", err)
		return result, err
	}
	defer e.cleanup(repoPath)

	// Detect build system
	buildSystem := e.DetectBuildSystem(repoPath)
	result.BuildSystem = string(buildSystem)
	
	if buildSystem == BuildSystemNone {
		result.Error = "no supported build system found"
		return result, fmt.Errorf("no supported build system found")
	}

	// Detect Java version
	javaVersion, err := e.DetectJavaVersion(repoPath)
	if err != nil {
		// Use default Java version
		javaVersion = Java17
	}
	result.JavaVersion = string(javaVersion)

	// Execute transformation based on build system
	var execErr error
	switch buildSystem {
	case BuildSystemMaven:
		execErr = e.executeMaven(ctx, repoPath, recipe)
	case BuildSystemGradle:
		execErr = e.executeGradle(ctx, repoPath, recipe)
	default:
		execErr = fmt.Errorf("unsupported build system: %s", buildSystem)
	}

	if execErr != nil {
		// Check if context was cancelled
		if ctx.Err() != nil {
			result.Error = fmt.Sprintf("context cancelled: %v", ctx.Err())
			return result, ctx.Err()
		}
		result.Error = fmt.Sprintf("transformation failed: %v", execErr)
		return result, execErr
	}

	// Generate diff
	diff, err := e.gitManager.GenerateDiff(ctx, repoPath)
	if err != nil {
		result.Error = fmt.Sprintf("failed to generate diff: %v", err)
		return result, err
	}

	// Set success result
	result.Success = true
	result.Diff = diff
	result.Duration = time.Since(startTime)
	result.Error = ""

	return result, nil
}

// DetectBuildSystem identifies the build system used in the source directory
func (e *ExecutorImpl) DetectBuildSystem(srcPath string) BuildSystem {
	// Check for Maven (pom.xml)
	pomPath := filepath.Join(srcPath, "pom.xml")
	if _, err := os.Stat(pomPath); err == nil {
		return BuildSystemMaven
	}

	// Check for Gradle (build.gradle or build.gradle.kts)
	gradlePath := filepath.Join(srcPath, "build.gradle")
	if _, err := os.Stat(gradlePath); err == nil {
		return BuildSystemGradle
	}

	gradleKtsPath := filepath.Join(srcPath, "build.gradle.kts")
	if _, err := os.Stat(gradleKtsPath); err == nil {
		return BuildSystemGradle
	}

	return BuildSystemNone
}

// DetectJavaVersion identifies the Java version from the source directory
func (e *ExecutorImpl) DetectJavaVersion(srcPath string) (JavaVersion, error) {
	// Check .java-version file first
	javaVersionFile := filepath.Join(srcPath, ".java-version")
	if data, err := os.ReadFile(javaVersionFile); err == nil {
		version := strings.TrimSpace(string(data))
		return e.normalizeJavaVersion(version), nil
	}

	// Check build system configuration
	buildSystem := e.DetectBuildSystem(srcPath)
	
	switch buildSystem {
	case BuildSystemMaven:
		return e.detectJavaVersionFromMaven(srcPath)
	case BuildSystemGradle:
		return e.detectJavaVersionFromGradle(srcPath)
	}

	// Default to Java 17
	return Java17, nil
}

// detectJavaVersionFromMaven extracts Java version from pom.xml
func (e *ExecutorImpl) detectJavaVersionFromMaven(srcPath string) (JavaVersion, error) {
	pomPath := filepath.Join(srcPath, "pom.xml")
	data, err := os.ReadFile(pomPath)
	if err != nil {
		return Java17, err
	}

	content := string(data)
	
	// Look for maven.compiler.source or maven.compiler.target
	patterns := []string{
		`<maven\.compiler\.source>(\d+)</maven\.compiler\.source>`,
		`<maven\.compiler\.target>(\d+)</maven\.compiler\.target>`,
		`<java\.version>(\d+)</java\.version>`,
	}

	for _, pattern := range patterns {
		re := regexp.MustCompile(pattern)
		if matches := re.FindStringSubmatch(content); len(matches) > 1 {
			return e.normalizeJavaVersion(matches[1]), nil
		}
	}

	// Default to Java 17 if not found
	return Java17, nil
}

// detectJavaVersionFromGradle extracts Java version from build.gradle
func (e *ExecutorImpl) detectJavaVersionFromGradle(srcPath string) (JavaVersion, error) {
	// Try build.gradle first
	gradlePath := filepath.Join(srcPath, "build.gradle")
	if _, err := os.Stat(gradlePath); err != nil {
		// Try build.gradle.kts
		gradlePath = filepath.Join(srcPath, "build.gradle.kts")
	}

	data, err := os.ReadFile(gradlePath)
	if err != nil {
		return Java17, err
	}

	content := string(data)
	
	// Look for sourceCompatibility or targetCompatibility
	patterns := []string{
		`VERSION_(\d+)`,
		`sourceCompatibility\s*=\s*["']?(\d+)`,
		`targetCompatibility\s*=\s*["']?(\d+)`,
		`JavaVersion\.VERSION_(\d+)`,
	}

	for _, pattern := range patterns {
		re := regexp.MustCompile(pattern)
		if matches := re.FindStringSubmatch(content); len(matches) > 1 {
			return e.normalizeJavaVersion(matches[1]), nil
		}
	}

	// Default to Java 17 if not found
	return Java17, nil
}

// normalizeJavaVersion converts various Java version formats to our standard
func (e *ExecutorImpl) normalizeJavaVersion(version string) JavaVersion {
	// Clean up version string
	version = strings.TrimSpace(version)
	
	// Handle 1.x format (e.g., 1.8 -> 8)
	if strings.HasPrefix(version, "1.") {
		version = strings.TrimPrefix(version, "1.")
	}
	
	// Extract major version number
	parts := strings.Split(version, ".")
	if len(parts) > 0 {
		version = parts[0]
	}

	// Map to our supported versions
	switch version {
	case "8":
		return Java11 // Upgrade from Java 8 to 11
	case "11":
		return Java11
	case "17":
		return Java17
	case "21":
		return Java21
	default:
		// For any version >= 21, use Java 21
		// For versions between 11 and 17, use Java 17
		// For anything else, default to Java 17
		return Java17
	}
}

// executeMaven runs OpenRewrite using Maven
func (e *ExecutorImpl) executeMaven(ctx context.Context, repoPath string, recipe RecipeConfig) error {
	// Create rewrite.yml
	rewriteYaml := e.generateRewriteYaml(recipe)
	yamlPath := filepath.Join(repoPath, "rewrite.yml")
	if err := os.WriteFile(yamlPath, []byte(rewriteYaml), 0644); err != nil {
		return fmt.Errorf("failed to create rewrite.yml: %w", err)
	}

	// Build Maven command arguments
	args := e.buildMavenCommand(recipe)
	
	// Execute Maven
	cmd := exec.CommandContext(ctx, e.config.MavenPath, args...)
	cmd.Dir = repoPath
	
	// Set JAVA_HOME if configured
	if e.config.JavaHome != "" {
		cmd.Env = append(os.Environ(), fmt.Sprintf("JAVA_HOME=%s", e.config.JavaHome))
	}

	// Run the command
	output, err := cmd.CombinedOutput()
	if err != nil {
		// Check if context was cancelled
		if ctx.Err() != nil {
			return fmt.Errorf("context cancelled: %w", ctx.Err())
		}
		return fmt.Errorf("maven execution failed: %w\nOutput: %s", err, string(output))
	}

	return nil
}

// executeGradle runs OpenRewrite using Gradle
func (e *ExecutorImpl) executeGradle(ctx context.Context, repoPath string, recipe RecipeConfig) error {
	// Create rewrite.yml
	rewriteYaml := e.generateRewriteYaml(recipe)
	yamlPath := filepath.Join(repoPath, "rewrite.yml")
	if err := os.WriteFile(yamlPath, []byte(rewriteYaml), 0644); err != nil {
		return fmt.Errorf("failed to create rewrite.yml: %w", err)
	}

	// Add OpenRewrite plugin to build.gradle if needed
	if err := e.addGradlePlugin(repoPath); err != nil {
		return fmt.Errorf("failed to add Gradle plugin: %w", err)
	}

	// Build Gradle command
	args := []string{
		"rewriteRun",
		"--no-daemon",
	}

	// Execute Gradle
	cmd := exec.CommandContext(ctx, e.config.GradlePath, args...)
	cmd.Dir = repoPath
	
	// Set JAVA_HOME if configured
	if e.config.JavaHome != "" {
		cmd.Env = append(os.Environ(), fmt.Sprintf("JAVA_HOME=%s", e.config.JavaHome))
	}

	// Run the command
	output, err := cmd.CombinedOutput()
	if err != nil {
		// Check if context was cancelled
		if ctx.Err() != nil {
			return fmt.Errorf("context cancelled: %w", ctx.Err())
		}
		return fmt.Errorf("gradle execution failed: %w\nOutput: %s", err, string(output))
	}

	return nil
}

// generateRewriteYaml creates the rewrite.yml configuration file
func (e *ExecutorImpl) generateRewriteYaml(recipe RecipeConfig) string {
	yaml := `---
type: specs.openrewrite.org/v1beta/recipe
name: PloyTransformation
recipeList:
  - %s
`
	return fmt.Sprintf(yaml, recipe.Recipe)
}

// buildMavenCommand builds the Maven command arguments for OpenRewrite
func (e *ExecutorImpl) buildMavenCommand(recipe RecipeConfig) []string {
	args := []string{
		"org.openrewrite.maven:rewrite-maven-plugin:5.34.0:run",
		"-Drewrite.activeRecipes=PloyTransformation",
	}

	// Add recipe artifacts if specified
	if recipe.Artifacts != "" {
		args = append(args, fmt.Sprintf("-Drewrite.recipeArtifactCoordinates=%s", recipe.Artifacts))
	}

	// Add any additional options
	for key, value := range recipe.Options {
		args = append(args, fmt.Sprintf("-D%s=%s", key, value))
	}

	return args
}

// addGradlePlugin adds the OpenRewrite plugin to build.gradle if not present
func (e *ExecutorImpl) addGradlePlugin(repoPath string) error {
	gradlePath := filepath.Join(repoPath, "build.gradle")
	
	// Check if build.gradle exists
	if _, err := os.Stat(gradlePath); os.IsNotExist(err) {
		// Try build.gradle.kts
		gradlePath = filepath.Join(repoPath, "build.gradle.kts")
		if _, err := os.Stat(gradlePath); os.IsNotExist(err) {
			return fmt.Errorf("no Gradle build file found")
		}
	}

	// Read current content
	content, err := os.ReadFile(gradlePath)
	if err != nil {
		return err
	}

	contentStr := string(content)
	
	// Check if OpenRewrite plugin is already present
	if strings.Contains(contentStr, "org.openrewrite.rewrite") {
		return nil // Already has the plugin
	}

	// Add plugin to plugins block
	if strings.Contains(contentStr, "plugins {") {
		// Insert into existing plugins block
		contentStr = strings.Replace(contentStr, "plugins {", 
			`plugins {
    id 'org.openrewrite.rewrite' version '6.16.0'`, 1)
	} else {
		// Add new plugins block at the beginning
		contentStr = `plugins {
    id 'org.openrewrite.rewrite' version '6.16.0'
}

` + contentStr
	}

	// Write back
	return os.WriteFile(gradlePath, []byte(contentStr), 0644)
}

// cleanup removes temporary files and directories
func (e *ExecutorImpl) cleanup(repoPath string) {
	if err := e.gitManager.Cleanup(repoPath); err != nil {
		fmt.Printf("Warning: failed to cleanup repository %s: %v\n", repoPath, err)
	}
}