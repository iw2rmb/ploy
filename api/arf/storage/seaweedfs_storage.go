package storage

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/iw2rmb/ploy/api/arf/models"
	"github.com/iw2rmb/ploy/internal/storage"
	"gopkg.in/yaml.v3"
)

// SeaweedFSRecipeStorage implements RecipeStorage using SeaweedFS
type SeaweedFSRecipeStorage struct {
	client     storage.StorageProvider
	bucketName string
	keyPrefix  string
	indexStore RecipeIndexStore
	validator  RecipeValidator
	cache      *recipeCache
	mu         sync.RWMutex
}

// recipeCache provides in-memory caching for recipes
type recipeCache struct {
	recipes map[string]*cacheEntry
	mu      sync.RWMutex
	ttl     time.Duration
}

type cacheEntry struct {
	recipe    *models.Recipe
	timestamp time.Time
}

// retryOperation retries a storage operation with exponential backoff
func (s *SeaweedFSRecipeStorage) retryOperation(operation func() error, maxRetries int) error {
	var lastErr error
	for attempt := 0; attempt < maxRetries; attempt++ {
		if err := operation(); err != nil {
			lastErr = err
			if attempt < maxRetries-1 {
				backoff := time.Duration(1<<attempt) * 100 * time.Millisecond
				if backoff > 5*time.Second {
					backoff = 5 * time.Second
				}
				time.Sleep(backoff)
				continue
			}
		} else {
			return nil
		}
	}
	return fmt.Errorf("operation failed after %d attempts: %w", maxRetries, lastErr)
}

// NewSeaweedFSRecipeStorage creates a new SeaweedFS-based recipe storage
func NewSeaweedFSRecipeStorage(client storage.StorageProvider, indexStore RecipeIndexStore, validator RecipeValidator) *SeaweedFSRecipeStorage {
	return &SeaweedFSRecipeStorage{
		client:     client,
		bucketName: "ploy-recipes",
		keyPrefix:  "recipes",
		indexStore: indexStore,
		validator:  validator,
		cache: &recipeCache{
			recipes: make(map[string]*cacheEntry),
			ttl:     5 * time.Minute,
		},
	}
}

// CreateRecipe creates a new recipe in storage
func (s *SeaweedFSRecipeStorage) CreateRecipe(ctx context.Context, recipe *models.Recipe) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Validate recipe
	if s.validator != nil {
		if err := s.validator.ValidateRecipe(recipe); err != nil {
			return fmt.Errorf("recipe validation failed: %w", err)
		}
	}

	// Set system fields
	recipe.SetSystemFields("system") // TODO: Get actual user from context

	// Check if recipe already exists
	existingKey := s.getRecipeKey(recipe.ID)
	if existing, _ := s.getObjectContent(existingKey); existing != nil {
		return fmt.Errorf("recipe with ID %s already exists", recipe.ID)
	}

	// Serialize recipe to YAML
	data, err := yaml.Marshal(recipe)
	if err != nil {
		return fmt.Errorf("failed to serialize recipe: %w", err)
	}

	// Store recipe in SeaweedFS with retry
	key := s.getRecipeKey(recipe.ID)
	err = s.retryOperation(func() error {
		_, err := s.client.PutObject(s.bucketName, key, bytes.NewReader(data), "application/x-yaml")
		return err
	}, 3)
	if err != nil {
		return fmt.Errorf("failed to store recipe: %w", err)
	}

	// Store version pointer
	if recipe.Metadata.Version != "" {
		versionKey := s.getVersionKey(recipe.Metadata.Name, recipe.Metadata.Version)
		_, err = s.client.PutObject(s.bucketName, versionKey, bytes.NewReader([]byte(recipe.ID)), "text/plain")
		if err != nil {
			// Log error but don't fail the operation
			fmt.Printf("Warning: failed to store version pointer: %v\n", err)
		}
	}

	// Update latest pointer
	latestKey := s.getLatestKey(recipe.Metadata.Name)
	_, err = s.client.PutObject(s.bucketName, latestKey, bytes.NewReader([]byte(recipe.ID)), "text/plain")
	if err != nil {
		// Log error but don't fail the operation
		fmt.Printf("Warning: failed to update latest pointer: %v\n", err)
	}

	// Update index
	if s.indexStore != nil {
		if err := s.indexStore.UpdateIndex(ctx, recipe, IndexActionAdd); err != nil {
			// Log error but don't fail the operation
			fmt.Printf("Warning: failed to update index: %v\n", err)
		}
	}

	// Clear cache for this recipe
	s.cache.delete(recipe.ID)

	return nil
}

