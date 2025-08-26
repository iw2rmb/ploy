package arf

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/hashicorp/consul/api"
	"github.com/iw2rmb/ploy/api/arf/models"
)

// RecipeCatalog provides recipe storage and discovery capabilities
type RecipeCatalog interface {
	StoreRecipe(ctx context.Context, recipe *models.Recipe) error
	GetRecipe(ctx context.Context, recipeID string) (*models.Recipe, error)
	ListRecipes(ctx context.Context, filters RecipeFilters) ([]*models.Recipe, error)
	UpdateRecipe(ctx context.Context, recipe *models.Recipe) error
	DeleteRecipe(ctx context.Context, recipeID string) error
	SearchRecipes(ctx context.Context, query string) ([]*models.Recipe, error)
	GetRecipeStats(ctx context.Context, recipeID string) (*RecipeStats, error)
	UpdateRecipeStats(ctx context.Context, recipeID string, success bool, executionTime time.Duration) error
}

// RecipeFilters defines search criteria for recipes
type RecipeFilters struct {
	Language      string   `json:"language,omitempty"`
	Category      string   `json:"category,omitempty"`
	Tags          []string `json:"tags,omitempty"`
	Author        string   `json:"author,omitempty"`
	MinConfidence float64  `json:"min_confidence,omitempty"`
	MaxConfidence float64  `json:"max_confidence,omitempty"`
}

// RecipeStats tracks usage and performance metrics for recipes
type RecipeStats struct {
	RecipeID        string        `json:"recipe_id"`
	TotalExecutions int64         `json:"total_executions"`
	SuccessfulRuns  int64         `json:"successful_runs"`
	FailedRuns      int64         `json:"failed_runs"`
	SuccessRate     float64       `json:"success_rate"`
	AvgExecutionTime time.Duration `json:"avg_execution_time"`
	LastExecuted    time.Time     `json:"last_executed"`
	FirstExecuted   time.Time     `json:"first_executed"`
}

// ConsulRecipeCatalog implements RecipeCatalog using Consul KV store
type ConsulRecipeCatalog struct {
	client     *api.Client
	keyPrefix  string
	statsPrefix string
}

// NewConsulRecipeCatalog creates a new Consul-based recipe catalog
func NewConsulRecipeCatalog(consulAddr, keyPrefix string) (*ConsulRecipeCatalog, error) {
	config := api.DefaultConfig()
	if consulAddr != "" {
		config.Address = consulAddr
	}

	client, err := api.NewClient(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create Consul client: %w", err)
	}

	catalog := &ConsulRecipeCatalog{
		client:      client,
		keyPrefix:   keyPrefix + "/recipes",
		statsPrefix: keyPrefix + "/stats",
	}

	return catalog, nil
}

// StoreRecipe stores a recipe in the Consul KV store
func (c *ConsulRecipeCatalog) StoreRecipe(ctx context.Context, recipe *models.Recipe) error {
	// Validate recipe before storing
	if recipe.ID == "" {
		return fmt.Errorf("recipe ID is required")
	}

	// Serialize recipe to JSON
	data, err := json.Marshal(recipe)
	if err != nil {
		return fmt.Errorf("failed to serialize recipe: %w", err)
	}

	// Store in Consul
	key := fmt.Sprintf("%s/%s", c.keyPrefix, recipe.ID)
	pair := &api.KVPair{
		Key:   key,
		Value: data,
	}

	kv := c.client.KV()
	_, err = kv.Put(pair, nil)
	if err != nil {
		return fmt.Errorf("failed to store recipe in Consul: %w", err)
	}

	// Initialize stats for new recipe
	stats := &RecipeStats{
		RecipeID:      recipe.ID,
		FirstExecuted: time.Now(),
	}
	
	if err := c.storeRecipeStats(ctx, stats); err != nil {
		// Log error but don't fail recipe storage
		fmt.Printf("Warning: failed to initialize recipe stats: %v\n", err)
	}

	return nil
}

