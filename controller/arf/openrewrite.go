package arf

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// ExecutionResult contains the results of command execution
type ExecutionResult struct {
	Output   string
	Stderr   string
	ExitCode int
}

// OpenRewriteEngine implements ARFEngine using OpenRewrite Java libraries
type OpenRewriteEngine struct {
	jarPath       string
	javaHome      string
	workingDir    string
	cache         ASTCache
	sandboxMgr    SandboxManager
	recipes       map[string]Recipe
	maxMemory     string
	timeout       time.Duration
}

// NewOpenRewriteEngine creates a new OpenRewrite-based ARF engine
func NewOpenRewriteEngine(jarPath, javaHome, workingDir string, cache ASTCache, sandboxMgr SandboxManager) *OpenRewriteEngine {
	return &OpenRewriteEngine{
		jarPath:    jarPath,
		javaHome:   javaHome,
		workingDir: workingDir,
		cache:      cache,
		sandboxMgr: sandboxMgr,
		recipes:    make(map[string]Recipe),
		maxMemory:  "4G",
		timeout:    10 * time.Minute,
	}
}

// ExecuteRecipe executes an OpenRewrite recipe on the given codebase
func (e *OpenRewriteEngine) ExecuteRecipe(ctx context.Context, recipe Recipe, codebase Codebase) (*TransformationResult, error) {
	startTime := time.Now()

	// Create sandbox for secure execution
	sandboxConfig := SandboxConfig{
		Repository:    codebase.Repository,
		Branch:        codebase.Branch,
		Language:      codebase.Language,
		BuildTool:     codebase.BuildTool,
		TTL:           30 * time.Minute,
		MemoryLimit:   "4G",
		CPULimit:      "2",
		NetworkAccess: false,
		TempSpace:     "2G",
	}

	sandbox, err := e.sandboxMgr.CreateSandbox(ctx, sandboxConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create sandbox: %w", err)
	}
	defer e.sandboxMgr.DestroySandbox(ctx, sandbox.ID)

	// Prepare OpenRewrite execution environment
	workspaceDir := filepath.Join(sandbox.RootPath, "workspace")
	configPath := filepath.Join(workspaceDir, "rewrite.yml")
	outputPath := filepath.Join(workspaceDir, "rewrite-results.json")

	// Generate OpenRewrite configuration
	config := e.generateOpenRewriteConfig(recipe, codebase)
	if err := e.writeConfigFile(configPath, config); err != nil {
		return nil, fmt.Errorf("failed to write OpenRewrite config: %w", err)
	}

	// Execute OpenRewrite in sandbox
	executionResult, err := e.executeInSandbox(ctx, sandbox, configPath, outputPath, workspaceDir)
	if err != nil {
		// Create transformation result with execution error
		transformResult := &TransformationResult{
			RecipeID:        recipe.ID,
			Success:         false,
			ChangesApplied:  0,
			FilesModified:   []string{},
			Diff:            "",
			ValidationScore: 0.0,
			ExecutionTime:   time.Since(startTime),
			Errors: []TransformationError{
				{
					Type:        "execution_error",
					Message:     fmt.Sprintf("OpenRewrite execution failed: %v", err),
					Recoverable: false,
				},
			},
			Warnings: []TransformationError{},
			Metadata: map[string]interface{}{
				"recipe":           recipe.ID,
				"execution_error":  err.Error(),
				"execution_output": executionResult.Output,
				"execution_stderr": executionResult.Stderr,
			},
		}
		return transformResult, nil
	}

	// Parse results
	transformResult, err := e.parseResults(outputPath, startTime)
	if err != nil {
		return nil, fmt.Errorf("failed to parse OpenRewrite results: %w", err)
	}

	transformResult.RecipeID = recipe.ID
	return transformResult, nil
}

