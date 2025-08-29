package arf

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"

	"github.com/iw2rmb/ploy/api/arf/models"
	"github.com/iw2rmb/ploy/internal/storage"
	"gopkg.in/yaml.v3"
)

// UnifiedRecipeMetadata represents the unified format for all recipes
type UnifiedRecipeMetadata struct {
	Metadata RecipeInfo           `json:"metadata" yaml:"metadata"`
	Maven    *MavenInfo          `json:"maven,omitempty" yaml:"maven,omitempty"`
	Steps    []models.RecipeStep `json:"steps,omitempty" yaml:"steps,omitempty"`
	Cache    *CacheInfo          `json:"cache,omitempty" yaml:"cache,omitempty"`
}

// RecipeInfo contains basic recipe metadata
type RecipeInfo struct {
	ID         string   `json:"id" yaml:"id"`
	Name       string   `json:"name" yaml:"name"`
	Version    string   `json:"version" yaml:"version"`
	Type       string   `json:"type" yaml:"type"`
	Source     string   `json:"source" yaml:"source"`
	Author     string   `json:"author" yaml:"author"`
	Tags       []string `json:"tags" yaml:"tags"`
	Categories []string `json:"categories" yaml:"categories"`
}

// MavenInfo contains Maven-specific recipe information
type MavenInfo struct {
	Group    string `json:"group" yaml:"group"`
	Artifact string `json:"artifact" yaml:"artifact"`
	Version  string `json:"version" yaml:"version"`
	Class    string `json:"class" yaml:"class"`
}

// CacheInfo contains cache-related metadata
type CacheInfo struct {
	StoredAt   time.Time `json:"stored_at" yaml:"stored_at"`
	JarPath    string    `json:"jar_path,omitempty" yaml:"jar_path,omitempty"`
	SizeBytes  int64     `json:"size_bytes" yaml:"size_bytes"`
	Hash       string    `json:"hash" yaml:"hash"`
}

// RecipeRegistry manages unified recipe storage for both Maven and custom recipes
type RecipeRegistry struct {
	storage storage.StorageProvider
	bucket  string
}

// NewRecipeRegistry creates a new recipe registry
func NewRecipeRegistry(storage storage.StorageProvider) *RecipeRegistry {
	return &RecipeRegistry{
		storage: storage,
		bucket:  "ploy-recipes",
	}
}

// RegisterMavenRecipe registers a Maven-based OpenRewrite recipe
func (r *RecipeRegistry) RegisterMavenRecipe(ctx context.Context, coords, jarPath, recipeClass string) error {
	// Parse Maven coordinates
	parts := strings.Split(coords, ":")
	if len(parts) != 3 {
		return fmt.Errorf("invalid Maven coordinates: %s", coords)
	}
	group, artifact, version := parts[0], parts[1], parts[2]

	// Generate recipe ID from class name
	recipeID := generateRecipeID(recipeClass)
	
	// Create unified metadata
	metadata := &UnifiedRecipeMetadata{
		Metadata: RecipeInfo{
			ID:         recipeID,
			Name:       generateRecipeName(recipeClass),
			Version:    version,
			Type:       "openrewrite",
			Source:     "maven",
			Author:     "OpenRewrite",
			Tags:       generateTags(recipeClass),
			Categories: generateCategories(recipeClass),
		},
		Maven: &MavenInfo{
			Group:    group,
			Artifact: artifact,
			Version:  version,
			Class:    recipeClass,
		},
		Cache: &CacheInfo{
			StoredAt:  time.Now(),
			JarPath:   jarPath,
			SizeBytes: 0, // Would be calculated from actual JAR
			Hash:      generateHash(coords),
		},
	}

	// Store in registry
	return r.storeRecipe(ctx, metadata)
}

// RegisterCustomRecipe registers a custom recipe
func (r *RecipeRegistry) RegisterCustomRecipe(ctx context.Context, recipe *models.Recipe) error {
	// Generate recipe ID
	recipeID := recipe.Metadata.Name
	if recipe.Metadata.Version != "" && recipe.Metadata.Version != "latest" {
		recipeID = fmt.Sprintf("%s-%s", recipe.Metadata.Name, recipe.Metadata.Version)
	}

	// Determine recipe type from steps
	recipeType := "composite"
	if len(recipe.Steps) == 1 {
		recipeType = string(recipe.Steps[0].Type)
	}

	// Create unified metadata
	metadata := &UnifiedRecipeMetadata{
		Metadata: RecipeInfo{
			ID:         recipeID,
			Name:       recipe.Metadata.Description,
			Version:    recipe.Metadata.Version,
			Type:       recipeType,
			Source:     "custom",
			Author:     recipe.Metadata.Author,
			Tags:       recipe.Metadata.Tags,
			Categories: recipe.Metadata.Categories,
		},
		Steps: recipe.Steps,
		Cache: &CacheInfo{
			StoredAt:  time.Now(),
			SizeBytes: 0, // Would be calculated
			Hash:      generateHash(recipeID),
		},
	}

	// Store in registry
	return r.storeRecipe(ctx, metadata)
}