// GetRecipe retrieves a recipe by ID
func (c *ConsulRecipeCatalog) GetRecipe(ctx context.Context, recipeID string) (*models.Recipe, error) {
	key := fmt.Sprintf("%s/%s", c.keyPrefix, recipeID)
	kv := c.client.KV()

	pair, _, err := kv.Get(key, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get recipe from Consul: %w", err)
	}

	if pair == nil {
		return nil, fmt.Errorf("recipe %s not found", recipeID)
	}

	var recipe models.Recipe
	if err := json.Unmarshal(pair.Value, &recipe); err != nil {
		return nil, fmt.Errorf("failed to deserialize recipe: %w", err)
	}

	return &recipe, nil
}

// ListRecipes returns all recipes matching the given filters
func (c *ConsulRecipeCatalog) ListRecipes(ctx context.Context, filters RecipeFilters) ([]*models.Recipe, error) {
	kv := c.client.KV()
	pairs, _, err := kv.List(c.keyPrefix, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to list recipes from Consul: %w", err)
	}

	var recipes []*models.Recipe
	for _, pair := range pairs {
		var recipe models.Recipe
		if err := json.Unmarshal(pair.Value, &recipe); err != nil {
			continue // Skip malformed recipes
		}

		if c.matchesFilters(&recipe, filters) {
			recipes = append(recipes, &recipe)
		}
	}

	return recipes, nil
}

// UpdateRecipe updates an existing recipe
func (c *ConsulRecipeCatalog) UpdateRecipe(ctx context.Context, recipe *models.Recipe) error {
	// Check if recipe exists
	existing, err := c.GetRecipe(ctx, recipe.ID)
	if err != nil {
		return fmt.Errorf("recipe %s not found: %w", recipe.ID, err)
	}

	// Preserve creation timestamp if it exists
	if existing != nil && recipe.Version == "" {
		recipe.Version = existing.Version
	}

	return c.StoreRecipe(ctx, recipe)
}

// DeleteRecipe removes a recipe from the catalog
func (c *ConsulRecipeCatalog) DeleteRecipe(ctx context.Context, recipeID string) error {
	key := fmt.Sprintf("%s/%s", c.keyPrefix, recipeID)
	kv := c.client.KV()

	_, err := kv.Delete(key, nil)
	if err != nil {
		return fmt.Errorf("failed to delete recipe from Consul: %w", err)
	}

	// Also delete associated stats
	statsKey := fmt.Sprintf("%s/%s", c.statsPrefix, recipeID)
	_, err = kv.Delete(statsKey, nil)
	if err != nil {
		// Log error but don't fail deletion
		fmt.Printf("Warning: failed to delete recipe stats: %v\n", err)
	}

	return nil
}

// SearchRecipes performs a text search across recipe names and descriptions
func (c *ConsulRecipeCatalog) SearchRecipes(ctx context.Context, query string) ([]*models.Recipe, error) {
	queryLower := strings.ToLower(query)
	
	kv := c.client.KV()
	pairs, _, err := kv.List(c.keyPrefix, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to search recipes in Consul: %w", err)
	}

	recipes := []*models.Recipe{}
	for _, pair := range pairs {
		var recipe models.Recipe
		if err := json.Unmarshal(pair.Value, &recipe); err != nil {
			continue
		}

		// Search in name, description, and tags
		if c.containsQuery(recipe.Metadata.Name, queryLower) ||
		   c.containsQuery(recipe.Metadata.Description, queryLower) ||
		   c.containsQueryInTags(recipe.Metadata.Tags, queryLower) {
			recipes = append(recipes, &recipe)
		}
	}

	return recipes, nil
}

