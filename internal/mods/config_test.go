package mods

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadConfig(t *testing.T) {
	tests := []struct {
		name        string
		yamlContent string
		expected    *TransflowConfig
		expectError bool
	}{
		{
			name: "valid complete config",
			yamlContent: `version: v1alpha1
id: test-workflow
target_repo: https://github.com/org/project
target_branch: refs/heads/main
base_ref: refs/heads/main
lane: C
build_timeout: 10m
steps:
  - type: recipe
    id: openrewrite-updates
    engine: openrewrite
    recipes:
      - com.acme.FixNulls
      - com.acme.UpdateApi`,
			expected: &TransflowConfig{
				Version:      "v1alpha1",
				ID:           "test-workflow",
				TargetRepo:   "https://github.com/org/project",
				TargetBranch: "refs/heads/main",
				BaseRef:      "refs/heads/main",
				Lane:         "C",
				BuildTimeout: "10m",
				Steps: []TransflowStep{
					{
						Type:   "recipe", // legacy YAML uses recipe; runner maps orw-apply in execution paths
						ID:     "openrewrite-updates",
						Engine: "openrewrite",
						Recipes: []string{
							"com.acme.FixNulls",
							"com.acme.UpdateApi",
						},
					},
				},
				SelfHeal: GetDefaultSelfHealConfig(),
			},
			expectError: false,
		},
		{
			name: "minimal valid config",
			yamlContent: `version: v1alpha1
id: minimal-workflow
target_repo: https://github.com/org/simple-project
base_ref: refs/heads/main
steps:
  - type: recipe
    id: simple-recipe
    engine: openrewrite
    recipes:
      - com.acme.SimpleRecipe`,
			expected: &TransflowConfig{
				Version:      "v1alpha1",
				ID:           "minimal-workflow",
				TargetRepo:   "https://github.com/org/simple-project",
				TargetBranch: "",
				BaseRef:      "refs/heads/main",
				Lane:         "",
				BuildTimeout: "",
				Steps: []TransflowStep{
					{
						Type:   "recipe", // legacy YAML uses recipe; runner maps orw-apply in execution paths
						ID:     "simple-recipe",
						Engine: "openrewrite",
						Recipes: []string{
							"com.acme.SimpleRecipe",
						},
					},
				},
				SelfHeal: GetDefaultSelfHealConfig(),
			},
			expectError: false,
		},
		{
			name: "missing required id field",
			yamlContent: `version: v1alpha1
target_repo: https://github.com/org/project
base_ref: refs/heads/main
steps:
  - type: recipe
    id: test-recipe
    engine: openrewrite
    recipes:
      - com.acme.Recipe`,
			expected:    nil,
			expectError: true,
		},
		{
			name: "missing required target_repo field",
			yamlContent: `version: v1alpha1
id: test-workflow
base_ref: refs/heads/main
steps:
  - type: recipe
    id: test-recipe
    engine: openrewrite
    recipes:
      - com.acme.Recipe`,
			expected:    nil,
			expectError: true,
		},
		{
			name: "missing required base_ref field",
			yamlContent: `version: v1alpha1
id: test-workflow
target_repo: https://github.com/org/project
steps:
  - type: recipe
    id: test-recipe
    engine: openrewrite
    recipes:
      - com.acme.Recipe`,
			expected:    nil,
			expectError: true,
		},
		{
			name: "empty steps array",
			yamlContent: `version: v1alpha1
id: test-workflow
target_repo: https://github.com/org/project
base_ref: refs/heads/main
steps: []`,
			expected:    nil,
			expectError: true,
		},
		{
			name: "invalid yaml syntax",
			yamlContent: `version: v1alpha1
id: test-workflow
target_repo: https://github.com/org/project
base_ref: refs/heads/main
steps:
  - type: recipe
    id: test-recipe
    engine: openrewrite
    recipes:
      - com.acme.Recipe
    invalid: [unclosed`,
			expected:    nil,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temporary file with test content
			tmpFile := filepath.Join(t.TempDir(), "transflow.yaml")
			err := os.WriteFile(tmpFile, []byte(tt.yamlContent), 0644)
			require.NoError(t, err)

			// Load config
			config, err := LoadConfig(tmpFile)

			// Check error expectation
			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, config)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, config)
			}
		})
	}
}

