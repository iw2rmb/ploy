package recipes

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/iw2rmb/ploy/api/recipes/models"
	"golang.org/x/mod/semver"
)

// RegistryStorageAdapter adapts RecipeRegistry to the RecipeStorage interface
// to enforce SeaweedFS-backed storage only (no in-memory fallback).
type RegistryStorageAdapter struct {
	reg *RecipeRegistry
}

func NewRegistryStorageAdapter(reg *RecipeRegistry) *RegistryStorageAdapter {
	return &RegistryStorageAdapter{reg: reg}
}

// Registry returns the underlying recipe registry.
func (a *RegistryStorageAdapter) Registry() *RecipeRegistry {
	return a.reg
}

// CRUD
func (a *RegistryStorageAdapter) CreateRecipe(ctx context.Context, recipe *models.Recipe) error {
	return a.reg.StoreRecipe(ctx, recipe)
}

func (a *RegistryStorageAdapter) GetRecipe(ctx context.Context, id string) (*models.Recipe, error) {
	return a.reg.GetRecipeAsModelsRecipe(ctx, id)
}

func (a *RegistryStorageAdapter) GetRecipeByNameAndVersion(ctx context.Context, name, version string) (*models.Recipe, error) {
	// Registry stores canonical IDs; emulate by scanning list and matching name+version
	list, err := a.reg.ListRecipes(ctx, RecipeFilters{})
	if err != nil {
		return nil, err
	}
	for _, r := range list {
		if r.Metadata.Name == name && r.Metadata.Version == version {
			return r, nil
		}
	}
	return nil, fmt.Errorf("recipe not found: %s@%s", name, version)
}

func (a *RegistryStorageAdapter) UpdateRecipe(ctx context.Context, id string, recipe *models.Recipe) error {
	recipe.ID = id
	return a.reg.UpdateRecipe(ctx, recipe)
}

func (a *RegistryStorageAdapter) DeleteRecipe(ctx context.Context, id string) error {
	return a.reg.DeleteRecipe(ctx, id)
}

// Query
func (a *RegistryStorageAdapter) ListRecipes(ctx context.Context, filter RecipeFilter) ([]*models.Recipe, error) {
	// Map RecipeFilter to RecipeFilters
	filters := RecipeFilters{
		Language: filter.Language,
		Author:   filter.Author,
		Tags:     filter.Tags,
	}
	return a.reg.ListRecipes(ctx, filters)
}

func (a *RegistryStorageAdapter) SearchRecipes(ctx context.Context, query string) ([]*RecipeSearchResult, error) {
	list, err := a.reg.SearchRecipes(ctx, query)
	if err != nil {
		return nil, err
	}
	out := make([]*RecipeSearchResult, 0, len(list))
	for _, r := range list {
		out = append(out, &RecipeSearchResult{Recipe: r, Score: 1.0})
	}
	return out, nil
}

func (a *RegistryStorageAdapter) GetRecipeVersions(ctx context.Context, name string) ([]*models.Recipe, error) {
	list, err := a.reg.ListRecipes(ctx, RecipeFilters{})
	if err != nil {
		return nil, err
	}
	var out []*models.Recipe
	for _, r := range list {
		if r.Metadata.Name == name {
			out = append(out, r)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return compareRecipeVersions(out[i].Metadata.Version, out[j].Metadata.Version) > 0
	})
	return out, nil
}

func (a *RegistryStorageAdapter) GetLatestRecipe(ctx context.Context, name string) (*models.Recipe, error) {
	versions, err := a.GetRecipeVersions(ctx, name)
	if err != nil {
		return nil, err
	}
	if len(versions) == 0 {
		return nil, fmt.Errorf("no versions found for recipe %s", name)
	}
	return versions[0], nil
}

func compareRecipeVersions(v1, v2 string) int {
	n1, ok1 := normalizeSemver(v1)
	n2, ok2 := normalizeSemver(v2)

	switch {
	case ok1 && ok2:
		return semver.Compare(n1, n2)
	case ok1:
		return 1
	case ok2:
		return -1
	default:
		return strings.Compare(v1, v2)
	}
}

func normalizeSemver(version string) (string, bool) {
	if version == "" {
		return "", false
	}
	if !strings.HasPrefix(version, "v") {
		version = "v" + version
	}
	if semver.IsValid(version) {
		return version, true
	}
	return "", false
}

// Bulk
func (a *RegistryStorageAdapter) ImportRecipes(ctx context.Context, recipes []*models.Recipe) error {
	for _, r := range recipes {
		if err := a.reg.StoreRecipe(ctx, r); err != nil {
			return err
		}
	}
	return nil
}

func (a *RegistryStorageAdapter) ExportRecipes(ctx context.Context, filter RecipeFilter) ([]*models.Recipe, error) {
	return a.ListRecipes(ctx, filter)
}

// Integrity
func (a *RegistryStorageAdapter) ValidateRecipe(ctx context.Context, recipe *models.Recipe) error {
	return recipe.Validate()
}

func (a *RegistryStorageAdapter) CheckRecipeIntegrity(ctx context.Context, id string) error {
	// Basic integrity: ensure it loads
	_, err := a.GetRecipe(ctx, id)
	return err
}

func (a *RegistryStorageAdapter) VerifyRecipeHash(ctx context.Context, id string, expectedHash string) (bool, error) {
	r, err := a.GetRecipe(ctx, id)
	if err != nil {
		return false, err
	}
	current, err := r.CalculateHash()
	if err != nil {
		return false, err
	}
	return current == expectedHash, nil
}

// Index (no external index; operations are no-ops)
func (a *RegistryStorageAdapter) RebuildIndex(ctx context.Context) error { return nil }
func (a *RegistryStorageAdapter) UpdateIndex(ctx context.Context, recipe *models.Recipe, action IndexAction) error {
	return nil
}