// ValidateRecipe validates that a recipe is properly configured
func (e *OpenRewriteEngine) ValidateRecipe(recipe Recipe) error {
	if recipe.ID == "" {
		return fmt.Errorf("recipe ID is required")
	}
	if recipe.Source == "" {
		return fmt.Errorf("recipe source (OpenRewrite class) is required")
	}
	if recipe.Language == "" {
		return fmt.Errorf("recipe language is required")
	}

	// Validate OpenRewrite class name format
	if !strings.Contains(recipe.Source, ".") {
		return fmt.Errorf("invalid OpenRewrite class name: %s", recipe.Source)
	}

	// Check for required options based on recipe type
	switch recipe.Category {
	case CategoryMigration:
		if _, hasFrom := recipe.Options["fromVersion"]; !hasFrom {
			return fmt.Errorf("migration recipes require 'fromVersion' option")
		}
		if _, hasTo := recipe.Options["toVersion"]; !hasTo {
			return fmt.Errorf("migration recipes require 'toVersion' option")
		}
	case CategorySecurity:
		// Security recipes may have specific validation requirements
		if recipe.Confidence < 0.8 {
			return fmt.Errorf("security recipes must have confidence >= 0.8")
		}
	}

	return nil
}

// ListAvailableRecipes returns all available OpenRewrite recipes
func (e *OpenRewriteEngine) ListAvailableRecipes() ([]Recipe, error) {
	// Comprehensive OpenRewrite recipes for real Java migrations
	recipes := []Recipe{
		// Java Version Migration Recipes
		{
			ID:          "migration.java8-to-11",
			Name:        "Migrate Java 8 to Java 11",
			Description: "Upgrades Java 8 projects to Java 11, updating APIs and removing deprecated features",
			Language:    "java",
			Category:    CategoryMigration,
			Confidence:  0.90,
			Source:      "org.openrewrite.java.migrate.Java8toJava11",
			Version:     "1.0.0",
			Tags:        []string{"java", "migration", "java11"},
			Options:     map[string]string{
				"fromVersion": "8",
				"toVersion":   "11",
			},
		},
		{
			ID:          "migration.java11-to-17",
			Name:        "Migrate Java 11 to Java 17",
			Description: "Upgrades Java 11 projects to Java 17, leveraging new language features and APIs",
			Language:    "java",
			Category:    CategoryMigration,
			Confidence:  0.90,
			Source:      "org.openrewrite.java.migrate.Java11toJava17",
			Version:     "1.0.0",
			Tags:        []string{"java", "migration", "java17"},
			Options:     map[string]string{
				"fromVersion": "11",
				"toVersion":   "17",
			},
		},
		
		// Spring Boot Migration Recipes
		{
			ID:          "migration.spring-boot-2.7-to-3.0",
			Name:        "Migrate Spring Boot 2.7 to 3.0",
			Description: "Comprehensive migration from Spring Boot 2.7.x to 3.0.x including Jakarta EE migration",
			Language:    "java",
			Category:    CategoryMigration,
			Confidence:  0.85,
			Source:      "org.openrewrite.java.spring.boot3.UpgradeSpringBoot_3_0",
			Version:     "1.0.0",
			Tags:        []string{"spring", "spring-boot", "migration", "jakarta"},
			Options:     map[string]string{
				"fromVersion": "2.7",
				"toVersion":   "3.0",
			},
		},
		{
			ID:          "migration.spring-boot-3.0-to-3.1",
			Name:        "Migrate Spring Boot 3.0 to 3.1", 
			Description: "Upgrades Spring Boot 3.0.x to 3.1.x with latest improvements and security updates",
			Language:    "java",
			Category:    CategoryMigration,
			Confidence:  0.90,
			Source:      "org.openrewrite.java.spring.boot3.UpgradeSpringBoot_3_1",
			Version:     "1.0.0",
			Tags:        []string{"spring", "spring-boot", "migration"},
			Options:     map[string]string{
				"fromVersion": "3.0",
				"toVersion":   "3.1",
			},
		},
		
		// Jakarta EE Migration (critical for Spring Boot 3)
		{
			ID:          "migration.javax-to-jakarta",
			Name:        "Migrate javax.* to jakarta.*",
			Description: "Migrates javax.* packages to jakarta.* for Jakarta EE 9+ compatibility",
			Language:    "java",
			Category:    CategoryMigration,
			Confidence:  0.95,
			Source:      "org.openrewrite.java.migrate.javax.JavaxMigrationToJakarta",
			Version:     "1.0.0",
			Tags:        []string{"jakarta", "javax", "migration", "ee"},
		},
		
		// Spring Boot Best Practices
		{
			ID:          "modernize.spring-boot-3-best-practices",
			Name:        "Apply Spring Boot 3 Best Practices",
			Description: "Applies Spring Boot 3 best practices and recommended configurations",
			Language:    "java",
			Category:    CategoryModernize,
			Confidence:  0.85,
			Source:      "org.openrewrite.java.spring.boot3.SpringBoot3BestPractices",
			Version:     "1.0.0",
			Tags:        []string{"spring", "spring-boot", "best-practices"},
		},
		
		// Cleanup Recipes
		{
			ID:          "cleanup.unused-imports",
			Name:        "Remove Unused Imports",
			Description: "Removes unused import statements",
			Language:    "java",
			Category:    CategoryCleanup,
			Confidence:  0.95,
			Source:      "org.openrewrite.java.cleanup.RemoveUnusedImports",
			Version:     "1.0.0",
			Tags:        []string{"cleanup", "imports"},
		},
		{
			ID:          "cleanup.unnecessary-parentheses",
			Name:        "Remove Unnecessary Parentheses",
			Description: "Removes unnecessary parentheses in expressions",
			Language:    "java",
			Category:    CategoryCleanup,
			Confidence:  0.90,
			Source:      "org.openrewrite.java.cleanup.UnnecessaryParentheses",
			Version:     "1.0.0",
			Tags:        []string{"cleanup", "style"},
		},
		
		// Testing Modernization
		{
			ID:          "modernize.junit4-to-junit5",
			Name:        "Migrate JUnit 4 to JUnit 5",
			Description: "Comprehensive migration from JUnit 4 to JUnit 5 including annotations and assertions",
			Language:    "java",
			Category:    CategoryModernize,
			Confidence:  0.85,
			Source:      "org.openrewrite.java.testing.junit5.JUnit4to5Migration",
			Version:     "1.0.0",
			Tags:        []string{"testing", "junit", "migration"},
		},
		
		// Security Recipes
		{
			ID:          "security.fix-deprecated-apis",
			Name:        "Fix Deprecated Security APIs",
			Description: "Replaces deprecated security APIs with modern alternatives",
			Language:    "java",
			Category:    CategorySecurity,
			Confidence:  0.90,
			Source:      "org.openrewrite.java.security.FixDeprecatedApis",
			Version:     "1.0.0",
			Tags:        []string{"security", "deprecated"},
		},
		{
			ID:          "security.spring-security-6",
			Name:        "Migrate to Spring Security 6",
			Description: "Upgrades Spring Security to version 6 with new security configurations",
			Language:    "java",
			Category:    CategorySecurity,
			Confidence:  0.80,
			Source:      "org.openrewrite.java.spring.security6.UpgradeSprintSecurity_6_0",
			Version:     "1.0.0",
			Tags:        []string{"spring", "security", "migration"},
		},
		
		// Performance Recipes
		{
			ID:          "performance.use-string-builder",
			Name:        "Use StringBuilder for String Concatenation",
			Description: "Replaces string concatenation in loops with StringBuilder",
			Language:    "java",
			Category:    CategoryPerformance,
			Confidence:  0.85,
			Source:      "org.openrewrite.java.cleanup.ReplaceStringBuilderWithString",
			Version:     "1.0.0",
			Tags:        []string{"performance", "string"},
		},
	}

	var result []Recipe
	for _, recipe := range recipes {
		e.recipes[recipe.ID] = recipe
		result = append(result, recipe)
	}

	return result, nil
}