// GetRecipe retrieves a recipe by ID
func (r *RecipeRegistry) GetRecipe(ctx context.Context, id string) (*UnifiedRecipeMetadata, error) {
	// Construct registry path
	key := fmt.Sprintf("registry/%s.yaml", id)
	
	// Get from storage
	dataReader, err := r.storage.GetObject(r.bucket, key)
	if err != nil {
		return nil, fmt.Errorf("recipe not found: %s", id)
	}
	defer dataReader.Close()
	
	// Read data
	data, err := io.ReadAll(dataReader)
	if err != nil {
		return nil, fmt.Errorf("failed to read recipe data: %w", err)
	}

	// Parse metadata
	var metadata UnifiedRecipeMetadata
	if err := yaml.Unmarshal(data, &metadata); err != nil {
		return nil, fmt.Errorf("failed to parse recipe metadata: %w", err)
	}

	return &metadata, nil
}

// ListAllRecipes lists all recipes in the registry
func (r *RecipeRegistry) ListAllRecipes(ctx context.Context) ([]*UnifiedRecipeMetadata, error) {
	// List all registry entries
	objects, err := r.storage.ListObjects(r.bucket, "registry/")
	if err != nil {
		return nil, fmt.Errorf("failed to list recipes: %w", err)
	}

	recipes := make([]*UnifiedRecipeMetadata, 0, len(objects))
	for _, obj := range objects {
		// Skip non-YAML files
		if !strings.HasSuffix(obj.Key, ".yaml") {
			continue
		}

		// Get recipe metadata
		dataReader, err := r.storage.GetObject(r.bucket, obj.Key)
		if err != nil {
			continue // Skip failed reads
		}
		
		// Read data
		data, err := io.ReadAll(dataReader)
		dataReader.Close()
		if err != nil {
			continue // Skip read errors
		}

		var metadata UnifiedRecipeMetadata
		if err := yaml.Unmarshal(data, &metadata); err != nil {
			continue // Skip invalid metadata
		}

		recipes = append(recipes, &metadata)
	}

	return recipes, nil
}

// QueryByType queries recipes by type
func (r *RecipeRegistry) QueryByType(ctx context.Context, recipeType string) ([]*UnifiedRecipeMetadata, error) {
	// Get all recipes
	allRecipes, err := r.ListAllRecipes(ctx)
	if err != nil {
		return nil, err
	}

	// Filter by type
	var filtered []*UnifiedRecipeMetadata
	for _, recipe := range allRecipes {
		if recipe.Metadata.Type == recipeType {
			filtered = append(filtered, recipe)
		}
	}

	return filtered, nil
}

// ExtractRecipeFromJAR extracts recipe metadata from a Maven JAR
func (r *RecipeRegistry) ExtractRecipeFromJAR(ctx context.Context, jarPath string) (*UnifiedRecipeMetadata, error) {
	// Read JAR file
	jarReader, err := r.storage.GetObject(r.bucket, jarPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read JAR: %w", err)
	}
	defer jarReader.Close()
	
	// Read JAR data
	jarData, err := io.ReadAll(jarReader)
	if err != nil {
		return nil, fmt.Errorf("failed to read JAR data: %w", err)
	}

	// Open as ZIP (JARs are ZIP files)
	reader, err := zip.NewReader(bytes.NewReader(jarData), int64(len(jarData)))
	if err != nil {
		return nil, fmt.Errorf("failed to open JAR: %w", err)
	}

	// Look for recipe metadata in META-INF/rewrite/
	var metadata *UnifiedRecipeMetadata
	for _, file := range reader.File {
		if strings.HasPrefix(file.Name, "META-INF/rewrite/") && strings.HasSuffix(file.Name, ".yml") {
			// Read recipe YAML
			rc, err := file.Open()
			if err != nil {
				continue
			}
			defer rc.Close()

			yamlData, err := io.ReadAll(rc)
			if err != nil {
				continue
			}

			// Parse recipe definition
			// TODO: Parse yamlData to extract recipe details
			_ = yamlData // For now, just acknowledge we have the data
			metadata = &UnifiedRecipeMetadata{
				Metadata: RecipeInfo{
					ID:         filepath.Base(file.Name),
					Name:       "Extracted Recipe",
					Type:       "openrewrite",
					Source:     "maven",
					Tags:       []string{"java"},
					Categories: []string{"java-migration"},
				},
			}
			break
		}
	}

	if metadata == nil {
		// Create default metadata if no recipe found in JAR
		metadata = &UnifiedRecipeMetadata{
			Metadata: RecipeInfo{
				ID:         filepath.Base(jarPath),
				Name:       "Maven Recipe",
				Type:       "openrewrite",
				Source:     "maven",
				Tags:       []string{"java"},
				Categories: []string{"java-migration"},
			},
		}
	}

	return metadata, nil
}

