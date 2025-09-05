package arf

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	models "github.com/iw2rmb/ploy/internal/arf/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockHTTPClient creates a test HTTP client that returns predefined responses
type mockHTTPClient struct {
	responses map[string]*http.Response
	t         *testing.T
}

func (m *mockHTTPClient) Do(req *http.Request) (*http.Response, error) {
	key := req.Method + " " + req.URL.Path
	if resp, ok := m.responses[key]; ok {
		return resp, nil
	}
	m.t.Fatalf("unexpected request: %s %s", req.Method, req.URL.Path)
	return nil, nil
}

func TestHandleARFRecipesCommand(t *testing.T) {
	tests := []struct {
		name        string
		args        []string
		expectError bool
	}{
		{
			name:        "no args shows list",
			args:        []string{},
			expectError: false,
		},
		{
			name:        "list command",
			args:        []string{"list"},
			expectError: false,
		},
		{
			name:        "search command with query",
			args:        []string{"search", "java"},
			expectError: false,
		},
		{
			name:        "show command with id",
			args:        []string{"show", "test-recipe"},
			expectError: false,
		},
		{
			name:        "help command",
			args:        []string{"--help"},
			expectError: false,
		},
		{
			name:        "unknown command",
			args:        []string{"unknown"},
			expectError: false, // prints usage, doesn't error
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Capture output
			oldStdout := os.Stdout
			r, w, _ := os.Pipe()
			os.Stdout = w

			// Mock the controller URL
			originalURL := arfControllerURL
			arfControllerURL = "http://test.local/v1"
			defer func() {
				arfControllerURL = originalURL
			}()

			// Run the command
			err := handleARFRecipesCommand(tt.args)

			// Restore stdout
			w.Close()
			os.Stdout = oldStdout

			// Check error expectation
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			// Read captured output
			var buf bytes.Buffer
			io.Copy(&buf, r)
			output := buf.String()

			// Verify some output was produced
			if !tt.expectError {
				assert.NotEmpty(t, output)
			}
		})
	}
}

func TestListRecipes(t *testing.T) {
	// Setup test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1/arf/recipes", r.URL.Path)

		response := struct {
			Recipes []models.Recipe `json:"recipes"`
			Count   int             `json:"count"`
			Total   int             `json:"total"`
		}{
			Recipes: []models.Recipe{
				{
					ID: "test-recipe-1",
					Metadata: models.RecipeMetadata{
						Name:        "Test Recipe 1",
						Description: "A test recipe",
						Languages:   []string{"java"},
						Categories:  []string{"cleanup"},
					},
				},
				{
					ID: "test-recipe-2",
					Metadata: models.RecipeMetadata{
						Name:        "Test Recipe 2",
						Description: "Another test recipe",
						Languages:   []string{"python"},
						Categories:  []string{"migration"},
					},
				},
			},
			Count: 2,
			Total: 2,
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	// Override the controller URL
	originalURL := arfControllerURL
	arfControllerURL = server.URL
	defer func() {
		arfControllerURL = originalURL
	}()

	// Capture output
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	// Test list recipes
	filter := RecipeFilter{
		Limit: 10,
	}
	err := listRecipes(filter, "table", false)

	// Restore stdout
	w.Close()
	os.Stdout = oldStdout

	// Check result
	require.NoError(t, err)

	// Read captured output
	var buf bytes.Buffer
	io.Copy(&buf, r)
	output := buf.String()

	// Verify output contains expected content
	assert.Contains(t, output, "Test Recipe 1")
	assert.Contains(t, output, "Test Recipe 2")
}

func TestSearchRecipes(t *testing.T) {
	// Setup test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1/arf/recipes/search", r.URL.Path)
		assert.Equal(t, "java migration", r.URL.Query().Get("q"))

		response := struct {
			Recipes []models.Recipe `json:"recipes"`
			Count   int             `json:"count"`
			Query   string          `json:"query"`
		}{
			Recipes: []models.Recipe{
				{
					ID: "java-migration",
					Metadata: models.RecipeMetadata{
						Name:        "Java Migration",
						Description: "Migrate Java versions",
						Languages:   []string{"java"},
						Categories:  []string{"migration"},
					},
				},
			},
			Count: 1,
			Query: "java migration",
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	// Override the controller URL
	originalURL := arfControllerURL
	arfControllerURL = server.URL
	defer func() {
		arfControllerURL = originalURL
	}()

	// Capture output
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	// Test search recipes
	flags := CommandFlags{
		OutputFormat: "table",
	}
	err := searchRecipes("java migration", flags)

	// Restore stdout
	w.Close()
	os.Stdout = oldStdout

	// Check result
	require.NoError(t, err)

	// Read captured output
	var buf bytes.Buffer
	io.Copy(&buf, r)
	output := buf.String()

	// Verify output contains expected content
	assert.Contains(t, output, "Java Migration")
}

func TestShowRecipe(t *testing.T) {
	// Setup test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.True(t, strings.HasPrefix(r.URL.Path, "/v1/arf/recipes/"))

		recipe := models.Recipe{
			ID: "test-recipe",
			Metadata: models.RecipeMetadata{
				Name:        "Test Recipe",
				Description: "A detailed test recipe",
				Version:     "1.0.0",
				Author:      "Test Author",
				Languages:   []string{"java"},
				Categories:  []string{"cleanup"},
				Tags:        []string{"test", "example"},
			},
			Steps: []models.RecipeStep{
				{
					Name: "Step 1",
					Type: "openrewrite",
					Config: map[string]interface{}{
						"recipe": "org.openrewrite.java.RemoveUnusedImports",
					},
				},
			},
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(recipe)
	}))
	defer server.Close()

	// Override the controller URL
	originalURL := arfControllerURL
	arfControllerURL = server.URL
	defer func() {
		arfControllerURL = originalURL
	}()

	// Capture output
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	// Test show recipe
	flags := CommandFlags{
		OutputFormat: "table",
		Verbose:      true,
	}
	err := showRecipe("test-recipe", flags)

	// Restore stdout
	w.Close()
	os.Stdout = oldStdout

	// Check result
	require.NoError(t, err)

	// Read captured output
	var buf bytes.Buffer
	io.Copy(&buf, r)
	output := buf.String()

	// Verify output contains expected content
	assert.Contains(t, output, "Test Recipe")
	assert.Contains(t, output, "A detailed test recipe")
}