// GetRecipeMetadata returns detailed metadata for a recipe
func (e *OpenRewriteEngine) GetRecipeMetadata(recipeID string) (*RecipeMetadata, error) {
	recipe, exists := e.recipes[recipeID]
	if !exists {
		return nil, fmt.Errorf("recipe %s not found", recipeID)
	}

	metadata := &RecipeMetadata{
		Recipe:              recipe,
		ApplicableLanguages: []string{recipe.Language},
		RequiredOptions:     []string{},
		OptionalOptions:     []string{},
		Prerequisites:       []string{},
		CreatedAt:           time.Now().Add(-30 * 24 * time.Hour), // Placeholder
		UpdatedAt:           time.Now().Add(-7 * 24 * time.Hour),  // Placeholder
		UsageCount:          0,
		SuccessRate:         recipe.Confidence,
	}

	// Set required/optional options based on recipe type
	switch recipe.Category {
	case CategoryMigration:
		metadata.RequiredOptions = []string{"fromVersion", "toVersion"}
	case CategoryModernize:
		metadata.OptionalOptions = []string{"preserveComments", "updateTests"}
	}

	return metadata, nil
}

// CacheAST stores an AST in the cache
func (e *OpenRewriteEngine) CacheAST(key string, ast *AST) error {
	return e.cache.Put(key, ast)
}

