package recipes

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func writeTempRecipeFile(t *testing.T, recipe map[string]any) string {
	t.Helper()
	data, err := yaml.Marshal(recipe)
	if err != nil {
		t.Fatalf("failed to marshal recipe: %v", err)
	}
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "recipe.yaml")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("failed to write recipe file: %v", err)
	}
	return path
}

func TestUploadRecipeDryRun(t *testing.T) {
	path := writeTempRecipeFile(t, map[string]any{
		"id": "sample.recipe",
		"metadata": map[string]any{
			"name":    "Sample Recipe",
			"version": "1.0.0",
		},
		"steps": []any{},
	})

	flags := CommandFlags{DryRun: true}
	var err error
	output := captureOutput(func() {
		err = uploadRecipe(path, flags)
	})

	if err != nil {
		t.Fatalf("uploadRecipe returned error: %v", err)
	}
	if !strings.Contains(output, "Recipe 'Sample Recipe' is valid and ready for upload") {
		t.Fatalf("unexpected output: %s", output)
	}
}

func TestUploadRecipeSendsRequest(t *testing.T) {
	requestCaptured := make(chan []byte, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("unexpected method: %s", r.Method)
		}
		if r.URL.Path != "/arf/recipes/upload" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("failed to read body: %v", err)
		}
		requestCaptured <- body
		_, _ = w.Write([]byte(`{"id":"generated-id","message":"ok"}`))
	}))
	t.Cleanup(server.Close)

	oldURL := controllerURL
	controllerURL = server.URL
	t.Cleanup(func() { controllerURL = oldURL })

	path := writeTempRecipeFile(t, map[string]any{
		"id": "sample.recipe",
		"metadata": map[string]any{
			"name":    "Sample Recipe",
			"version": "1.0.0",
		},
		"steps": []any{},
	})

	var err error
	output := captureOutput(func() {
		err = uploadRecipe(path, CommandFlags{})
	})

	if err != nil {
		t.Fatalf("uploadRecipe returned error: %v", err)
	}

	select {
	case body := <-requestCaptured:
		var payload map[string]any
		if err := json.Unmarshal(body, &payload); err != nil {
			t.Fatalf("payload not JSON: %v\n%s", err, string(body))
		}
		if id, _ := payload["id"].(string); id == "" {
			t.Fatalf("expected id in payload: %v", payload)
		}
	default:
		t.Fatalf("uploadRecipe did not send request")
	}

	if !strings.Contains(output, "uploaded successfully") {
		t.Fatalf("expected success message, got: %s", output)
	}
}

func TestUpdateRecipeSendsRequest(t *testing.T) {
	var received bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received = true
		if r.Method != http.MethodPut {
			t.Fatalf("unexpected method: %s", r.Method)
		}
		if r.URL.Path != "/arf/recipes/recipe-123" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		_, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)

	oldURL := controllerURL
	controllerURL = server.URL
	t.Cleanup(func() { controllerURL = oldURL })

	path := writeTempRecipeFile(t, map[string]any{
		"id": "recipe-123",
		"metadata": map[string]any{
			"name":    "Recipe",
			"version": "1.0.0",
		},
	})

	if err := updateRecipe("recipe-123", path, CommandFlags{}); err != nil {
		t.Fatalf("updateRecipe returned error: %v", err)
	}
	if !received {
		t.Fatalf("expected request to be sent")
	}
}

func TestDeleteRecipeForce(t *testing.T) {
	var received bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received = true
		if r.Method != http.MethodDelete {
			t.Fatalf("unexpected method: %s", r.Method)
		}
		if r.URL.Path != "/arf/recipes/delete-me" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(server.Close)

	oldURL := controllerURL
	controllerURL = server.URL
	t.Cleanup(func() { controllerURL = oldURL })

	var err error
	output := captureOutput(func() {
		err = deleteRecipe("delete-me", CommandFlags{Force: true})
	})
	if err != nil {
		t.Fatalf("deleteRecipe returned error: %v", err)
	}
	if !received {
		t.Fatalf("expected delete request")
	}
	if !strings.Contains(output, "deleted successfully") {
		t.Fatalf("expected success message, got: %s", output)
	}
}