// GetRecipe retrieves a recipe by ID
func (s *SeaweedFSRecipeStorage) GetRecipe(ctx context.Context, id string) (*models.Recipe, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Check if recipe is deleted
	if s.isRecipeDeleted(id) {
		return nil, fmt.Errorf("recipe not found: %s (deleted)", id)
	}

	// Check cache first
	if cached := s.cache.get(id); cached != nil {
		return cached, nil
	}

	// Retrieve from storage
	key := s.getRecipeKey(id)
	data, err := s.getObjectContent(key)
	if err != nil {
		return nil, fmt.Errorf("recipe not found: %s", id)
	}

	// Deserialize recipe
	var recipe models.Recipe
	if err := yaml.Unmarshal(data, &recipe); err != nil {
		return nil, fmt.Errorf("failed to deserialize recipe: %w", err)
	}

	// Cache the recipe
	s.cache.set(id, &recipe)

	return &recipe, nil
}

// GetRecipeByNameAndVersion retrieves a recipe by name and version
func (s *SeaweedFSRecipeStorage) GetRecipeByNameAndVersion(ctx context.Context, name, version string) (*models.Recipe, error) {
	// Get recipe ID from version pointer
	versionKey := s.getVersionKey(name, version)
	idData, err := s.getObjectContent(versionKey)
	if err != nil {
		return nil, fmt.Errorf("recipe version not found: %s@%s", name, version)
	}

	recipeID := string(idData)
	return s.GetRecipe(ctx, recipeID)
}

// UpdateRecipe updates an existing recipe
func (s *SeaweedFSRecipeStorage) UpdateRecipe(ctx context.Context, id string, recipe *models.Recipe) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Validate recipe
	if s.validator != nil {
		if err := s.validator.ValidateRecipe(recipe); err != nil {
			return fmt.Errorf("recipe validation failed: %w", err)
		}
	}

	// Check if recipe exists
	existingKey := s.getRecipeKey(id)
	if existing, _ := s.getObjectContent(existingKey); existing == nil {
		return fmt.Errorf("recipe not found: %s", id)
	}

	// Update system fields
	recipe.ID = id
	recipe.UpdatedAt = time.Now()
	hash, _ := recipe.CalculateHash()
	recipe.Hash = hash

	// Serialize recipe
	data, err := yaml.Marshal(recipe)
	if err != nil {
		return fmt.Errorf("failed to serialize recipe: %w", err)
	}

	// Update in storage with retry
	err = s.retryOperation(func() error {
		_, err := s.client.PutObject(s.bucketName, existingKey, bytes.NewReader(data), "application/x-yaml")
		return err
	}, 3)
	if err != nil {
		return fmt.Errorf("failed to update recipe: %w", err)
	}

	// Update index
	if s.indexStore != nil {
		if err := s.indexStore.UpdateIndex(ctx, recipe, IndexActionUpdate); err != nil {
			fmt.Printf("Warning: failed to update index: %v\n", err)
		}
	}

	// Clear cache
	s.cache.delete(id)

	return nil
}

// DeleteRecipe deletes a recipe
func (s *SeaweedFSRecipeStorage) DeleteRecipe(ctx context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Get recipe first to update index
	recipe, err := s.GetRecipe(ctx, id)
	if err != nil {
		return err
	}

	// Mark recipe as deleted by creating a deletion marker
	key := s.getRecipeKey(id)
	deletionKey := s.getDeletionKey(id)

	// Create deletion marker with timestamp
	deletionInfo := map[string]interface{}{
		"deleted_at":   time.Now().UTC().Format(time.RFC3339),
		"deleted_by":   "system", // TODO: Get from context
		"original_key": key,
	}
	deletionData, _ := yaml.Marshal(deletionInfo)

	// Store deletion marker
	err = s.retryOperation(func() error {
		_, err := s.client.PutObject(s.bucketName, deletionKey, bytes.NewReader(deletionData), "application/x-yaml")
		return err
	}, 3)
	if err != nil {
		return fmt.Errorf("failed to mark recipe as deleted: %w", err)
	}

	// Update latest pointer if this was the latest version
	latestKey := s.getLatestKey(recipe.Metadata.Name)
	latestData, err := s.getObjectContent(latestKey)
	if err == nil && string(latestData) == id {
		// Find the next most recent version (excluding deleted ones)
		versions, err := s.GetRecipeVersions(ctx, recipe.Metadata.Name)
		if err == nil && len(versions) > 0 {
			// Set latest to the most recent non-deleted version
			for _, v := range versions {
				if v.ID != id && !s.isRecipeDeleted(v.ID) {
					s.client.PutObject(s.bucketName, latestKey, bytes.NewReader([]byte(v.ID)), "text/plain")
					break
				}
			}
		}
	}

	// Update index
	if s.indexStore != nil {
		if err := s.indexStore.UpdateIndex(ctx, recipe, IndexActionRemove); err != nil {
			fmt.Printf("Warning: failed to update index: %v\n", err)
		}
	}

	// Clear cache
	s.cache.delete(id)

	return nil
}