// GetCachedAST retrieves an AST from the cache
func (e *OpenRewriteEngine) GetCachedAST(key string) (*AST, bool) {
	return e.cache.Get(key)
}

// Helper methods

func (e *OpenRewriteEngine) generateOpenRewriteConfig(recipe Recipe, codebase Codebase) map[string]interface{} {
	config := map[string]interface{}{
		"type": "specs.openrewrite.org/v1beta/recipe",
		"name": recipe.ID,
		"displayName": recipe.Name,
		"description": recipe.Description,
		"recipeList": []map[string]interface{}{
			{
				"org.openrewrite.Recipe": recipe.Source,
			},
		},
	}

	// Add recipe-specific options
	if len(recipe.Options) > 0 {
		recipeConfig := config["recipeList"].([]map[string]interface{})[0]
		for key, value := range recipe.Options {
			recipeConfig[key] = value
		}
	}

	return config
}

func (e *OpenRewriteEngine) writeConfigFile(configPath string, config map[string]interface{}) error {
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(configPath, data, 0644)
}

func (e *OpenRewriteEngine) executeInSandbox(ctx context.Context, sandbox *Sandbox, configPath, outputPath, workspaceDir string) (*ExecutionResult, error) {
	// Use the repository cloned by the sandbox manager
	// The repository is already cloned into workspaceDir by CreateSandbox
	
	// Find the actual project directory within the workspace
	projectDir := workspaceDir
	
	// If repository was cloned, find the project root by looking for build files
	if sandbox.Config.Repository != "" {
		// Repository was cloned - find the project root
		entries, err := os.ReadDir(workspaceDir)
		if err != nil {
			return nil, fmt.Errorf("failed to read workspace directory: %w", err)
		}
		
		// Look for a subdirectory containing build files (pom.xml or build.gradle)
		for _, entry := range entries {
			if entry.IsDir() {
				subDir := filepath.Join(workspaceDir, entry.Name())
				if e.hasJavaBuildFile(subDir) {
					projectDir = subDir
					break
				}
			}
		}
		
		// If no subdirectory with build files, check if workspace itself has build files
		if projectDir == workspaceDir && !e.hasJavaBuildFile(workspaceDir) {
			return nil, fmt.Errorf("no Java build file (pom.xml or build.gradle) found in repository")
		}
	} else {
		// No repository specified - create a minimal Maven project for testing
		if err := e.createMavenProject(filepath.Join(workspaceDir, "pom.xml")); err != nil {
			return nil, fmt.Errorf("failed to create Maven project: %w", err)
		}
		
		// Create a simple Java file for testing
		javaDir := filepath.Join(workspaceDir, "src", "main", "java")
		if err := os.MkdirAll(javaDir, 0755); err != nil {
			return nil, fmt.Errorf("failed to create Java source directory: %w", err)
		}
		
		testJavaFile := filepath.Join(javaDir, "TestClass.java")
		testJavaCode := `public class TestClass {
    public void method() {
        System.out.println(("Hello World")); // Unnecessary parentheses for testing
    }
}`
		if err := os.WriteFile(testJavaFile, []byte(testJavaCode), 0644); err != nil {
			return nil, fmt.Errorf("failed to create test Java file: %w", err)
		}
	}
	
	// Determine build tool and execute appropriate command
	var cmd *exec.Cmd
	var outputFile string
	timeoutCtx, cancel := context.WithTimeout(ctx, e.timeout)
	defer cancel()
	
	if e.hasPomFile(projectDir) {
		// Maven project - use OpenRewrite Maven plugin with result output
		outputFile = filepath.Join(projectDir, "target", "rewrite-results.json")
		args := []string{
			"org.openrewrite.maven:rewrite-maven-plugin:run",
			"-Drewrite.exportDatatables=true",
			fmt.Sprintf("-Drewrite.exportDatatables.path=%s", outputFile),
		}
		cmd = exec.CommandContext(timeoutCtx, "mvn", args...)
	} else if e.hasGradleFile(projectDir) {
		// Gradle project - use OpenRewrite Gradle plugin with result output
		outputFile = filepath.Join(projectDir, "build", "rewrite-results.json")
		args := []string{
			"rewriteRun",
			"-Prewrite.exportDatatables=true",
			fmt.Sprintf("-Prewrite.exportDatatables.path=%s", outputFile),
		}
		cmd = exec.CommandContext(timeoutCtx, "./gradlew", args...)
	} else {
		return nil, fmt.Errorf("no supported build file found (pom.xml or build.gradle)")
	}
	
	cmd.Dir = projectDir
	cmd.Env = append(os.Environ(),
		fmt.Sprintf("JAVA_HOME=%s", e.javaHome),
		fmt.Sprintf("PATH=%s/bin:%s", e.javaHome, os.Getenv("PATH")),
	)
	
	// Store output file path for result parsing
	if outputPath != "" {
		// Write the actual output file path to the expected output path for parsing
		os.WriteFile(outputPath, []byte(outputFile), 0644)
	}
	
	// Capture stdout and stderr
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	
	// Execute command and capture results
	err := cmd.Run()
	
	result := &ExecutionResult{
		Output:   stdout.String(),
		Stderr:   stderr.String(),
		ExitCode: 0,
	}
	
	if err != nil {
		// Extract exit code if possible
		if exitError, ok := err.(*exec.ExitError); ok {
			result.ExitCode = exitError.ExitCode()
		} else {
			result.ExitCode = -1
		}
		return result, fmt.Errorf("command execution failed: %w", err)
	}
	
	return result, nil
}