func TestDownloadRecipeWritesFile(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("unexpected method: %s", r.Method)
		}
		if r.URL.Path != "/arf/recipes/sample" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"id":"sample","metadata":{"name":"Sample Recipe"},"steps":[]}`))
	}))
	t.Cleanup(server.Close)

	oldURL := controllerURL
	controllerURL = server.URL
	t.Cleanup(func() { controllerURL = oldURL })

	tmpDir := t.TempDir()
	outPath := filepath.Join(tmpDir, "downloaded.yaml")
	err := downloadRecipe("sample", CommandFlags{OutputFile: outPath})
	if err != nil {
		t.Fatalf("downloadRecipe returned error: %v", err)
	}

	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("failed to read output: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "Sample Recipe") {
		t.Fatalf("expected recipe name in output: %s", content)
	}
}

func TestGetRecipeStatsTableOutput(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("unexpected method: %s", r.Method)
		}
		if r.URL.Path != "/arf/recipes/sample/stats" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{
  "recipe_id": "sample",
  "total_executions": 10,
  "successful_runs": 8,
  "failed_runs": 2,
  "success_rate": 0.8,
  "avg_execution_time": "5m",
  "last_executed": "2024-01-02T15:04:05Z",
  "first_executed": "2024-01-01T00:00:00Z"
}`))
	}))
	t.Cleanup(server.Close)

	oldURL := controllerURL
	controllerURL = server.URL
	t.Cleanup(func() { controllerURL = oldURL })

	var err error
	output := captureOutput(func() {
		err = getRecipeStats("sample", CommandFlags{})
	})
	if err != nil {
		t.Fatalf("getRecipeStats returned error: %v", err)
	}
	for _, token := range []string{"Recipe Statistics", "Total Executions", "Success Rate: 80.00%"} {
		if !strings.Contains(output, token) {
			t.Fatalf("expected %q in output, got: %s", token, output)
		}
	}
}

func TestListRecipesTableMode(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("unexpected method: %s", r.Method)
		}
		if r.URL.Path != "/arf/recipes" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if r.URL.RawQuery != "language=go&limit=5" {
			t.Fatalf("unexpected query: %s", r.URL.RawQuery)
		}
		_, _ = w.Write([]byte(`{
  "recipes": [
    {
      "id": "sample.recipe",
      "metadata": {
        "name": "Sample Recipe",
        "description": "Demo",
        "author": "Author",
        "version": "1.0.0",
        "languages": ["go"],
        "categories": ["migration"]
      },
      "steps": [
        {"name": "Step1", "type": "task", "timeout": "1m"}
      ],
      "created_at": "2024-01-02T00:00:00Z",
      "updated_at": "2024-01-03T00:00:00Z"
    }
  ],
  "count": 1,
  "total": 1
}`))
	}))
	t.Cleanup(server.Close)

	oldURL := controllerURL
	controllerURL = server.URL
	t.Cleanup(func() { controllerURL = oldURL })

	filter := RecipeFilter{Language: "go", Limit: 5}
	var err error
	output := captureOutput(func() {
		err = listRecipes(filter, "table", false)
	})
	if err != nil {
		t.Fatalf("listRecipes returned error: %v", err)
	}
	if !strings.Contains(output, "🔍 Filters: language: go") {
		t.Fatalf("expected filter summary, got: %s", output)
	}
	if !strings.Contains(output, "sample.recipe") {
		t.Fatalf("expected recipe id in output, got: %s", output)
	}
	if !strings.Contains(output, "Showing 1-1 of 1 recipes") {
		t.Fatalf("expected pagination summary, got: %s", output)
	}
}