func TestConfigValidation(t *testing.T) {
	tests := []struct {
		name        string
		config      *TransflowConfig
		expectError bool
		errorMsg    string
	}{
		{
			name: "valid config",
			config: &TransflowConfig{
				Version:      "v1alpha1",
				ID:           "test-workflow",
				TargetRepo:   "https://github.com/org/project",
				BaseRef:      "refs/heads/main",
				BuildTimeout: "10m",
				Steps: []TransflowStep{
					{
						Type:    "orw-apply",
						ID:      "test-recipe",
						Engine:  "openrewrite",
						Recipes: []string{"com.acme.Recipe"},
					},
				},
			},
			expectError: false,
		},
		{
			name: "empty id",
			config: &TransflowConfig{
				ID:         "",
				TargetRepo: "https://github.com/org/project",
				BaseRef:    "refs/heads/main",
				Steps: []TransflowStep{
					{Type: "orw-apply", ID: "java-migration", Recipes: []string{"org.openrewrite.java.migrate.UpgradeToJava17"}},
				},
			},
			expectError: true,
			errorMsg:    "id is required",
		},
		{
			name: "empty target_repo",
			config: &TransflowConfig{
				ID:         "test-workflow",
				TargetRepo: "",
				BaseRef:    "refs/heads/main",
				Steps: []TransflowStep{
					{Type: "orw-apply", ID: "java-migration", Recipes: []string{"org.openrewrite.java.migrate.UpgradeToJava17"}},
				},
			},
			expectError: true,
			errorMsg:    "target_repo is required",
		},
		{
			name: "empty base_ref",
			config: &TransflowConfig{
				ID:         "test-workflow",
				TargetRepo: "https://github.com/org/project",
				BaseRef:    "",
				Steps: []TransflowStep{
					{Type: "orw-apply", ID: "java-migration", Recipes: []string{"org.openrewrite.java.migrate.UpgradeToJava17"}},
				},
			},
			expectError: true,
			errorMsg:    "base_ref is required",
		},
		{
			name: "no steps",
			config: &TransflowConfig{
				ID:         "test-workflow",
				TargetRepo: "https://github.com/org/project",
				BaseRef:    "refs/heads/main",
				Steps:      []TransflowStep{},
			},
			expectError: true,
			errorMsg:    "at least one step is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()

			if tt.expectError {
				assert.Error(t, err)
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestParseBuildTimeout(t *testing.T) {
	tests := []struct {
		name        string
		timeout     string
		expected    time.Duration
		expectError bool
	}{
		{
			name:        "valid 10m timeout",
			timeout:     "10m",
			expected:    10 * time.Minute,
			expectError: false,
		},
		{
			name:        "valid 30s timeout",
			timeout:     "30s",
			expected:    30 * time.Second,
			expectError: false,
		},
		{
			name:        "valid 2h timeout",
			timeout:     "2h",
			expected:    2 * time.Hour,
			expectError: false,
		},
		{
			name:        "empty timeout uses default",
			timeout:     "",
			expected:    10 * time.Minute, // default
			expectError: false,
		},
		{
			name:        "invalid timeout format",
			timeout:     "invalid",
			expected:    0,
			expectError: true,
		},
		{
			name:        "negative timeout",
			timeout:     "-5m",
			expected:    0,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := &TransflowConfig{
				BuildTimeout: tt.timeout,
			}

			duration, err := config.ParseBuildTimeout()

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, duration)
			}
		})
	}
}

func TestGenerateAppName(t *testing.T) {
	tests := []struct {
		name     string
		id       string
		expected string // We'll test the pattern, not exact match due to timestamp
	}{
		{
			name:     "simple id",
			id:       "test-workflow",
			expected: "tfw-test-workflow-",
		},
		{
			name:     "id with underscores",
			id:       "my_complex_workflow",
			expected: "tfw-my_complex_workflow-",
		},
		{
			name:     "short id",
			id:       "w1",
			expected: "tfw-w1-",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			appName := GenerateAppName(tt.id)

			// Check prefix
			assert.True(t, strings.HasPrefix(appName, tt.expected))

			// Check total length reasonable (prefix + timestamp should be < 64 chars)
			assert.Less(t, len(appName), 64)

			// Check it contains only valid characters for app names
			assert.Regexp(t, `^[a-zA-Z0-9_-]+$`, appName)
		})
	}
}

func TestGenerateBranchName(t *testing.T) {
	tests := []struct {
		name     string
		id       string
		expected string // We'll test the pattern, not exact match due to timestamp
	}{
		{
			name:     "simple id",
			id:       "test-workflow",
			expected: "workflow/test-workflow/",
		},
		{
			name:     "id with underscores",
			id:       "my_workflow_2",
			expected: "workflow/my_workflow_2/",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			branchName := GenerateBranchName(tt.id)

			// Check prefix
			assert.True(t, strings.HasPrefix(branchName, tt.expected))

			// Check it contains timestamp at the end
			parts := strings.Split(branchName, "/")
			assert.Len(t, parts, 3)
			assert.Equal(t, "workflow", parts[0])
			assert.Equal(t, tt.id, parts[1])

			// Timestamp part should be numeric
			assert.Regexp(t, `^\d+$`, parts[2])
		})
	}
}

func TestTransflowStep_OpenRewriteOverrides(t *testing.T) {
	yamlContent := `version: v1alpha1
id: ow-overrides
target_repo: https://example.com/x/y.git
base_ref: main
steps:
  - type: orw-apply
    id: java11to17
    recipes:
      - org.openrewrite.java.migrate.UpgradeToJava17
    recipe_group: org.openrewrite.recipe
    recipe_artifact: rewrite-migrate-java
    recipe_version: 3.17.0
    maven_plugin_version: 6.18.0
    discover_recipe: false
`
	tmp := filepath.Join(t.TempDir(), "tf.yaml")
	require.NoError(t, os.WriteFile(tmp, []byte(yamlContent), 0644))

	cfg, err := LoadConfig(tmp)
	require.NoError(t, err)
	require.Len(t, cfg.Steps, 1)
	s := cfg.Steps[0]
	assert.Equal(t, "orw-apply", s.Type)
	assert.Equal(t, "java11to17", s.ID)
	assert.Equal(t, "org.openrewrite.recipe", s.RecipeGroup)
	assert.Equal(t, "rewrite-migrate-java", s.RecipeArtifact)
	assert.Equal(t, "3.17.0", s.RecipeVersion)
	assert.Equal(t, "6.18.0", s.MavenPluginVersion)
	if assert.NotNil(t, s.DiscoverRecipe) {
		assert.Equal(t, false, *s.DiscoverRecipe)
	}
}