// ListRecipes lists recipes with filtering
func (s *SeaweedFSRecipeStorage) ListRecipes(ctx context.Context, filter RecipeFilter) ([]*models.Recipe, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// If we have specific filters, use index to get recipe IDs
	var recipeIDs []string
	useIndex := false

	if len(filter.Tags) > 0 && s.indexStore != nil {
		ids, err := s.indexStore.QueryByTags(ctx, filter.Tags)
		if err == nil {
			recipeIDs = ids
			useIndex = true
		}
	} else if filter.Author != "" && s.indexStore != nil {
		ids, err := s.indexStore.QueryByAuthor(ctx, filter.Author)
		if err == nil {
			recipeIDs = ids
			useIndex = true
		}
	}

	var recipes []*models.Recipe

	if useIndex {
		// Fetch specific recipes by ID
		for _, id := range recipeIDs {
			recipe, err := s.GetRecipe(ctx, id)
			if err == nil && s.matchesFilter(recipe, filter) {
				recipes = append(recipes, recipe)
			}
		}
	} else {
		// List all recipes and filter in memory
		objects, err := s.client.ListObjects(s.bucketName, s.keyPrefix+"/")
		if err != nil {
			return nil, fmt.Errorf("failed to list recipes: %w", err)
		}

		for _, obj := range objects {
			// Skip metadata files and deletion markers
			if strings.Contains(obj.Key, "/latest") || strings.Contains(obj.Key, "/versions/") || strings.Contains(obj.Key, "/.deleted/") {
				continue
			}

			// Extract recipe ID from key
			parts := strings.Split(obj.Key, "/")
			if len(parts) < 2 {
				continue
			}
			recipeID := parts[len(parts)-1]
			recipeID = strings.TrimSuffix(recipeID, ".yaml")

			// Skip deleted recipes
			if s.isRecipeDeleted(recipeID) {
				continue
			}

			recipe, err := s.GetRecipe(ctx, recipeID)
			if err == nil && s.matchesFilter(recipe, filter) {
				recipes = append(recipes, recipe)
			}
		}
	}

	// Apply pagination
	start := filter.Offset
	end := start + filter.Limit
	if end > len(recipes) || filter.Limit == 0 {
		end = len(recipes)
	}
	if start > len(recipes) {
		start = len(recipes)
	}

	return recipes[start:end], nil
}

// SearchRecipes performs full-text search
func (s *SeaweedFSRecipeStorage) SearchRecipes(ctx context.Context, query string) ([]*RecipeSearchResult, error) {
	if s.indexStore == nil {
		return nil, fmt.Errorf("search not available: index store not configured")
	}

	// Perform full-text search using index
	recipeIDs, err := s.indexStore.FullTextSearch(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("search failed: %w", err)
	}

	// Fetch recipes and calculate relevance scores
	var results []*RecipeSearchResult
	queryLower := strings.ToLower(query)

	for _, id := range recipeIDs {
		recipe, err := s.GetRecipe(ctx, id)
		if err != nil {
			continue
		}

		// Calculate simple relevance score
		score := s.calculateRelevanceScore(recipe, queryLower)
		results = append(results, &RecipeSearchResult{
			Recipe: recipe,
			Score:  score,
		})
	}

	// Sort by relevance score
	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})

	return results, nil
}

