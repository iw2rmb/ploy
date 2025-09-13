package recipes

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/iw2rmb/ploy/api/arf/models"
)

type noopStorage struct{}

func (n *noopStorage) CreateRecipe(ctx context.Context, r *models.Recipe) error { return nil }
func (n *noopStorage) GetRecipe(ctx context.Context, id string) (*models.Recipe, error) {
	return &models.Recipe{ID: id, Metadata: models.RecipeMetadata{Name: id}}, nil
}
func (n *noopStorage) GetRecipeByNameAndVersion(ctx context.Context, name, version string) (*models.Recipe, error) {
	return nil, nil
}
func (n *noopStorage) UpdateRecipe(ctx context.Context, id string, r *models.Recipe) error {
	return nil
}
func (n *noopStorage) DeleteRecipe(ctx context.Context, id string) error { return nil }
func (n *noopStorage) ListRecipes(ctx context.Context, f RecipeFilter) ([]*models.Recipe, error) {
	return []*models.Recipe{}, nil
}
func (n *noopStorage) SearchRecipes(ctx context.Context, q string) ([]*RecipeSearchResult, error) {
	return []*RecipeSearchResult{}, nil
}
func (n *noopStorage) GetRecipeVersions(ctx context.Context, name string) ([]*models.Recipe, error) {
	return nil, nil
}
func (n *noopStorage) GetLatestRecipe(ctx context.Context, name string) (*models.Recipe, error) {
	return nil, nil
}
func (n *noopStorage) ImportRecipes(ctx context.Context, recipes []*models.Recipe) error { return nil }
func (n *noopStorage) ExportRecipes(ctx context.Context, filter RecipeFilter) ([]*models.Recipe, error) {
	return []*models.Recipe{}, nil
}
func (n *noopStorage) ValidateRecipe(ctx context.Context, recipe *models.Recipe) error { return nil }
func (n *noopStorage) CheckRecipeIntegrity(ctx context.Context, id string) error       { return nil }
func (n *noopStorage) VerifyRecipeHash(ctx context.Context, id string, expectedHash string) (bool, error) {
	return true, nil
}
func (n *noopStorage) RebuildIndex(ctx context.Context) error { return nil }
func (n *noopStorage) UpdateIndex(ctx context.Context, recipe *models.Recipe, action IndexAction) error {
	return nil
}

func TestExecuteRecipeByID_OpenRewriteRejected(t *testing.T) {
	ex := NewRecipeExecutor(&noopStorage{}, nil)
	if _, err := ex.ExecuteRecipeByID(context.Background(), "org.openrewrite.Recipe", ".", "openrewrite", ""); err == nil {
		t.Fatalf("expected error for openrewrite execution")
	}
}

func TestCheckConditions_FileExistsAndNotExists(t *testing.T) {
	ex := NewRecipeExecutor(&noopStorage{}, nil)
	dir := t.TempDir()
	f := filepath.Join(dir, "a.txt")
	_ = os.WriteFile(f, []byte("x"), 0o644)

	conds := []models.ExecutionCondition{{Type: models.ConditionFileExists, Value: "a.txt"}}
	if !ex.checkConditions(conds, dir) {
		t.Fatalf("expected true when file exists")
	}

	conds = []models.ExecutionCondition{{Type: models.ConditionFileNotExists, Value: "missing.txt"}}
	if !ex.checkConditions(conds, dir) {
		t.Fatalf("expected true when file not exists")
	}
}