func TestSearchRecipesCatalogMode(t *testing.T) {
	t.Setenv("PLOY_RECIPES_CATALOG", "true")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("unexpected method: %s", r.Method)
		}
		if r.URL.Path != "/arf/recipes" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if r.URL.RawQuery != "query=Sample" {
			t.Fatalf("unexpected query: %s", r.URL.RawQuery)
		}
		_, _ = w.Write([]byte(`[
  {
    "id": "sample.recipe",
    "display_name": "Sample Recipe",
    "description": "Demo",
    "tags": ["tag"],
    "pack": "sample-pack",
    "version": "1.0.0"
  }
]`))
	}))
	t.Cleanup(server.Close)

	oldURL := controllerURL
	controllerURL = server.URL
	t.Cleanup(func() { controllerURL = oldURL })

	output := captureOutput(func() {
		err := searchRecipes("Sample", CommandFlags{OutputFormat: "table"})
		if err != nil {
			t.Fatalf("searchRecipes returned error: %v", err)
		}
	})
	if !strings.Contains(output, "ID\tPACK\tVERSION") {
		t.Fatalf("expected table headers in output, got: %s", output)
	}
	if !strings.Contains(output, "Total: 1 recipes") {
		t.Fatalf("expected total count, got: %s", output)
	}
}

func TestSearchRecipesTableOutput(t *testing.T) {
	t.Setenv("PLOY_RECIPES_CATALOG", "false")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("unexpected method: %s", r.Method)
		}
		if r.URL.Path != "/arf/recipes/search" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if r.URL.RawQuery != "q=Sample" {
			t.Fatalf("unexpected query: %s", r.URL.RawQuery)
		}
		_, _ = w.Write([]byte(`{
  "recipes": [
    {
      "id": "sample.recipe",
      "metadata": {
        "name": "Sample Recipe",
        "description": "Demo",
        "author": "Author",
        "version": "1.0.0",
        "languages": ["go"],
        "categories": ["migration"]
      },
      "steps": [],
      "created_at": "2024-01-02T00:00:00Z",
      "updated_at": "2024-01-03T00:00:00Z"
    }
  ],
  "count": 1,
  "query": "Sample"
}`))
	}))
	t.Cleanup(server.Close)

	oldURL := controllerURL
	controllerURL = server.URL
	t.Cleanup(func() { controllerURL = oldURL })

	output := captureOutput(func() {
		err := searchRecipes("Sample", CommandFlags{OutputFormat: "table"})
		if err != nil {
			t.Fatalf("searchRecipes returned error: %v", err)
		}
	})
	if !strings.Contains(output, "Search results for \"Sample\"") {
		t.Fatalf("expected search header, got: %s", output)
	}
	if !strings.Contains(output, "Total: 1 recipes") {
		t.Fatalf("expected total count, got: %s", output)
	}
}

func TestHandleRecipeCommandShow(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("unexpected method: %s", r.Method)
		}
		if r.URL.Path != "/arf/recipes/sample" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{
  "id": "sample",
  "metadata": {
    "name": "Sample Recipe",
    "description": "Demo",
    "author": "Author",
    "version": "1.0.0",
    "languages": ["go"],
    "categories": ["migration"]
  },
  "steps": [],
  "created_at": "2024-01-02T00:00:00Z",
  "updated_at": "2024-01-03T00:00:00Z"
}`))
	}))
	t.Cleanup(server.Close)

	oldURL := controllerURL
	controllerURL = server.URL
	t.Cleanup(func() { controllerURL = oldURL })

	var err error
	output := captureOutput(func() {
		err = handleRecipeCommand([]string{"show", "sample"})
	})
	if err != nil {
		t.Fatalf("handleRecipeCommand returned error: %v", err)
	}
	if !strings.Contains(output, "Recipe Details") {
		t.Fatalf("expected recipe details output, got: %s", output)
	}
}

func TestHandleRecipeCommandUnknown(t *testing.T) {
	var err error
	output := captureOutput(func() {
		err = handleRecipeCommand([]string{"unknown"})
	})
	if err != nil {
		t.Fatalf("expected nil error, got: %v", err)
	}
	if !strings.Contains(output, "Unknown recipes action") {
		t.Fatalf("expected unknown action message, got: %s", output)
	}
	if !strings.Contains(output, "Usage: ploy recipe") {
		t.Fatalf("expected usage output, got: %s", output)
	}
}