// GetRecipeVersions gets all versions of a recipe
func (s *SeaweedFSRecipeStorage) GetRecipeVersions(ctx context.Context, name string) ([]*models.Recipe, error) {
	// List all version pointers
	versionPrefix := fmt.Sprintf("%s/%s/versions/", s.keyPrefix, name)
	objects, err := s.client.ListObjects(s.bucketName, versionPrefix)
	if err != nil {
		return nil, fmt.Errorf("failed to list recipe versions: %w", err)
	}

	var recipes []*models.Recipe
	for _, obj := range objects {
		// Read version pointer to get recipe ID
		data, err := s.getObjectContent(obj.Key)
		if err != nil {
			continue
		}

		recipeID := string(data)
		recipe, err := s.GetRecipe(ctx, recipeID)
		if err == nil {
			recipes = append(recipes, recipe)
		}
	}

	// Sort by version (newest first)
	sort.Slice(recipes, func(i, j int) bool {
		return recipes[i].Metadata.Version > recipes[j].Metadata.Version
	})

	return recipes, nil
}

// GetLatestRecipe gets the latest version of a recipe
func (s *SeaweedFSRecipeStorage) GetLatestRecipe(ctx context.Context, name string) (*models.Recipe, error) {
	latestKey := s.getLatestKey(name)
	idData, err := s.getObjectContent(latestKey)
	if err != nil {
		return nil, fmt.Errorf("latest version not found for recipe: %s", name)
	}

	recipeID := string(idData)
	return s.GetRecipe(ctx, recipeID)
}

// ImportRecipes imports multiple recipes
func (s *SeaweedFSRecipeStorage) ImportRecipes(ctx context.Context, recipes []*models.Recipe) error {
	for _, recipe := range recipes {
		if err := s.CreateRecipe(ctx, recipe); err != nil {
			// Log error but continue with other recipes
			fmt.Printf("Warning: failed to import recipe %s: %v\n", recipe.Metadata.Name, err)
		}
	}
	return nil
}

// ExportRecipes exports recipes matching filter
func (s *SeaweedFSRecipeStorage) ExportRecipes(ctx context.Context, filter RecipeFilter) ([]*models.Recipe, error) {
	return s.ListRecipes(ctx, filter)
}

// ValidateRecipe validates a recipe
func (s *SeaweedFSRecipeStorage) ValidateRecipe(ctx context.Context, recipe *models.Recipe) error {
	if s.validator == nil {
		return nil
	}
	return s.validator.ValidateRecipe(recipe)
}

// CheckRecipeIntegrity checks recipe integrity
func (s *SeaweedFSRecipeStorage) CheckRecipeIntegrity(ctx context.Context, id string) error {
	recipe, err := s.GetRecipe(ctx, id)
	if err != nil {
		return err
	}

	// Calculate current hash
	currentHash, err := recipe.CalculateHash()
	if err != nil {
		return fmt.Errorf("failed to calculate hash: %w", err)
	}

	// Compare with stored hash
	if recipe.Hash != "" && recipe.Hash != currentHash {
		return fmt.Errorf("integrity check failed: hash mismatch")
	}

	return nil
}

// VerifyRecipeHash verifies recipe hash
func (s *SeaweedFSRecipeStorage) VerifyRecipeHash(ctx context.Context, id string, expectedHash string) (bool, error) {
	recipe, err := s.GetRecipe(ctx, id)
	if err != nil {
		return false, err
	}

	currentHash, err := recipe.CalculateHash()
	if err != nil {
		return false, fmt.Errorf("failed to calculate hash: %w", err)
	}

	return currentHash == expectedHash, nil
}

// RebuildIndex rebuilds the recipe index
func (s *SeaweedFSRecipeStorage) RebuildIndex(ctx context.Context) error {
	if s.indexStore == nil {
		return fmt.Errorf("index store not configured")
	}

	// Clear existing index
	if err := s.indexStore.ClearIndex(ctx); err != nil {
		return fmt.Errorf("failed to clear index: %w", err)
	}

	// List all recipes
	filter := RecipeFilter{Limit: 0} // No limit
	recipes, err := s.ListRecipes(ctx, filter)
	if err != nil {
		return fmt.Errorf("failed to list recipes: %w", err)
	}

	// Re-index each recipe
	for _, recipe := range recipes {
		if err := s.indexStore.UpdateIndex(ctx, recipe, IndexActionAdd); err != nil {
			fmt.Printf("Warning: failed to index recipe %s: %v\n", recipe.ID, err)
		}
	}

	return nil
}

// UpdateIndex updates the recipe index
func (s *SeaweedFSRecipeStorage) UpdateIndex(ctx context.Context, recipe *models.Recipe, action IndexAction) error {
	if s.indexStore == nil {
		return nil
	}
	return s.indexStore.UpdateIndex(ctx, recipe, action)
}