func TestParseCommonFlags(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		expected CommandFlags
	}{
		{
			name: "no flags",
			args: []string{},
			expected: CommandFlags{
				OutputFormat: "table",
			},
		},
		{
			name: "dry run flag",
			args: []string{"--dry-run"},
			expected: CommandFlags{
				DryRun:       true,
				OutputFormat: "table",
			},
		},
		{
			name: "force flag short",
			args: []string{"-f"},
			expected: CommandFlags{
				Force:        true,
				OutputFormat: "table",
			},
		},
		{
			name: "verbose flag",
			args: []string{"--verbose"},
			expected: CommandFlags{
				Verbose:      true,
				OutputFormat: "table",
			},
		},
		{
			name: "output format json",
			args: []string{"--output", "json"},
			expected: CommandFlags{
				OutputFormat: "json",
			},
		},
		{
			name: "output format yaml short",
			args: []string{"-o", "yaml"},
			expected: CommandFlags{
				OutputFormat: "yaml",
			},
		},
		{
			name: "multiple flags",
			args: []string{"--dry-run", "-v", "--output", "json", "--force"},
			expected: CommandFlags{
				DryRun:       true,
				Force:        true,
				Verbose:      true,
				OutputFormat: "json",
			},
		},
		{
			name: "with name flag",
			args: []string{"--name", "my-recipe"},
			expected: CommandFlags{
				Name:         "my-recipe",
				OutputFormat: "table",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseCommonFlags(tt.args)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestParseCatalogListFromRecipes(t *testing.T) {
	input := `[
		{
			"id": "org.openrewrite.java.RemoveUnusedImports",
			"display_name": "Remove Unused Imports",
			"description": "Remove unused import statements",
			"tags": ["cleanup", "java"],
			"pack": "rewrite-java",
			"version": "8.1.0"
		},
		{
			"id": "org.openrewrite.java.migrate.UpgradeToJava17",
			"display_name": "Upgrade to Java 17",
			"description": "Migrate to Java 17",
			"tags": ["migration", "java"],
			"pack": "rewrite-migrate-java",
			"version": "2.0.0"
		}
	]`

	items, err := parseCatalogList([]byte(input))
	require.NoError(t, err)
	assert.Len(t, items, 2)

	assert.Equal(t, "org.openrewrite.java.RemoveUnusedImports", items[0].ID)
	assert.Equal(t, "Remove Unused Imports", items[0].DisplayName)
	assert.Equal(t, "rewrite-java", items[0].Pack)
	assert.Equal(t, "8.1.0", items[0].Version)

	assert.Equal(t, "org.openrewrite.java.migrate.UpgradeToJava17", items[1].ID)
	assert.Equal(t, "Upgrade to Java 17", items[1].DisplayName)
	assert.Equal(t, "rewrite-migrate-java", items[1].Pack)
}

func TestHandleRecipeList(t *testing.T) {
	tests := []struct {
		name        string
		args        []string
		envCatalog  string
		expectError bool
	}{
		{
			name:        "default list",
			args:        []string{},
			expectError: false,
		},
		{
			name:        "with language filter",
			args:        []string{"--language", "java"},
			expectError: false,
		},
		{
			name:        "with output format",
			args:        []string{"--output", "json"},
			expectError: false,
		},
		{
			name:        "catalog mode",
			args:        []string{},
			envCatalog:  "true",
			expectError: false,
		},
		{
			name:        "with pack filter",
			args:        []string{"--pack", "rewrite-java"},
			expectError: false,
		},
		{
			name:        "with version filter",
			args:        []string{"--version", "8.1.0"},
			expectError: false,
		},
		{
			name:        "with pack and version filters",
			args:        []string{"--pack", "rewrite-spring", "--version", "5.0.0"},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup test server
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Return appropriate response based on catalog mode
				if tt.envCatalog == "true" {
					// Catalog mode response
					items := []catalogRecipe{
						{
							ID:          "test-recipe",
							DisplayName: "Test Recipe",
							Description: "Test",
							Pack:        "test-pack",
							Version:     "1.0.0",
						},
					}
					w.Header().Set("Content-Type", "application/json")
					json.NewEncoder(w).Encode(items)
				} else {
					// Regular mode response
					response := struct {
						Recipes []models.Recipe `json:"recipes"`
						Count   int             `json:"count"`
					}{
						Recipes: []models.Recipe{
							{
								ID: "test-recipe",
								Metadata: models.RecipeMetadata{
									Name: "Test Recipe",
								},
							},
						},
						Count: 1,
					}
					w.Header().Set("Content-Type", "application/json")
					json.NewEncoder(w).Encode(response)
				}
			}))
			defer server.Close()

			// Set environment and controller URL
			if tt.envCatalog != "" {
				os.Setenv("PLOY_RECIPES_CATALOG", tt.envCatalog)
				defer os.Unsetenv("PLOY_RECIPES_CATALOG")
			}

			originalURL := arfControllerURL
			arfControllerURL = server.URL
			defer func() {
				arfControllerURL = originalURL
			}()

			// Capture output
			oldStdout := os.Stdout
			r, w, _ := os.Pipe()
			os.Stdout = w

			// Run command
			err := handleRecipeList(tt.args)

			// Restore stdout
			w.Close()
			os.Stdout = oldStdout

			// Read output
			var buf bytes.Buffer
			io.Copy(&buf, r)

			// Check result
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestParseFilterFlags(t *testing.T) {
	tests := []struct {
		name              string
		args              []string
		expectedFilter    RecipeFilter
		expectedRemaining []string
	}{
		{
			name: "parse pack flag",
			args: []string{"--pack", "rewrite-java", "--output", "json"},
			expectedFilter: RecipeFilter{
				Pack:  "rewrite-java",
				Limit: 20,
			},
			expectedRemaining: []string{"--output", "json"},
		},
		{
			name: "parse version flag",
			args: []string{"--version", "8.1.0", "-v"},
			expectedFilter: RecipeFilter{
				Version: "8.1.0",
				Limit:   20,
			},
			expectedRemaining: []string{"-v"},
		},
		{
			name: "parse pack and version flags",
			args: []string{"--pack", "rewrite-spring", "--version", "5.0.0", "--language", "java"},
			expectedFilter: RecipeFilter{
				Pack:     "rewrite-spring",
				Version:  "5.0.0",
				Language: "java",
				Limit:    20,
			},
			expectedRemaining: []string{},
		},
		{
			name: "short form flags",
			args: []string{"-p", "rewrite-java", "-V", "8.1.0"},
			expectedFilter: RecipeFilter{
				Pack:    "rewrite-java",
				Version: "8.1.0",
				Limit:   20,
			},
			expectedRemaining: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filter, remaining := ParseFilterFlags(tt.args)

			// Check filter fields
			assert.Equal(t, tt.expectedFilter.Pack, filter.Pack, "Pack mismatch")
			assert.Equal(t, tt.expectedFilter.Version, filter.Version, "Version mismatch")
			assert.Equal(t, tt.expectedFilter.Language, filter.Language, "Language mismatch")
			assert.Equal(t, tt.expectedFilter.Limit, filter.Limit, "Limit mismatch")

			// Check remaining args
			assert.Equal(t, tt.expectedRemaining, remaining, "Remaining args mismatch")
		})
	}
}