// storeRecipe stores recipe metadata in the registry
func (r *RecipeRegistry) storeRecipe(ctx context.Context, metadata *UnifiedRecipeMetadata) error {
	// Serialize to YAML
	data, err := yaml.Marshal(metadata)
	if err != nil {
		return fmt.Errorf("failed to serialize recipe: %w", err)
	}

	// Store in registry
	key := fmt.Sprintf("registry/%s.yaml", metadata.Metadata.ID)
	dataReader := bytes.NewReader(data)
	result, err := r.storage.PutObject(r.bucket, key, dataReader, "application/x-yaml")
	if err != nil {
		return fmt.Errorf("failed to store recipe: %w", err)
	}
	_ = result // We don't need the result for now

	// If it's a custom recipe, also store the implementation
	if metadata.Metadata.Source == "custom" && len(metadata.Steps) > 0 {
		implKey := fmt.Sprintf("custom/%s/recipe.yaml", metadata.Metadata.ID)
		dataReader = bytes.NewReader(data)
		result, err = r.storage.PutObject(r.bucket, implKey, dataReader, "application/x-yaml")
		if err != nil {
			return fmt.Errorf("failed to store custom recipe implementation: %w", err)
		}
		_ = result // We don't need the result for now
	}

	return nil
}

// Helper functions

func generateRecipeID(recipeClass string) string {
	// Convert class name to ID
	// org.openrewrite.java.migrate.Java11toJava17 -> java11to17
	parts := strings.Split(recipeClass, ".")
	if len(parts) > 0 {
		lastPart := parts[len(parts)-1]
		// Convert CamelCase to lowercase
		id := strings.ToLower(lastPart)
		// Handle special cases
		id = strings.ReplaceAll(id, "upgradetojava", "java")
		id = strings.ReplaceAll(id, "tojava", "to")
		id = strings.ReplaceAll(id, "migration", "")
		id = strings.ReplaceAll(id, "upgrade", "")
		return strings.TrimSuffix(id, "-")
	}
	return "unknown"
}

func generateRecipeName(recipeClass string) string {
	// Convert class name to human-readable name
	// org.openrewrite.java.migrate.Java11toJava17 -> Java 11 to 17 Migration
	parts := strings.Split(recipeClass, ".")
	if len(parts) > 0 {
		lastPart := parts[len(parts)-1]
		// Add spaces to CamelCase
		name := ""
		for i, r := range lastPart {
			if i > 0 && r >= 'A' && r <= 'Z' {
				name += " "
			}
			name += string(r)
		}
		// Clean up known patterns
		name = strings.ReplaceAll(name, "Java11to Java17", "Java 11 to 17")
		name = strings.ReplaceAll(name, "Java8to Java11", "Java 8 to 11")
		name = strings.ReplaceAll(name, "Spring Boot_3_2", "Spring Boot 3.2")
		if !strings.Contains(name, "Migration") && strings.Contains(name, "Java") {
			name += " Migration"
		}
		return name
	}
	return "Unknown Recipe"
}

func generateTags(recipeClass string) []string {
	tags := []string{}
	lower := strings.ToLower(recipeClass)
	
	if strings.Contains(lower, "java") {
		tags = append(tags, "java")
	}
	if strings.Contains(lower, "spring") {
		tags = append(tags, "spring")
	}
	if strings.Contains(lower, "boot") {
		tags = append(tags, "spring-boot")
	}
	if strings.Contains(lower, "security") {
		tags = append(tags, "security")
	}
	if strings.Contains(lower, "migrate") || strings.Contains(lower, "migration") {
		tags = append(tags, "migration")
	}
	if strings.Contains(lower, "11to17") || strings.Contains(lower, "java17") {
		tags = append(tags, "java17")
	}
	if strings.Contains(lower, "8to11") || strings.Contains(lower, "java11") {
		tags = append(tags, "java11")
	}
	if strings.Contains(lower, "junit") {
		tags = append(tags, "testing", "junit")
	}
	
	if len(tags) == 0 {
		tags = append(tags, "openrewrite")
	}
	
	return tags
}

func generateCategories(recipeClass string) []string {
	categories := []string{}
	lower := strings.ToLower(recipeClass)
	
	if strings.Contains(lower, "java") && (strings.Contains(lower, "migrate") || strings.Contains(lower, "upgrade")) {
		categories = append(categories, "java-migration")
	}
	if strings.Contains(lower, "spring") {
		categories = append(categories, "spring")
	}
	if strings.Contains(lower, "security") {
		categories = append(categories, "security")
	}
	if strings.Contains(lower, "test") || strings.Contains(lower, "junit") {
		categories = append(categories, "testing")
	}
	if strings.Contains(lower, "log") {
		categories = append(categories, "logging")
	}
	
	if len(categories) == 0 {
		categories = append(categories, "transformation")
	}
	
	return categories
}

func generateHash(input string) string {
	// Simple hash generation for demo
	// In production, use crypto/sha256
	return fmt.Sprintf("sha256:%x", input)
}