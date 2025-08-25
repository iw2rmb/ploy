package arf

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/iw2rmb/ploy/controller/arf/models"
	"github.com/iw2rmb/ploy/controller/arf/storage"
)

// InMemoryRecipeStorage provides a simple in-memory implementation of RecipeStorage
// This is used for testing and as a temporary solution
type InMemoryRecipeStorage struct {
	recipes map[string]*models.Recipe
	mu      sync.RWMutex
}

// NewInMemoryRecipeStorage creates a new in-memory recipe storage
func NewInMemoryRecipeStorage() *InMemoryRecipeStorage {
	return &InMemoryRecipeStorage{
		recipes: make(map[string]*models.Recipe),
	}
}

// CreateRecipe creates a new recipe in storage
func (s *InMemoryRecipeStorage) CreateRecipe(ctx context.Context, recipe *models.Recipe) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if recipe.ID == "" {
		recipe.ID = recipe.GenerateID()
	}
	
	if _, exists := s.recipes[recipe.ID]; exists {
		return fmt.Errorf("recipe %s already exists", recipe.ID)
	}

	recipe.CreatedAt = time.Now()
	recipe.UpdatedAt = time.Now()
	s.recipes[recipe.ID] = recipe
	return nil
}

// GetRecipe retrieves a recipe by ID
func (s *InMemoryRecipeStorage) GetRecipe(ctx context.Context, id string) (*models.Recipe, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	recipe, exists := s.recipes[id]
	if !exists {
		return nil, fmt.Errorf("recipe not found: %s", id)
	}
	return recipe, nil
}

// GetRecipeByNameAndVersion retrieves a recipe by name and version
func (s *InMemoryRecipeStorage) GetRecipeByNameAndVersion(ctx context.Context, name, version string) (*models.Recipe, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, recipe := range s.recipes {
		if recipe.Metadata.Name == name && recipe.Metadata.Version == version {
			return recipe, nil
		}
	}
	return nil, fmt.Errorf("recipe not found: %s@%s", name, version)
}

// UpdateRecipe updates an existing recipe
func (s *InMemoryRecipeStorage) UpdateRecipe(ctx context.Context, id string, recipe *models.Recipe) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.recipes[id]; !exists {
		return fmt.Errorf("recipe %s not found", id)
	}

	recipe.ID = id
	recipe.UpdatedAt = time.Now()
	s.recipes[id] = recipe
	return nil
}

// DeleteRecipe deletes a recipe
func (s *InMemoryRecipeStorage) DeleteRecipe(ctx context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.recipes, id)
	return nil
}

// ListRecipes lists recipes with filtering
func (s *InMemoryRecipeStorage) ListRecipes(ctx context.Context, filter storage.RecipeFilter) ([]*models.Recipe, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var results []*models.Recipe
	for _, recipe := range s.recipes {
		results = append(results, recipe)
	}
	return results, nil
}

// SearchRecipes performs full-text search
func (s *InMemoryRecipeStorage) SearchRecipes(ctx context.Context, query string) ([]*storage.RecipeSearchResult, error) {
	return nil, fmt.Errorf("search not implemented in memory storage")
}

// GetRecipeVersions gets all versions of a recipe
func (s *InMemoryRecipeStorage) GetRecipeVersions(ctx context.Context, name string) ([]*models.Recipe, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var results []*models.Recipe
	for _, recipe := range s.recipes {
		if recipe.Metadata.Name == name {
			results = append(results, recipe)
		}
	}
	return results, nil
}

// GetLatestRecipe gets the latest version of a recipe
func (s *InMemoryRecipeStorage) GetLatestRecipe(ctx context.Context, name string) (*models.Recipe, error) {
	versions, err := s.GetRecipeVersions(ctx, name)
	if err != nil {
		return nil, err
	}
	if len(versions) == 0 {
		return nil, fmt.Errorf("no versions found for recipe %s", name)
	}
	return versions[0], nil
}

// ImportRecipes imports multiple recipes
func (s *InMemoryRecipeStorage) ImportRecipes(ctx context.Context, recipes []*models.Recipe) error {
	for _, recipe := range recipes {
		if err := s.CreateRecipe(ctx, recipe); err != nil {
			return err
		}
	}
	return nil
}

// ExportRecipes exports recipes matching filter
func (s *InMemoryRecipeStorage) ExportRecipes(ctx context.Context, filter storage.RecipeFilter) ([]*models.Recipe, error) {
	return s.ListRecipes(ctx, filter)
}

// ValidateRecipe validates a recipe
func (s *InMemoryRecipeStorage) ValidateRecipe(ctx context.Context, recipe *models.Recipe) error {
	return recipe.Validate()
}

// CheckRecipeIntegrity checks recipe integrity
func (s *InMemoryRecipeStorage) CheckRecipeIntegrity(ctx context.Context, id string) error {
	recipe, err := s.GetRecipe(ctx, id)
	if err != nil {
		return err
	}
	
	currentHash, err := recipe.CalculateHash()
	if err != nil {
		return err
	}
	
	if recipe.Hash != "" && recipe.Hash != currentHash {
		return fmt.Errorf("integrity check failed")
	}
	
	return nil
}

// VerifyRecipeHash verifies recipe hash
func (s *InMemoryRecipeStorage) VerifyRecipeHash(ctx context.Context, id string, expectedHash string) (bool, error) {
	recipe, err := s.GetRecipe(ctx, id)
	if err != nil {
		return false, err
	}
	
	currentHash, err := recipe.CalculateHash()
	if err != nil {
		return false, err
	}
	
	return currentHash == expectedHash, nil
}

// RebuildIndex rebuilds the recipe index (no-op for in-memory)
func (s *InMemoryRecipeStorage) RebuildIndex(ctx context.Context) error {
	return nil
}

// UpdateIndex updates the recipe index (no-op for in-memory)
func (s *InMemoryRecipeStorage) UpdateIndex(ctx context.Context, recipe *models.Recipe, action storage.IndexAction) error {
	return nil
}