// Helper methods

func (s *SeaweedFSRecipeStorage) getRecipeKey(id string) string {
	return fmt.Sprintf("%s/%s.yaml", s.keyPrefix, id)
}

func (s *SeaweedFSRecipeStorage) getVersionKey(name, version string) string {
	return fmt.Sprintf("%s/%s/versions/%s", s.keyPrefix, name, version)
}

func (s *SeaweedFSRecipeStorage) getLatestKey(name string) string {
	return fmt.Sprintf("%s/%s/latest", s.keyPrefix, name)
}

func (s *SeaweedFSRecipeStorage) getDeletionKey(id string) string {
	return fmt.Sprintf("%s/.deleted/%s.yaml", s.keyPrefix, id)
}

func (s *SeaweedFSRecipeStorage) isRecipeDeleted(id string) bool {
	deletionKey := s.getDeletionKey(id)
	_, err := s.getObjectContent(deletionKey)
	return err == nil // If deletion marker exists, recipe is deleted
}

func (s *SeaweedFSRecipeStorage) getObjectContent(key string) ([]byte, error) {
	reader, err := s.client.GetObject(s.bucketName, key)
	if err != nil {
		return nil, err
	}
	defer reader.Close()

	data, err := io.ReadAll(reader)
	if err != nil {
		return nil, err
	}

	return data, nil
}

func (s *SeaweedFSRecipeStorage) matchesFilter(recipe *models.Recipe, filter RecipeFilter) bool {
	// Check tags
	if len(filter.Tags) > 0 {
		hasTag := false
		for _, tag := range filter.Tags {
			if recipe.Metadata.HasTag(tag) {
				hasTag = true
				break
			}
		}
		if !hasTag {
			return false
		}
	}

	// Check categories
	if len(filter.Categories) > 0 {
		hasCategory := false
		for _, category := range filter.Categories {
			if recipe.Metadata.HasCategory(category) {
				hasCategory = true
				break
			}
		}
		if !hasCategory {
			return false
		}
	}

	// Check languages
	if len(filter.Languages) > 0 {
		hasLanguage := false
		for _, language := range filter.Languages {
			if recipe.Metadata.SupportsLanguage(language) {
				hasLanguage = true
				break
			}
		}
		if !hasLanguage {
			return false
		}
	}

	// Check frameworks
	if len(filter.Frameworks) > 0 {
		hasFramework := false
		for _, framework := range filter.Frameworks {
			if recipe.Metadata.SupportsFramework(framework) {
				hasFramework = true
				break
			}
		}
		if !hasFramework {
			return false
		}
	}

	// Check author
	if filter.Author != "" && !strings.EqualFold(recipe.Metadata.Author, filter.Author) {
		return false
	}

	// Check age
	if filter.MaxAge > 0 {
		age := time.Since(recipe.UpdatedAt)
		if age > filter.MaxAge {
			return false
		}
	}

	return true
}

func (s *SeaweedFSRecipeStorage) calculateRelevanceScore(recipe *models.Recipe, query string) float64 {
	score := 0.0

	// Name match (highest weight)
	if strings.Contains(strings.ToLower(recipe.Metadata.Name), query) {
		score += 10.0
	}

	// Description match
	if strings.Contains(strings.ToLower(recipe.Metadata.Description), query) {
		score += 5.0
	}

	// Tag match
	for _, tag := range recipe.Metadata.Tags {
		if strings.Contains(strings.ToLower(tag), query) {
			score += 3.0
		}
	}

	// Category match
	for _, category := range recipe.Metadata.Categories {
		if strings.Contains(strings.ToLower(category), query) {
			score += 2.0
		}
	}

	// Author match
	if strings.Contains(strings.ToLower(recipe.Metadata.Author), query) {
		score += 1.0
	}

	return score
}

// Cache methods

func (c *recipeCache) get(id string) *models.Recipe {
	c.mu.RLock()
	defer c.mu.RUnlock()

	entry, exists := c.recipes[id]
	if !exists {
		return nil
	}

	// Check if cache entry has expired
	if time.Since(entry.timestamp) > c.ttl {
		return nil
	}

	return entry.recipe
}

func (c *recipeCache) set(id string, recipe *models.Recipe) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.recipes[id] = &cacheEntry{
		recipe:    recipe,
		timestamp: time.Now(),
	}
}

func (c *recipeCache) delete(id string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	delete(c.recipes, id)
}