// GetRecipeStats retrieves usage statistics for a recipe
func (c *ConsulRecipeCatalog) GetRecipeStats(ctx context.Context, recipeID string) (*RecipeStats, error) {
	key := fmt.Sprintf("%s/%s", c.statsPrefix, recipeID)
	kv := c.client.KV()

	pair, _, err := kv.Get(key, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get recipe stats from Consul: %w", err)
	}

	if pair == nil {
		// Return empty stats if none exist
		return &RecipeStats{
			RecipeID: recipeID,
		}, nil
	}

	var stats RecipeStats
	if err := json.Unmarshal(pair.Value, &stats); err != nil {
		return nil, fmt.Errorf("failed to deserialize recipe stats: %w", err)
	}

	return &stats, nil
}

// UpdateRecipeStats updates usage statistics for a recipe
func (c *ConsulRecipeCatalog) UpdateRecipeStats(ctx context.Context, recipeID string, success bool, executionTime time.Duration) error {
	// Get current stats
	stats, err := c.GetRecipeStats(ctx, recipeID)
	if err != nil {
		return fmt.Errorf("failed to get current stats: %w", err)
	}

	// Update stats
	stats.TotalExecutions++
	if success {
		stats.SuccessfulRuns++
	} else {
		stats.FailedRuns++
	}

	// Calculate success rate
	if stats.TotalExecutions > 0 {
		stats.SuccessRate = float64(stats.SuccessfulRuns) / float64(stats.TotalExecutions)
	}

	// Update average execution time
	if stats.TotalExecutions == 1 {
		stats.AvgExecutionTime = executionTime
	} else {
		// Incremental average calculation
		totalTime := stats.AvgExecutionTime * time.Duration(stats.TotalExecutions-1)
		stats.AvgExecutionTime = (totalTime + executionTime) / time.Duration(stats.TotalExecutions)
	}

	stats.LastExecuted = time.Now()
	if stats.FirstExecuted.IsZero() {
		stats.FirstExecuted = stats.LastExecuted
	}

	return c.storeRecipeStats(ctx, stats)
}

// Helper methods

func (c *ConsulRecipeCatalog) storeRecipeStats(ctx context.Context, stats *RecipeStats) error {
	data, err := json.Marshal(stats)
	if err != nil {
		return fmt.Errorf("failed to serialize recipe stats: %w", err)
	}

	key := fmt.Sprintf("%s/%s", c.statsPrefix, stats.RecipeID)
	pair := &api.KVPair{
		Key:   key,
		Value: data,
	}

	kv := c.client.KV()
	_, err = kv.Put(pair, nil)
	if err != nil {
		return fmt.Errorf("failed to store recipe stats in Consul: %w", err)
	}

	return nil
}

func (c *ConsulRecipeCatalog) matchesFilters(recipe *models.Recipe, filters RecipeFilters) bool {
	// Language filter
	if filters.Language != "" {
		hasLanguage := false
		for _, lang := range recipe.Metadata.Languages {
			if lang == filters.Language {
				hasLanguage = true
				break
			}
		}
		if !hasLanguage {
			return false
		}
	}

	// Category filter
	if filters.Category != "" {
		hasCategory := false
		for _, cat := range recipe.Metadata.Categories {
			if cat == filters.Category {
				hasCategory = true
				break
			}
		}
		if !hasCategory {
			return false
		}
	}

	// Author filter
	if filters.Author != "" && recipe.Metadata.Author != filters.Author {
		return false
	}

	// Tags filter
	if len(filters.Tags) > 0 {
		hasAllTags := true
		for _, filterTag := range filters.Tags {
			found := false
			for _, recipeTag := range recipe.Metadata.Tags {
				if strings.EqualFold(recipeTag, filterTag) {
					found = true
					break
				}
			}
			if !found {
				hasAllTags = false
				break
			}
		}
		if !hasAllTags {
			return false
		}
	}

	return true
}

func (c *ConsulRecipeCatalog) containsQuery(text, query string) bool {
	return strings.Contains(strings.ToLower(text), query)
}

func (c *ConsulRecipeCatalog) containsQueryInTags(tags []string, query string) bool {
	for _, tag := range tags {
		if c.containsQuery(tag, query) {
			return true
		}
	}
	return false
}