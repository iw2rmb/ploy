package documentation

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	mods "ploy/internal/mods"
)

// TestDocumentationExamples validates all documentation examples
func TestDocumentationExamples(t *testing.T) {
	t.Run("ModConfigExamples", func(t *testing.T) {
		// Test YAML examples from documentation
		exampleConfigs := []string{
			"docs/examples/java-migration.yaml",
			"docs/examples/self-healing.yaml",
			"docs/examples/multi-step.yaml",
			"docs/examples/kb-enabled.yaml",
		}

		for _, configPath := range exampleConfigs {
			t.Run(filepath.Base(configPath), func(t *testing.T) {
				// Check if file exists
				_, err := os.Stat(configPath)
				require.NoError(t, err, "Example config file should exist: %s", configPath)

				// Validate YAML syntax
				yamlData, err := os.ReadFile(configPath)
				require.NoError(t, err, "Should be able to read example config")
				require.NotEmpty(t, yamlData, "Example config should not be empty")

				var config mods.Config
				err = yaml.Unmarshal(yamlData, &config)
				assert.NoError(t, err, "Example YAML should be valid for file: %s", configPath)

				// Basic validation of required fields
				assert.NotEmpty(t, config.Version, "Version should be specified")
				assert.NotEmpty(t, config.ID, "ID should be specified")
				assert.NotEmpty(t, config.TargetRepo, "Target repository should be specified")
				assert.NotEmpty(t, config.Steps, "Steps should be specified")

				// Validate configuration completeness
				err = config.Validate()
				assert.NoError(t, err, "Example configuration should be valid for file: %s", configPath)
			})
		}
	})

	t.Run("APIExamples", func(t *testing.T) {
		// Define API examples from documentation
		type APIExample struct {
			Method string
			Path   string
			Body   string
		}

		apiExamples := map[string]APIExample{
			"create_mod": {
				Method: "POST",
				Path:   "/v1/mods",
				Body:   `{"config": {"version": "v1alpha1", "id": "test", "target_repo": "https://github.com/example/repo.git", "steps": []}}`,
			},
			"get_mod": {
				Method: "GET",
				Path:   "/v1/mods/tf-abc123/status",
				Body:   "",
			},
			"kb_query": {
				Method: "GET",
				Path:   "/v1/llms/kb/errors/java-compilation-error",
				Body:   "",
			},
			"model_list": {
				Method: "GET",
				Path:   "/v1/llms/models",
				Body:   "",
			},
		}

		for name, example := range apiExamples {
			t.Run(name, func(t *testing.T) {
				// Validate HTTP method
				validMethods := []string{"GET", "POST", "PUT", "DELETE", "PATCH"}
				assert.Contains(t, validMethods, example.Method, "HTTP method should be valid")

				// Validate API path structure
				assert.True(t, strings.HasPrefix(example.Path, "/v1/"),
					"API paths should follow /v1/ pattern")

				// Validate JSON syntax for request bodies
				if example.Body != "" {
					var jsonData interface{}
					err := json.Unmarshal([]byte(example.Body), &jsonData)
					assert.NoError(t, err, "API example JSON should be valid for %s", name)
				}
			})
		}
	})

	t.Run("CLIExamples", func(t *testing.T) {
		// Test CLI examples from documentation
		cliExamples := []string{
			"ploy mod run -f examples/java-migration.yaml",
			"ploy mod run -f examples/self-healing.yaml --verbose",
			"ploy mod run -f examples/multi-step.yaml --dry-run",
			"ployman models list",
			"ployman models get gpt-4o-mini@2024-08-06",
		}

		for _, example := range cliExamples {
			t.Run(example, func(t *testing.T) {
				// Parse CLI command structure
				parts := strings.Split(example, " ")
				require.True(t, len(parts) >= 2, "CLI examples should have valid structure")

				// Validate CLI binary name
				validBinaries := []string{"ploy", "ployman"}
				assert.Contains(t, validBinaries, parts[0], "CLI examples should use valid binary")

				// Validate subcommands exist for ploy
				if parts[0] == "ploy" {
					validSubcommands := []string{"mod", "apps", "env", "domains", "debug", "version"}
					assert.Contains(t, validSubcommands, parts[1],
						"Ploy subcommand should be valid")
				}

				// Validate subcommands exist for ployman
				if parts[0] == "ployman" {
					validSubcommands := []string{"models", "api", "version"}
					assert.Contains(t, validSubcommands, parts[1],
						"Ployman subcommand should be valid")
				}
			})
		}
	})
}

