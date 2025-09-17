package recipes

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"

	"github.com/iw2rmb/ploy/api/recipes/models"
	"github.com/iw2rmb/ploy/internal/storage"
	"gopkg.in/yaml.v3"
)

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
	// Try the ID as-is first
	key := fmt.Sprintf("registry/%s.yaml", id)

	dataReader, err := r.storage.GetObject(r.bucket, key)
	if err != nil {
		// If not found and ID looks like a full recipe class name, try generating the recipe ID
		if strings.Contains(id, ".") {
			generatedID := generateRecipeID(id)
			if generatedID != id {
				key = fmt.Sprintf("registry/%s.yaml", generatedID)
				dataReader, err = r.storage.GetObject(r.bucket, key)
			}
		}

		if err != nil {
			return nil, fmt.Errorf("recipe not found: %s", id)
		}
	}
	defer func() { _ = dataReader.Close() }()

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
		func() { _ = dataReader.Close() }()
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
	defer func() { _ = jarReader.Close() }()

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
			defer func() { _ = rc.Close() }()

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
