package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/hashicorp/consul/api"
	"github.com/iw2rmb/ploy/controller/arf/models"
)

// ConsulRecipeIndex implements RecipeIndexStore using Consul KV
type ConsulRecipeIndex struct {
	client    *api.Client
	keyPrefix string
	mu        sync.RWMutex
}

// RecipeIndexEntry stores recipe metadata in the index
type RecipeIndexEntry struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Author      string    `json:"author"`
	Version     string    `json:"version"`
	Tags        []string  `json:"tags"`
	Categories  []string  `json:"categories"`
	Languages   []string  `json:"languages"`
	Frameworks  []string  `json:"frameworks"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// NewConsulRecipeIndex creates a new Consul-based recipe index
func NewConsulRecipeIndex(consulAddr, keyPrefix string) (*ConsulRecipeIndex, error) {
	config := api.DefaultConfig()
	if consulAddr != "" {
		config.Address = consulAddr
	}

	client, err := api.NewClient(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create Consul client: %w", err)
	}

	if keyPrefix == "" {
		keyPrefix = "ploy/arf/recipes"
	}

	return &ConsulRecipeIndex{
		client:    client,
		keyPrefix: keyPrefix,
	}, nil
}

// retryConsulOperation retries a Consul operation with exponential backoff
func (c *ConsulRecipeIndex) retryConsulOperation(operation func() error, maxRetries int) error {
	var lastErr error
	for attempt := 0; attempt < maxRetries; attempt++ {
		if err := operation(); err != nil {
			lastErr = err
			if attempt < maxRetries-1 {
				backoff := time.Duration(1<<attempt) * 50 * time.Millisecond
				if backoff > 2*time.Second {
					backoff = 2 * time.Second
				}
				time.Sleep(backoff)
				continue
			}
		} else {
			return nil
		}
	}
	return fmt.Errorf("consul operation failed after %d attempts: %w", maxRetries, lastErr)
}

// BuildIndex builds the complete index from scratch
func (c *ConsulRecipeIndex) BuildIndex(ctx context.Context) error {
	// This would typically be called during initialization
	// The actual recipes would be provided by the storage layer
	return nil
}

// UpdateIndex updates the index for a specific recipe
func (c *ConsulRecipeIndex) UpdateIndex(ctx context.Context, recipe *models.Recipe, action IndexAction) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	switch action {
	case IndexActionAdd, IndexActionUpdate:
		return c.addOrUpdateRecipeIndex(ctx, recipe)
	case IndexActionRemove:
		return c.removeRecipeIndex(ctx, recipe)
	default:
		return fmt.Errorf("unknown index action: %s", action)
	}
}

// ClearIndex removes all index entries
func (c *ConsulRecipeIndex) ClearIndex(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Delete all keys under the prefix
	_, err := c.client.KV().DeleteTree(c.keyPrefix, nil)
	if err != nil {
		return fmt.Errorf("failed to clear index: %w", err)
	}

	return nil
}

// QueryByTags queries recipes by tags
func (c *ConsulRecipeIndex) QueryByTags(ctx context.Context, tags []string) ([]string, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	recipeIDMap := make(map[string]bool)

	for _, tag := range tags {
		key := fmt.Sprintf("%s/index/by-tag/%s/", c.keyPrefix, normalizeKey(tag))
		pairs, _, err := c.client.KV().List(key, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to query by tag %s: %w", tag, err)
		}

		for _, pair := range pairs {
			// Extract recipe ID from the key
			parts := strings.Split(pair.Key, "/")
			if len(parts) > 0 {
				recipeID := parts[len(parts)-1]
				recipeIDMap[recipeID] = true
			}
		}
	}

	// Convert map to slice
	recipeIDs := make([]string, 0, len(recipeIDMap))
	for id := range recipeIDMap {
		recipeIDs = append(recipeIDs, id)
	}

	return recipeIDs, nil
}

// QueryByLanguage queries recipes by language
func (c *ConsulRecipeIndex) QueryByLanguage(ctx context.Context, language string) ([]string, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	key := fmt.Sprintf("%s/index/by-language/%s/", c.keyPrefix, normalizeKey(language))
	pairs, _, err := c.client.KV().List(key, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to query by language %s: %w", language, err)
	}

	recipeIDs := make([]string, 0, len(pairs))
	for _, pair := range pairs {
		parts := strings.Split(pair.Key, "/")
		if len(parts) > 0 {
			recipeIDs = append(recipeIDs, parts[len(parts)-1])
		}
	}

	return recipeIDs, nil
}

// QueryByCategory queries recipes by category
func (c *ConsulRecipeIndex) QueryByCategory(ctx context.Context, category string) ([]string, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	key := fmt.Sprintf("%s/index/by-category/%s/", c.keyPrefix, normalizeKey(category))
	pairs, _, err := c.client.KV().List(key, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to query by category %s: %w", category, err)
	}

	recipeIDs := make([]string, 0, len(pairs))
	for _, pair := range pairs {
		parts := strings.Split(pair.Key, "/")
		if len(parts) > 0 {
			recipeIDs = append(recipeIDs, parts[len(parts)-1])
		}
	}

	return recipeIDs, nil
}

// QueryByAuthor queries recipes by author
func (c *ConsulRecipeIndex) QueryByAuthor(ctx context.Context, author string) ([]string, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	key := fmt.Sprintf("%s/index/by-author/%s/", c.keyPrefix, normalizeKey(author))
	pairs, _, err := c.client.KV().List(key, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to query by author %s: %w", author, err)
	}

	recipeIDs := make([]string, 0, len(pairs))
	for _, pair := range pairs {
		parts := strings.Split(pair.Key, "/")
		if len(parts) > 0 {
			recipeIDs = append(recipeIDs, parts[len(parts)-1])
		}
	}

	return recipeIDs, nil
}

// FullTextSearch performs full-text search across recipe metadata
func (c *ConsulRecipeIndex) FullTextSearch(ctx context.Context, query string) ([]string, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	// Get all recipe metadata entries
	metadataKey := fmt.Sprintf("%s/metadata/", c.keyPrefix)
	pairs, _, err := c.client.KV().List(metadataKey, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to list recipe metadata: %w", err)
	}

	queryLower := strings.ToLower(query)
	queryTerms := strings.Fields(queryLower) // Split into words

	type searchResult struct {
		id    string
		score int
	}

	results := make([]searchResult, 0)

	for _, pair := range pairs {
		var entry RecipeIndexEntry
		if err := json.Unmarshal(pair.Value, &entry); err != nil {
			continue
		}

		// Calculate relevance score
		score := 0

		// Check name (highest weight)
		nameLower := strings.ToLower(entry.Name)
		for _, term := range queryTerms {
			if strings.Contains(nameLower, term) {
				score += 10
			}
		}

		// Check description
		descLower := strings.ToLower(entry.Description)
		for _, term := range queryTerms {
			if strings.Contains(descLower, term) {
				score += 5
			}
		}

		// Check tags
		for _, tag := range entry.Tags {
			tagLower := strings.ToLower(tag)
			for _, term := range queryTerms {
				if strings.Contains(tagLower, term) {
					score += 3
				}
			}
		}

		// Check categories
		for _, category := range entry.Categories {
			catLower := strings.ToLower(category)
			for _, term := range queryTerms {
				if strings.Contains(catLower, term) {
					score += 2
				}
			}
		}

		// Check author
		authorLower := strings.ToLower(entry.Author)
		for _, term := range queryTerms {
			if strings.Contains(authorLower, term) {
				score += 1
			}
		}

		if score > 0 {
			results = append(results, searchResult{
				id:    entry.ID,
				score: score,
			})
		}
	}

	// Sort by score (highest first)
	sort.Slice(results, func(i, j int) bool {
		return results[i].score > results[j].score
	})

	// Extract IDs
	recipeIDs := make([]string, 0, len(results))
	for _, result := range results {
		recipeIDs = append(recipeIDs, result.id)
	}

	return recipeIDs, nil
}

// GetIndexStats returns index statistics
func (c *ConsulRecipeIndex) GetIndexStats(ctx context.Context) (*IndexStats, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	stats := &IndexStats{
		LastUpdated: time.Now(),
	}

	// Count total recipes
	metadataKey := fmt.Sprintf("%s/metadata/", c.keyPrefix)
	pairs, _, err := c.client.KV().List(metadataKey, nil)
	if err == nil {
		stats.TotalRecipes = len(pairs)
	}

	// Count unique tags
	tagKey := fmt.Sprintf("%s/index/by-tag/", c.keyPrefix)
	tagDirs, _, err := c.client.KV().Keys(tagKey, "/", nil)
	if err == nil {
		stats.TotalTags = len(tagDirs)
	}

	// Count unique categories
	catKey := fmt.Sprintf("%s/index/by-category/", c.keyPrefix)
	catDirs, _, err := c.client.KV().Keys(catKey, "/", nil)
	if err == nil {
		stats.TotalCategories = len(catDirs)
	}

	// Count unique languages
	langKey := fmt.Sprintf("%s/index/by-language/", c.keyPrefix)
	langDirs, _, err := c.client.KV().Keys(langKey, "/", nil)
	if err == nil {
		stats.TotalLanguages = len(langDirs)
	}

	// Count unique authors
	authorKey := fmt.Sprintf("%s/index/by-author/", c.keyPrefix)
	authorDirs, _, err := c.client.KV().Keys(authorKey, "/", nil)
	if err == nil {
		stats.TotalAuthors = len(authorDirs)
	}

	// Estimate index size (simplified)
	stats.IndexSize = int64(stats.TotalRecipes * 1024) // Rough estimate

	return stats, nil
}

// Helper methods

func (c *ConsulRecipeIndex) addOrUpdateRecipeIndex(ctx context.Context, recipe *models.Recipe) error {
	// Create index entry
	entry := RecipeIndexEntry{
		ID:          recipe.ID,
		Name:        recipe.Metadata.Name,
		Description: recipe.Metadata.Description,
		Author:      recipe.Metadata.Author,
		Version:     recipe.Metadata.Version,
		Tags:        recipe.Metadata.Tags,
		Categories:  recipe.Metadata.Categories,
		Languages:   recipe.Metadata.Languages,
		Frameworks:  recipe.Metadata.Frameworks,
		UpdatedAt:   recipe.UpdatedAt,
	}

	// Store metadata
	metadataKey := fmt.Sprintf("%s/metadata/%s", c.keyPrefix, recipe.ID)
	metadataData, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("failed to marshal metadata: %w", err)
	}

	// Store metadata with retry
	err = c.retryConsulOperation(func() error {
		_, err := c.client.KV().Put(&api.KVPair{
			Key:   metadataKey,
			Value: metadataData,
		}, nil)
		return err
	}, 3)
	if err != nil {
		return fmt.Errorf("failed to store metadata: %w", err)
	}

	// Index by tags
	for _, tag := range recipe.Metadata.Tags {
		tagKey := fmt.Sprintf("%s/index/by-tag/%s/%s", c.keyPrefix, normalizeKey(tag), recipe.ID)
		if _, err := c.client.KV().Put(&api.KVPair{
			Key:   tagKey,
			Value: []byte("1"), // Simple marker
		}, nil); err != nil {
			fmt.Printf("Warning: failed to index tag %s: %v\n", tag, err)
		}
	}

	// Index by categories
	for _, category := range recipe.Metadata.Categories {
		catKey := fmt.Sprintf("%s/index/by-category/%s/%s", c.keyPrefix, normalizeKey(category), recipe.ID)
		if _, err := c.client.KV().Put(&api.KVPair{
			Key:   catKey,
			Value: []byte("1"),
		}, nil); err != nil {
			fmt.Printf("Warning: failed to index category %s: %v\n", category, err)
		}
	}

	// Index by languages
	for _, language := range recipe.Metadata.Languages {
		langKey := fmt.Sprintf("%s/index/by-language/%s/%s", c.keyPrefix, normalizeKey(language), recipe.ID)
		if _, err := c.client.KV().Put(&api.KVPair{
			Key:   langKey,
			Value: []byte("1"),
		}, nil); err != nil {
			fmt.Printf("Warning: failed to index language %s: %v\n", language, err)
		}
	}

	// Index by author
	if recipe.Metadata.Author != "" {
		authorKey := fmt.Sprintf("%s/index/by-author/%s/%s", c.keyPrefix, normalizeKey(recipe.Metadata.Author), recipe.ID)
		if _, err := c.client.KV().Put(&api.KVPair{
			Key:   authorKey,
			Value: []byte("1"),
		}, nil); err != nil {
			fmt.Printf("Warning: failed to index author %s: %v\n", recipe.Metadata.Author, err)
		}
	}

	// Index by frameworks
	for _, framework := range recipe.Metadata.Frameworks {
		frameworkKey := fmt.Sprintf("%s/index/by-framework/%s/%s", c.keyPrefix, normalizeKey(framework), recipe.ID)
		if _, err := c.client.KV().Put(&api.KVPair{
			Key:   frameworkKey,
			Value: []byte("1"),
		}, nil); err != nil {
			fmt.Printf("Warning: failed to index framework %s: %v\n", framework, err)
		}
	}

	return nil
}

func (c *ConsulRecipeIndex) removeRecipeIndex(ctx context.Context, recipe *models.Recipe) error {
	// Remove metadata
	metadataKey := fmt.Sprintf("%s/metadata/%s", c.keyPrefix, recipe.ID)
	if _, err := c.client.KV().Delete(metadataKey, nil); err != nil {
		return fmt.Errorf("failed to remove metadata: %w", err)
	}

	// Remove tag indices
	for _, tag := range recipe.Metadata.Tags {
		tagKey := fmt.Sprintf("%s/index/by-tag/%s/%s", c.keyPrefix, normalizeKey(tag), recipe.ID)
		c.client.KV().Delete(tagKey, nil)
	}

	// Remove category indices
	for _, category := range recipe.Metadata.Categories {
		catKey := fmt.Sprintf("%s/index/by-category/%s/%s", c.keyPrefix, normalizeKey(category), recipe.ID)
		c.client.KV().Delete(catKey, nil)
	}

	// Remove language indices
	for _, language := range recipe.Metadata.Languages {
		langKey := fmt.Sprintf("%s/index/by-language/%s/%s", c.keyPrefix, normalizeKey(language), recipe.ID)
		c.client.KV().Delete(langKey, nil)
	}

	// Remove author index
	if recipe.Metadata.Author != "" {
		authorKey := fmt.Sprintf("%s/index/by-author/%s/%s", c.keyPrefix, normalizeKey(recipe.Metadata.Author), recipe.ID)
		c.client.KV().Delete(authorKey, nil)
	}

	// Remove framework indices
	for _, framework := range recipe.Metadata.Frameworks {
		frameworkKey := fmt.Sprintf("%s/index/by-framework/%s/%s", c.keyPrefix, normalizeKey(framework), recipe.ID)
		c.client.KV().Delete(frameworkKey, nil)
	}

	return nil
}

// normalizeKey converts a string to a valid Consul key component
func normalizeKey(s string) string {
	// Convert to lowercase and replace spaces with hyphens
	s = strings.ToLower(s)
	s = strings.ReplaceAll(s, " ", "-")
	s = strings.ReplaceAll(s, "/", "-")
	s = strings.ReplaceAll(s, ".", "-")
	s = strings.ReplaceAll(s, "_", "-")
	
	// Remove any non-alphanumeric characters except hyphens
	result := strings.Builder{}
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			result.WriteRune(r)
		}
	}
	
	return result.String()
}