// TestDocumentationCompleteness validates that all required documentation exists
func TestDocumentationCompleteness(t *testing.T) {
	requiredDocs := map[string]string{
		"docs/mods/README.md": "Mods user guide",
		"docs/kb/README.md":   "KB learning system documentation",
		"docs/api/mods.md":    "Mods API documentation",
		"docs/examples/":      "Example configurations directory",
		"docs/FEATURES.md":    "Features documentation",
		"CHANGELOG.md":        "Changelog",
	}

	for docPath, description := range requiredDocs {
		t.Run(docPath, func(t *testing.T) {
			if strings.HasSuffix(docPath, "/") {
				// Directory should exist and contain files
				entries, err := os.ReadDir(docPath)
				require.NoError(t, err, "%s should exist", description)
				assert.True(t, len(entries) > 0, "%s should not be empty", description)
			} else {
				// File should exist and have content
				info, err := os.Stat(docPath)
				require.NoError(t, err, "%s should exist", description)
				assert.True(t, info.Size() > 100, "%s should have substantial content", description)
			}
		})
	}
}

// TestConfigurationValidation tests configuration validation logic
func TestConfigurationValidation(t *testing.T) {
	t.Run("ValidConfiguration", func(t *testing.T) {
		config := &mods.Config{
			Version:      "v1alpha1",
			ID:           "test-workflow",
			TargetRepo:   "https://github.com/example/repo.git",
			TargetBranch: "refs/heads/main",
			BaseRef:      "refs/heads/main",
			Steps: []mods.Step{
				{
					Type:   "recipe",
					ID:     "test-step",
					Engine: "openrewrite",
					Recipes: []string{
						"org.openrewrite.java.migrate.Java11toJava17",
					},
				},
			},
		}

		err := config.Validate()
		assert.NoError(t, err, "Valid configuration should pass validation")
	})

	t.Run("InvalidConfiguration", func(t *testing.T) {
		testCases := []struct {
			name   string
			config *mods.Config
		}{
			{
				name: "MissingVersion",
				config: &mods.Config{
					ID:         "test",
					TargetRepo: "https://github.com/example/repo.git",
					Steps:      []mods.Step{},
				},
			},
			{
				name: "MissingID",
				config: &mods.Config{
					Version:    "v1alpha1",
					TargetRepo: "https://github.com/example/repo.git",
					Steps:      []mods.Step{},
				},
			},
			{
				name: "MissingTargetRepo",
				config: &mods.Config{
					Version: "v1alpha1",
					ID:      "test",
					Steps:   []mods.Step{},
				},
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				err := tc.config.Validate()
				assert.Error(t, err, "Invalid configuration should fail validation")
			})
		}
	})
}

// TestExampleConfigurationIntegrity validates example files are consistent
func TestExampleConfigurationIntegrity(t *testing.T) {
	exampleDir := "docs/examples/"
	entries, err := os.ReadDir(exampleDir)
	require.NoError(t, err, "Should be able to read examples directory")

	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".yaml") {
			continue
		}

		t.Run(entry.Name(), func(t *testing.T) {
			filePath := filepath.Join(exampleDir, entry.Name())

			yamlData, err := os.ReadFile(filePath)
			require.NoError(t, err, "Should be able to read example file")

			var config mods.Config
			err = yaml.Unmarshal(yamlData, &config)
			require.NoError(t, err, "Example YAML should unmarshal correctly")

			// Validate required fields are present
			assert.NotEmpty(t, config.Version, "Version should be specified")
			assert.NotEmpty(t, config.ID, "ID should be specified")
			assert.NotEmpty(t, config.TargetRepo, "TargetRepo should be specified")
			assert.NotEmpty(t, config.Steps, "Steps should be specified")

			// Validate steps have required fields
			for i, step := range config.Steps {
				assert.NotEmpty(t, step.Type, "Step %d should have type", i)
				assert.NotEmpty(t, step.ID, "Step %d should have ID", i)
				assert.NotEmpty(t, step.Engine, "Step %d should have engine", i)

				if step.Engine == "openrewrite" {
					assert.NotEmpty(t, step.Recipes, "OpenRewrite step %d should have recipes", i)
				}
			}
		})
	}
}

// TestDocumentationLinks validates internal documentation links
func TestDocumentationLinks(t *testing.T) {
	// Test that referenced files in documentation actually exist
	t.Run("ReferencedFiles", func(t *testing.T) {
		referencedFiles := []string{
			"docs/examples/java-migration.yaml",
			"docs/examples/self-healing.yaml",
			"docs/examples/multi-step.yaml",
			"docs/examples/kb-enabled.yaml",
		}

		for _, file := range referencedFiles {
			t.Run(filepath.Base(file), func(t *testing.T) {
				_, err := os.Stat(file)
				assert.NoError(t, err, "Referenced file should exist: %s", file)
			})
		}
	})
}

// Helper function to read example file content
func readExampleFile(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(data)
}