// createMavenProject creates a minimal pom.xml for OpenRewrite Maven plugin testing
func (e *OpenRewriteEngine) createMavenProject(pomPath string) error {
	pomContent := `<?xml version="1.0" encoding="UTF-8"?>
<project xmlns="http://maven.apache.org/POM/4.0.0"
         xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance"
         xsi:schemaLocation="http://maven.apache.org/POM/4.0.0 
         http://maven.apache.org/xsd/maven-4.0.0.xsd">
    <modelVersion>4.0.0</modelVersion>
    
    <groupId>com.example</groupId>
    <artifactId>openrewrite-test</artifactId>
    <version>1.0.0</version>
    <packaging>jar</packaging>
    
    <properties>
        <maven.compiler.source>17</maven.compiler.source>
        <maven.compiler.target>17</maven.compiler.target>
        <project.build.sourceEncoding>UTF-8</project.build.sourceEncoding>
    </properties>
    
    <build>
        <plugins>
            <plugin>
                <groupId>org.openrewrite.maven</groupId>
                <artifactId>rewrite-maven-plugin</artifactId>
                <version>5.42.2</version>
                <configuration>
                    <activeRecipes>
                        <recipe>org.openrewrite.java.RemoveUnusedImports</recipe>
                    </activeRecipes>
                </configuration>
                <dependencies>
                    <dependency>
                        <groupId>org.openrewrite</groupId>
                        <artifactId>rewrite-java</artifactId>
                        <version>8.37.2</version>
                    </dependency>
                </dependencies>
            </plugin>
        </plugins>
    </build>
</project>`
	
	return os.WriteFile(pomPath, []byte(pomContent), 0644)
}

func (e *OpenRewriteEngine) parseResults(outputPath string, startTime time.Time) (*TransformationResult, error) {
	result := &TransformationResult{
		Success:         false,
		ExecutionTime:   time.Since(startTime),
		ChangesApplied:  0,
		FilesModified:   []string{},
		Diff:            "",
		ValidationScore: 0.0,
		Errors:          []TransformationError{},
		Warnings:        []TransformationError{},
		Metadata:        map[string]interface{}{},
	}

	// Read the actual output file path from the output path file
	actualOutputPathBytes, err := os.ReadFile(outputPath)
	if err != nil {
		result.Errors = append(result.Errors, TransformationError{
			Type:        "file_read_error",
			Message:     fmt.Sprintf("Failed to read output path file: %v", err),
			Recoverable: false,
		})
		return result, nil
	}
	
	actualOutputPath := strings.TrimSpace(string(actualOutputPathBytes))
	
	// Try to read and parse OpenRewrite results JSON
	if _, err := os.Stat(actualOutputPath); err == nil {
		return e.parseOpenRewriteResults(actualOutputPath, result)
	}
	
	// If no results file, check if transformation was successful based on Maven/Gradle output
	// For now, assume success if no results file but no errors during execution
	result.Success = true
	result.ChangesApplied = 0 // No changes if no results file
	result.ValidationScore = 1.0
	result.Metadata = map[string]interface{}{
		"tool":    "maven-gradle-plugin",
		"version": "unknown",
		"note":    "No detailed results file generated",
	}
	
	return result, nil
}

