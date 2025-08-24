package arf

import (
	"context"
	"fmt"
	"io/ioutil"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// MockOpenRewriteEngine provides simulated OpenRewrite transformations for testing
type MockOpenRewriteEngine struct {
	recipes map[string]MockRecipeHandler
}

// MockRecipeHandler defines how a mock recipe transforms code
type MockRecipeHandler func(ctx context.Context, repoPath string) (*TransformationResult, error)

// NewMockOpenRewriteEngine creates a mock OpenRewrite engine for testing
func NewMockOpenRewriteEngine() *MockOpenRewriteEngine {
	engine := &MockOpenRewriteEngine{
		recipes: make(map[string]MockRecipeHandler),
	}
	
	// Register mock recipes
	engine.registerMockRecipes()
	
	return engine
}

// registerMockRecipes sets up mock transformations
func (m *MockOpenRewriteEngine) registerMockRecipes() {
	// Java 11 to 17 migration
	m.recipes["org.openrewrite.java.migrate.Java11toJava17"] = m.mockJava11To17Migration
	
	// Spring Boot 3 upgrade
	m.recipes["org.openrewrite.java.spring.boot3.UpgradeSpringBoot_3_0"] = m.mockSpringBoot3Upgrade
	
	// Spring Boot 3 best practices
	m.recipes["org.openrewrite.java.spring.boot3.SpringBoot3BestPractices"] = m.mockSpringBoot3BestPractices
	
	// Generic cleanup recipes
	m.recipes["org.openrewrite.java.cleanup.UnnecessaryParentheses"] = m.mockRemoveUnnecessaryParentheses
	m.recipes["org.openrewrite.java.cleanup.RemoveUnusedImports"] = m.mockRemoveUnusedImports
}

// ApplyRecipe applies a mock recipe to the repository
func (m *MockOpenRewriteEngine) ApplyRecipe(ctx context.Context, recipeID string, repoPath string) (*TransformationResult, error) {
	handler, exists := m.recipes[recipeID]
	if !exists {
		// Return a generic transformation for unknown recipes
		return m.genericTransformation(ctx, repoPath, recipeID)
	}
	
	return handler(ctx, repoPath)
}

// mockJava11To17Migration simulates Java version migration
func (m *MockOpenRewriteEngine) mockJava11To17Migration(ctx context.Context, repoPath string) (*TransformationResult, error) {
	startTime := time.Now()
	
	result := &TransformationResult{
		RecipeID:       "org.openrewrite.java.migrate.Java11toJava17",
		Success:        true,
		ChangesApplied: 0,
		FilesModified:  []string{},
	}
	
	// Find pom.xml or build.gradle
	pomPath := filepath.Join(repoPath, "pom.xml")
	gradlePath := filepath.Join(repoPath, "build.gradle")
	
	if _, err := os.Stat(pomPath); err == nil {
		// Update Maven configuration
		if err := m.updateMavenJavaVersion(pomPath, "17"); err == nil {
			result.ChangesApplied++
			result.FilesModified = append(result.FilesModified, "pom.xml")
			result.Diff = "Updated Java version from 11 to 17 in pom.xml"
		}
	} else if _, err := os.Stat(gradlePath); err == nil {
		// Update Gradle configuration
		if err := m.updateGradleJavaVersion(gradlePath, "17"); err == nil {
			result.ChangesApplied++
			result.FilesModified = append(result.FilesModified, "build.gradle")
			result.Diff = "Updated Java version from 11 to 17 in build.gradle"
		}
	}
	
	// Simulate updating some Java files with new language features
	javaFiles := m.findJavaFiles(repoPath, 3) // Find up to 3 Java files
	for _, file := range javaFiles {
		if m.addJava17Features(file) {
			result.ChangesApplied++
			relPath, _ := filepath.Rel(repoPath, file)
			result.FilesModified = append(result.FilesModified, relPath)
		}
	}
	
	result.ExecutionTime = time.Since(startTime)
	
	return result, nil
}

// mockSpringBoot3Upgrade simulates Spring Boot 3 upgrade
func (m *MockOpenRewriteEngine) mockSpringBoot3Upgrade(ctx context.Context, repoPath string) (*TransformationResult, error) {
	startTime := time.Now()
	
	result := &TransformationResult{
		RecipeID:       "org.openrewrite.java.spring.boot3.UpgradeSpringBoot_3_0",
		Success:        true,
		ChangesApplied: 0,
		FilesModified:  []string{},
	}
	
	// Update Spring Boot version in pom.xml
	pomPath := filepath.Join(repoPath, "pom.xml")
	if _, err := os.Stat(pomPath); err == nil {
		if err := m.updateSpringBootVersion(pomPath, "3.0.0"); err == nil {
			result.ChangesApplied++
			result.FilesModified = append(result.FilesModified, "pom.xml")
			result.Diff = "Updated Spring Boot version to 3.0.0"
		}
	}
	
	// Update imports in Java files (javax -> jakarta)
	javaFiles := m.findJavaFiles(repoPath, 5)
	for _, file := range javaFiles {
		if m.updateJavaxToJakarta(file) {
			result.ChangesApplied++
			relPath, _ := filepath.Rel(repoPath, file)
			result.FilesModified = append(result.FilesModified, relPath)
		}
	}
	
	result.ExecutionTime = time.Since(startTime)
	
	return result, nil
}

// mockSpringBoot3BestPractices applies Spring Boot 3 best practices
func (m *MockOpenRewriteEngine) mockSpringBoot3BestPractices(ctx context.Context, repoPath string) (*TransformationResult, error) {
	startTime := time.Now()
	
	result := &TransformationResult{
		RecipeID:       "org.openrewrite.java.spring.boot3.SpringBoot3BestPractices",
		Success:        true,
		ChangesApplied: 0,
		FilesModified:  []string{},
		Diff:          "Applied Spring Boot 3 best practices",
	}
	
	// Simulate minor improvements in configuration files
	configFiles := []string{
		"application.properties",
		"application.yml",
		"application.yaml",
	}
	
	for _, configFile := range configFiles {
		path := filepath.Join(repoPath, "src/main/resources", configFile)
		if _, err := os.Stat(path); err == nil {
			// Add a comment about best practices
			if m.addConfigComment(path) {
				result.ChangesApplied++
				result.FilesModified = append(result.FilesModified, "src/main/resources/"+configFile)
			}
		}
	}
	
	result.ExecutionTime = time.Since(startTime)
	
	return result, nil
}

// mockRemoveUnnecessaryParentheses removes unnecessary parentheses
func (m *MockOpenRewriteEngine) mockRemoveUnnecessaryParentheses(ctx context.Context, repoPath string) (*TransformationResult, error) {
	startTime := time.Now()
	
	result := &TransformationResult{
		RecipeID:       "org.openrewrite.java.cleanup.UnnecessaryParentheses",
		Success:        true,
		ChangesApplied: 0,
		FilesModified:  []string{},
	}
	
	// Find and modify a few Java files
	javaFiles := m.findJavaFiles(repoPath, 2)
	for _, file := range javaFiles {
		if m.removeParentheses(file) {
			result.ChangesApplied++
			relPath, _ := filepath.Rel(repoPath, file)
			result.FilesModified = append(result.FilesModified, relPath)
		}
	}
	
	result.Diff = fmt.Sprintf("Removed unnecessary parentheses from %d files", result.ChangesApplied)
	result.ExecutionTime = time.Since(startTime)
	
	return result, nil
}

// mockRemoveUnusedImports removes unused imports
func (m *MockOpenRewriteEngine) mockRemoveUnusedImports(ctx context.Context, repoPath string) (*TransformationResult, error) {
	startTime := time.Now()
	
	result := &TransformationResult{
		RecipeID:       "org.openrewrite.java.cleanup.RemoveUnusedImports",
		Success:        true,
		ChangesApplied: 0,
		FilesModified:  []string{},
	}
	
	// Find and clean imports in Java files
	javaFiles := m.findJavaFiles(repoPath, 3)
	for _, file := range javaFiles {
		if m.cleanImports(file) {
			result.ChangesApplied++
			relPath, _ := filepath.Rel(repoPath, file)
			result.FilesModified = append(result.FilesModified, relPath)
		}
	}
	
	result.Diff = fmt.Sprintf("Removed unused imports from %d files", result.ChangesApplied)
	result.ExecutionTime = time.Since(startTime)
	
	return result, nil
}

// genericTransformation provides a generic transformation for unknown recipes
func (m *MockOpenRewriteEngine) genericTransformation(ctx context.Context, repoPath string, recipeID string) (*TransformationResult, error) {
	startTime := time.Now()
	
	result := &TransformationResult{
		RecipeID:       recipeID,
		Success:        true,
		ChangesApplied: 1,
		FilesModified:  []string{"MockFile.java"},
		Diff:          fmt.Sprintf("Applied mock transformation for recipe: %s", recipeID),
	}
	
	result.ExecutionTime = time.Since(startTime)
	
	return result, nil
}

// Helper functions for mock transformations

func (m *MockOpenRewriteEngine) findJavaFiles(repoPath string, limit int) []string {
	var files []string
	count := 0
	
	filepath.Walk(repoPath, func(path string, info os.FileInfo, err error) error {
		if err != nil || count >= limit {
			return filepath.SkipDir
		}
		
		if strings.HasSuffix(path, ".java") && !strings.Contains(path, "/test/") {
			files = append(files, path)
			count++
		}
		
		return nil
	})
	
	return files
}

func (m *MockOpenRewriteEngine) updateMavenJavaVersion(pomPath string, version string) error {
	content, err := ioutil.ReadFile(pomPath)
	if err != nil {
		return err
	}
	
	text := string(content)
	
	// Update java.version property
	text = strings.ReplaceAll(text, "<java.version>11</java.version>", "<java.version>17</java.version>")
	text = strings.ReplaceAll(text, "<java.version>1.8</java.version>", "<java.version>17</java.version>")
	
	// Update maven.compiler properties
	text = strings.ReplaceAll(text, "<maven.compiler.source>11</maven.compiler.source>", "<maven.compiler.source>17</maven.compiler.source>")
	text = strings.ReplaceAll(text, "<maven.compiler.target>11</maven.compiler.target>", "<maven.compiler.target>17</maven.compiler.target>")
	
	return ioutil.WriteFile(pomPath, []byte(text), 0644)
}

func (m *MockOpenRewriteEngine) updateGradleJavaVersion(gradlePath string, version string) error {
	content, err := ioutil.ReadFile(gradlePath)
	if err != nil {
		return err
	}
	
	text := string(content)
	
	// Update sourceCompatibility and targetCompatibility
	text = strings.ReplaceAll(text, "sourceCompatibility = '11'", "sourceCompatibility = '17'")
	text = strings.ReplaceAll(text, "targetCompatibility = '11'", "targetCompatibility = '17'")
	text = strings.ReplaceAll(text, "sourceCompatibility = JavaVersion.VERSION_11", "sourceCompatibility = JavaVersion.VERSION_17")
	text = strings.ReplaceAll(text, "targetCompatibility = JavaVersion.VERSION_11", "targetCompatibility = JavaVersion.VERSION_17")
	
	return ioutil.WriteFile(gradlePath, []byte(text), 0644)
}

func (m *MockOpenRewriteEngine) updateSpringBootVersion(pomPath string, version string) error {
	content, err := ioutil.ReadFile(pomPath)
	if err != nil {
		return err
	}
	
	text := string(content)
	
	// Update Spring Boot version
	text = strings.ReplaceAll(text, "<version>2.7.", "<version>3.0.")
	text = strings.ReplaceAll(text, "<version>2.6.", "<version>3.0.")
	text = strings.ReplaceAll(text, "<version>2.5.", "<version>3.0.")
	
	return ioutil.WriteFile(pomPath, []byte(text), 0644)
}

func (m *MockOpenRewriteEngine) addJava17Features(javaFile string) bool {
	content, err := ioutil.ReadFile(javaFile)
	if err != nil {
		return false
	}
	
	text := string(content)
	modified := false
	
	// Add text blocks (Java 15+ feature)
	if strings.Contains(text, "String sql = ") && !strings.Contains(text, `"""`) {
		// Don't actually modify for now, just simulate
		modified = true
	}
	
	// Add switch expressions (Java 14+ feature)
	if strings.Contains(text, "switch (") && !strings.Contains(text, "->") {
		// Don't actually modify for now, just simulate
		modified = true
	}
	
	// Simulate adding a comment about Java 17
	if !strings.Contains(text, "Java 17") {
		text = "// Upgraded to Java 17\n" + text
		ioutil.WriteFile(javaFile, []byte(text), 0644)
		modified = true
	}
	
	return modified
}

func (m *MockOpenRewriteEngine) updateJavaxToJakarta(javaFile string) bool {
	content, err := ioutil.ReadFile(javaFile)
	if err != nil {
		return false
	}
	
	text := string(content)
	original := text
	
	// Update imports
	text = strings.ReplaceAll(text, "import javax.servlet", "import jakarta.servlet")
	text = strings.ReplaceAll(text, "import javax.persistence", "import jakarta.persistence")
	text = strings.ReplaceAll(text, "import javax.validation", "import jakarta.validation")
	
	if text != original {
		return ioutil.WriteFile(javaFile, []byte(text), 0644) == nil
	}
	
	return false
}

func (m *MockOpenRewriteEngine) addConfigComment(configFile string) bool {
	content, err := ioutil.ReadFile(configFile)
	if err != nil {
		return false
	}
	
	text := string(content)
	if !strings.Contains(text, "Spring Boot 3 best practices applied") {
		text = "# Spring Boot 3 best practices applied\n" + text
		return ioutil.WriteFile(configFile, []byte(text), 0644) == nil
	}
	
	return false
}

func (m *MockOpenRewriteEngine) removeParentheses(javaFile string) bool {
	// Simulate removing unnecessary parentheses
	// In reality, this would parse and modify the AST
	rand.Seed(time.Now().UnixNano())
	return rand.Float32() > 0.5 // 50% chance of making changes
}

func (m *MockOpenRewriteEngine) cleanImports(javaFile string) bool {
	// Simulate cleaning imports
	// In reality, this would analyze usage and remove unused imports
	rand.Seed(time.Now().UnixNano())
	return rand.Float32() > 0.3 // 70% chance of making changes
}