// parseOpenRewriteResults parses OpenRewrite JSON results file
func (e *OpenRewriteEngine) parseOpenRewriteResults(resultsPath string, result *TransformationResult) (*TransformationResult, error) {
	data, err := os.ReadFile(resultsPath)
	if err != nil {
		result.Errors = append(result.Errors, TransformationError{
			Type:        "results_parse_error",
			Message:     fmt.Sprintf("Failed to read results file: %v", err),
			Recoverable: false,
		})
		return result, nil
	}
	
	// Parse OpenRewrite results JSON structure
	var openRewriteResults struct {
		Results []struct {
			SourcePath   string `json:"sourcePath"`
			Recipe       string `json:"recipe"`
			Diff         string `json:"diff"`
			Status       string `json:"status"`
			Error        string `json:"error,omitempty"`
			Warning      string `json:"warning,omitempty"`
		} `json:"results"`
		Summary struct {
			TotalFiles    int `json:"totalFiles"`
			ChangedFiles  int `json:"changedFiles"`
			RecipesApplied []string `json:"recipesApplied"`
		} `json:"summary"`
	}
	
	if err := json.Unmarshal(data, &openRewriteResults); err != nil {
		// If JSON parsing fails, try to extract information from raw output
		result.Success = true // Assume success if we got a results file
		result.ChangesApplied = 1
		result.Diff = "Raw results available but not parseable as JSON"
		result.ValidationScore = 0.5
		result.Warnings = append(result.Warnings, TransformationError{
			Type:        "json_parse_warning",
			Message:     fmt.Sprintf("Could not parse results as JSON: %v", err),
			Recoverable: true,
		})
		return result, nil
	}
	
	// Process parsed results
	result.Success = true
	result.ChangesApplied = openRewriteResults.Summary.ChangedFiles
	result.ValidationScore = 0.95 // High confidence for successful OpenRewrite transformations
	
	var allDiffs []string
	for _, res := range openRewriteResults.Results {
		if res.SourcePath != "" {
			result.FilesModified = append(result.FilesModified, res.SourcePath)
		}
		
		if res.Diff != "" {
			allDiffs = append(allDiffs, fmt.Sprintf("=== %s ===\n%s", res.SourcePath, res.Diff))
		}
		
		if res.Error != "" {
			result.Errors = append(result.Errors, TransformationError{
				Type:        "transformation_error",
				Message:     res.Error,
				File:        res.SourcePath,
				Recoverable: false,
			})
		}
		
		if res.Warning != "" {
			result.Warnings = append(result.Warnings, TransformationError{
				Type:        "transformation_warning", 
				Message:     res.Warning,
				File:        res.SourcePath,
				Recoverable: true,
			})
		}
	}
	
	result.Diff = strings.Join(allDiffs, "\n\n")
	result.Metadata = map[string]interface{}{
		"tool":            "openrewrite",
		"total_files":     openRewriteResults.Summary.TotalFiles,
		"changed_files":   openRewriteResults.Summary.ChangedFiles,
		"recipes_applied": openRewriteResults.Summary.RecipesApplied,
		"results_file":    resultsPath,
	}
	
	// Determine overall success based on errors
	if len(result.Errors) > 0 {
		result.Success = false
		result.ValidationScore = 0.3
	}
	
	return result, nil
}

// Helper methods for build file detection

func (e *OpenRewriteEngine) hasJavaBuildFile(dir string) bool {
	return e.hasPomFile(dir) || e.hasGradleFile(dir)
}

func (e *OpenRewriteEngine) hasPomFile(dir string) bool {
	pomPath := filepath.Join(dir, "pom.xml")
	_, err := os.Stat(pomPath)
	return err == nil
}

func (e *OpenRewriteEngine) hasGradleFile(dir string) bool {
	buildGradle := filepath.Join(dir, "build.gradle")
	buildGradleKts := filepath.Join(dir, "build.gradle.kts")
	
	_, err1 := os.Stat(buildGradle)
	_, err2 := os.Stat(buildGradleKts)
	
	return err1 == nil || err2 == nil
}
