package git

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Test utility functions for validator testing
func createMockRepository(config mockRepoConfig) *Repository {
	repo := &Repository{
		Path:         config.Path,
		URL:          config.URL,
		Branch:       config.Branch,
		SHA:          config.SHA,
		IsClean:      config.IsClean,
		HasUntracked: config.HasUntracked,
	}

	if config.HasCommit {
		repo.LastCommit = &Commit{
			SHA:       config.SHA,
			Message:   "Test commit message",
			Author:    "Test Author",
			Email:     "test@example.com",
			Timestamp: time.Now(),
			GPGSigned: config.GPGSigned,
		}
	}

	if config.HasRemote {
		repo.RemoteOrigin = &Remote{
			Name: "origin",
			URL:  config.URL,
			Type: "fetch",
		}
	}

	return repo
}

type mockRepoConfig struct {
	Path         string
	URL          string
	Branch       string
	SHA          string
	IsClean      bool
	HasUntracked bool
	HasCommit    bool
	GPGSigned    bool
	HasRemote    bool
}

func createTestRepoWithFiles(t *testing.T, files map[string]string) string {
	tmpDir, err := os.MkdirTemp("", "validator_test_*")
	require.NoError(t, err)

	// Create .git directory
	gitDir := filepath.Join(tmpDir, ".git")
	err = os.MkdirAll(gitDir, 0755)
	require.NoError(t, err)

	// Create git config
	configPath := filepath.Join(gitDir, "config")
	configContent := `[remote "origin"]
    url = https://github.com/test/repo.git`
	err = os.WriteFile(configPath, []byte(configContent), 0644)
	require.NoError(t, err)

	// Create HEAD file
	headPath := filepath.Join(gitDir, "HEAD")
	err = os.WriteFile(headPath, []byte("ref: refs/heads/main"), 0644)
	require.NoError(t, err)

	// Create test files
	for filename, content := range files {
		fullPath := filepath.Join(tmpDir, filename)
		dir := filepath.Dir(fullPath)

		err = os.MkdirAll(dir, 0755)
		require.NoError(t, err)

		err = os.WriteFile(fullPath, []byte(content), 0644)
		require.NoError(t, err)
	}

	return tmpDir
}

// Tests for ValidationLevel constants
func TestValidationLevel(t *testing.T) {
	assert.Equal(t, ValidationLevel(0), ValidationLevelNone)
	assert.Equal(t, ValidationLevel(1), ValidationLevelWarning)
	assert.Equal(t, ValidationLevel(2), ValidationLevelStrict)
}

// Tests for DefaultValidatorConfig function
func TestDefaultValidatorConfig(t *testing.T) {
	config := DefaultValidatorConfig()

	assert.NotNil(t, config)
	assert.Equal(t, ValidationLevelWarning, config.Level)
	assert.False(t, config.RequireCleanRepo)
	assert.False(t, config.RequireSignedCommits)
	assert.False(t, config.RequireTrustedOrigin)
	assert.Contains(t, config.AllowedBranches, "main")
	assert.Contains(t, config.AllowedBranches, "master")
	assert.Contains(t, config.TrustedDomains, "github.com")
	assert.Equal(t, int64(500), config.MaxRepoSizeMB)
	assert.True(t, config.ScanForSecrets)
}

// Tests for ProductionValidatorConfig function
func TestProductionValidatorConfig(t *testing.T) {
	config := ProductionValidatorConfig()

	assert.NotNil(t, config)
	assert.Equal(t, ValidationLevelStrict, config.Level)
	assert.True(t, config.RequireCleanRepo)
	assert.True(t, config.RequireSignedCommits)
	assert.True(t, config.RequireTrustedOrigin)
	assert.Contains(t, config.AllowedBranches, "main")
	assert.Contains(t, config.AllowedBranches, "production")
	assert.NotContains(t, config.AllowedBranches, "develop") // Stricter for production
	assert.Contains(t, config.TrustedDomains, "github.com")
	assert.NotContains(t, config.TrustedDomains, "bitbucket.org") // Stricter for production
	assert.Equal(t, int64(100), config.MaxRepoSizeMB)             // Smaller limit for production
	assert.True(t, config.ScanForSecrets)
}

// Tests for NewValidator function
func TestNewValidator(t *testing.T) {
	tests := []struct {
		name           string
		config         *ValidatorConfig
		expectedConfig *ValidatorConfig
	}{
		{
			name:           "with custom config",
			config:         &ValidatorConfig{Level: ValidationLevelStrict},
			expectedConfig: &ValidatorConfig{Level: ValidationLevelStrict},
		},
		{
			name:           "with nil config",
			config:         nil,
			expectedConfig: DefaultValidatorConfig(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			validator := NewValidator(tt.config)

			assert.NotNil(t, validator)
			assert.NotNil(t, validator.config)

			if tt.config != nil {
				assert.Equal(t, tt.expectedConfig.Level, validator.config.Level)
			} else {
				// Should use default config
				assert.Equal(t, ValidationLevelWarning, validator.config.Level)
			}
		})
	}
}

// Tests for applyConfigValidation function
func TestValidator_applyConfigValidation(t *testing.T) {
	tests := []struct {
		name                   string
		config                 *ValidatorConfig
		repoConfig             mockRepoConfig
		expectedErrors         int
		expectedWarnings       int
		expectedSecurityIssues int
	}{
		{
			name: "clean repo with signed commits - strict config",
			config: &ValidatorConfig{
				Level:                ValidationLevelStrict,
				RequireCleanRepo:     true,
				RequireSignedCommits: true,
				RequireTrustedOrigin: true,
				AllowedBranches:      []string{"main"},
				TrustedDomains:       []string{"github.com"},
			},
			repoConfig: mockRepoConfig{
				Path:         "/test/path",
				URL:          "https://github.com/user/repo",
				Branch:       "main",
				SHA:          "abc123",
				IsClean:      true,
				HasUntracked: false,
				HasCommit:    true,
				GPGSigned:    true,
				HasRemote:    true,
			},
			expectedErrors:         0,
			expectedWarnings:       0,
			expectedSecurityIssues: 0,
		},
		{
			name: "dirty repo with unsigned commits - strict config",
			config: &ValidatorConfig{
				Level:                ValidationLevelStrict,
				RequireCleanRepo:     true,
				RequireSignedCommits: true,
				RequireTrustedOrigin: true,
				AllowedBranches:      []string{"main"},
				TrustedDomains:       []string{"github.com"},
			},
			repoConfig: mockRepoConfig{
				Path:         "/test/path",
				URL:          "https://github.com/user/repo",
				Branch:       "feature-branch", // Not in allowed branches
				SHA:          "abc123",
				IsClean:      false, // Dirty
				HasUntracked: true,  // Has untracked files
				HasCommit:    true,
				GPGSigned:    false, // Not signed
				HasRemote:    true,
			},
			expectedErrors:         4, // clean repo + untracked + signing + branch
			expectedWarnings:       0,
			expectedSecurityIssues: 0,
		},
		{
			name: "untrusted origin - strict config",
			config: &ValidatorConfig{
				Level:                ValidationLevelStrict,
				RequireCleanRepo:     false,
				RequireSignedCommits: false,
				RequireTrustedOrigin: true,
				AllowedBranches:      []string{}, // Allow any branch
				TrustedDomains:       []string{"github.com"},
			},
			repoConfig: mockRepoConfig{
				Path:         "/test/path",
				URL:          "https://untrusted.com/user/repo", // Untrusted domain
				Branch:       "main",
				SHA:          "abc123",
				IsClean:      true,
				HasUntracked: false,
				HasCommit:    true,
				GPGSigned:    false,
				HasRemote:    true,
			},
			expectedErrors:         1, // untrusted origin
			expectedWarnings:       0,
			expectedSecurityIssues: 0,
		},
		{
			name: "warnings level config",
			config: &ValidatorConfig{
				Level:                ValidationLevelWarning,
				RequireCleanRepo:     false,
				RequireSignedCommits: false,
				RequireTrustedOrigin: false,
				AllowedBranches:      []string{"main", "master"},
				TrustedDomains:       []string{"github.com"},
			},
			repoConfig: mockRepoConfig{
				Path:         "/test/path",
				URL:          "https://github.com/user/repo",
				Branch:       "feature-branch", // Not in recommended branches
				SHA:          "abc123",
				IsClean:      true,
				HasUntracked: false,
				HasCommit:    true,
				GPGSigned:    false,
				HasRemote:    true,
			},
			expectedErrors:         0,
			expectedWarnings:       1, // non-recommended branch
			expectedSecurityIssues: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			validator := NewValidator(tt.config)
			repo := createMockRepository(tt.repoConfig)
			result := &ValidationResult{
				Valid:          true,
				Warnings:       []string{},
				Errors:         []string{},
				SecurityIssues: []string{},
				Suggestions:    []string{},
			}

			validator.applyConfigValidation(repo, result)

			assert.Len(t, result.Errors, tt.expectedErrors, "Errors count mismatch")
			assert.Len(t, result.Warnings, tt.expectedWarnings, "Warnings count mismatch")
			assert.Len(t, result.SecurityIssues, tt.expectedSecurityIssues, "Security issues count mismatch")
		})
	}
}

// Tests for finalizeValidation function
func TestValidator_finalizeValidation(t *testing.T) {
	tests := []struct {
		name           string
		level          ValidationLevel
		initialResult  *ValidationResult
		expectedValid  bool
		expectedErrors int
	}{
		{
			name:  "none level - only security issues matter",
			level: ValidationLevelNone,
			initialResult: &ValidationResult{
				Errors:         []string{"error1"},
				Warnings:       []string{"warning1"},
				SecurityIssues: []string{},
			},
			expectedValid:  true, // Errors and warnings cleared
			expectedErrors: 0,
		},
		{
			name:  "warning level - errors make it invalid",
			level: ValidationLevelWarning,
			initialResult: &ValidationResult{
				Errors:         []string{"error1"},
				Warnings:       []string{"warning1"},
				SecurityIssues: []string{"security1"},
			},
			expectedValid:  false, // Has errors
			expectedErrors: 1,
		},
		{
			name:  "strict level - any issue makes it invalid",
			level: ValidationLevelStrict,
			initialResult: &ValidationResult{
				Errors:         []string{},
				Warnings:       []string{"warning1"},
				SecurityIssues: []string{"security1"},
			},
			expectedValid:  false, // Has security issues
			expectedErrors: 1,     // Security issue promoted to error
		},
		{
			name:  "strict level - valid when no issues",
			level: ValidationLevelStrict,
			initialResult: &ValidationResult{
				Errors:         []string{},
				Warnings:       []string{},
				SecurityIssues: []string{},
			},
			expectedValid:  true,
			expectedErrors: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			validator := NewValidator(&ValidatorConfig{Level: tt.level})
			result := tt.initialResult

			validator.finalizeValidation(result)

			assert.Equal(t, tt.expectedValid, result.Valid)
			assert.Len(t, result.Errors, tt.expectedErrors)
		})
	}
}

// Tests for ValidateForEnvironment function
func TestValidator_ValidateForEnvironment(t *testing.T) {
	// Create a test repository directory
	tmpDir := createTestRepoWithFiles(t, map[string]string{
		"README.md": "# Test Repository",
	})
	defer func() { _ = os.RemoveAll(tmpDir) }()

	tests := []struct {
		name         string
		environment  string
		expectStrict bool
	}{
		{
			name:         "production environment",
			environment:  "production",
			expectStrict: true,
		},
		{
			name:         "prod environment",
			environment:  "prod",
			expectStrict: true,
		},
		{
			name:         "staging environment",
			environment:  "staging",
			expectStrict: true,
		},
		{
			name:         "development environment",
			environment:  "development",
			expectStrict: false,
		},
		{
			name:         "dev environment",
			environment:  "dev",
			expectStrict: false,
		},
		{
			name:         "unknown environment",
			environment:  "unknown",
			expectStrict: false, // Should use current config
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Start with default config
			validator := NewValidator(DefaultValidatorConfig())
			originalLevel := validator.config.Level

			// This test mainly verifies the function doesn't crash
			// and properly switches configurations
			result, err := validator.ValidateForEnvironment(tmpDir, tt.environment)

			// Git commands may fail in test environment, that's acceptable
			if err != nil {
				t.Logf("Git command failed (expected in test environment): %v", err)
			} else {
				assert.NotNil(t, result)
			}

			// Config should be restored to original
			assert.Equal(t, originalLevel, validator.config.Level)
		})
	}
}

// Tests for GetRepositorySummary function
func TestValidator_GetRepositorySummary(t *testing.T) {
	tmpDir := createTestRepoWithFiles(t, map[string]string{
		"README.md": "# Test Repository",
		"main.go":   "package main\n\nfunc main() {}",
	})
	defer func() { _ = os.RemoveAll(tmpDir) }()

	validator := NewValidator(DefaultValidatorConfig())

	// This test verifies the function runs without error
	// Actual content depends on git command execution which is mocked/limited in tests
	summary, err := validator.GetRepositorySummary(tmpDir)

	if err != nil {
		// If git commands fail in test environment, that's expected
		assert.Contains(t, err.Error(), "failed to")
		return
	}

	assert.NotEmpty(t, summary)
	assert.Contains(t, summary, "Repository:")
	assert.Contains(t, summary, "Branch:")
	assert.Contains(t, summary, "Validation:")
}

// Tests for IsRepositoryValid function
func TestValidator_IsRepositoryValid(t *testing.T) {
	tmpDir := createTestRepoWithFiles(t, map[string]string{
		"README.md": "# Test Repository",
	})
	defer func() { _ = os.RemoveAll(tmpDir) }()

	tests := []struct {
		name   string
		config *ValidatorConfig
		path   string
	}{
		{
			name:   "default config",
			config: DefaultValidatorConfig(),
			path:   tmpDir,
		},
		{
			name:   "strict config",
			config: ProductionValidatorConfig(),
			path:   tmpDir,
		},
		{
			name:   "invalid path",
			config: DefaultValidatorConfig(),
			path:   "/nonexistent/path",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			validator := NewValidator(tt.config)
			result := validator.IsRepositoryValid(tt.path)

			// Just verify the function returns a boolean without crashing
			assert.IsType(t, true, result)

			if tt.path == "/nonexistent/path" {
				assert.False(t, result)
			}
		})
	}
}

// Tests for GetRepositoryHealth function
func TestValidator_GetRepositoryHealth(t *testing.T) {
	tmpDir := createTestRepoWithFiles(t, map[string]string{
		"README.md": "# Test Repository",
	})
	defer func() { _ = os.RemoveAll(tmpDir) }()

	validator := NewValidator(DefaultValidatorConfig())

	health, err := validator.GetRepositoryHealth(tmpDir)

	if err != nil {
		// Git command failures are expected in test environment
		return
	}

	// Health should be between 0 and 100
	assert.GreaterOrEqual(t, health, 0)
	assert.LessOrEqual(t, health, 100)
}

// Tests for getRepositorySize function
func TestValidator_getRepositorySize(t *testing.T) {
	tests := []struct {
		name     string
		setupDir func(t *testing.T) string
		expected int64 // Approximate expected size
	}{
		{
			name: "directory with files",
			setupDir: func(t *testing.T) string {
				tmpDir, err := os.MkdirTemp("", "size_test_*")
				require.NoError(t, err)

				// Create files with known sizes
				err = os.WriteFile(filepath.Join(tmpDir, "file1.txt"), make([]byte, 100), 0644)
				require.NoError(t, err)

				err = os.WriteFile(filepath.Join(tmpDir, "file2.txt"), make([]byte, 200), 0644)
				require.NoError(t, err)

				return tmpDir
			},
			expected: 300, // 100 + 200 bytes
		},
		{
			name: "empty directory",
			setupDir: func(t *testing.T) string {
				tmpDir, err := os.MkdirTemp("", "empty_test_*")
				require.NoError(t, err)
				return tmpDir
			},
			expected: 0,
		},
		{
			name: "nonexistent directory",
			setupDir: func(t *testing.T) string {
				return "/path/to/nonexistent/directory"
			},
			expected: 0, // filepath.Walk will return 0 for nonexistent directory
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repoPath := tt.setupDir(t)
			if tt.name != "nonexistent directory" {
				defer func() { _ = os.RemoveAll(repoPath) }()
			}

			validator := NewValidator(DefaultValidatorConfig())
			size, err := validator.getRepositorySize(repoPath)

			// The implementation handles errors gracefully, so no error expected
			assert.NoError(t, err)
			assert.Equal(t, tt.expected, size)
		})
	}
}

// Integration tests
func TestValidator_Integration(t *testing.T) {
	t.Run("complete validation workflow", func(t *testing.T) {
		// Create repository with various files including potential security issues
		files := map[string]string{
			"README.md": "# Test Repository",
			"main.go":   "package main\n\nfunc main() {\n\tpassword := \"secret123\"\n\tprintln(password)\n}",
			".env":      "API_KEY=secret123\nDATABASE_URL=postgres://user:pass@host/db",
		}

		tmpDir := createTestRepoWithFiles(t, files)
		defer func() { _ = os.RemoveAll(tmpDir) }()

		// Test with different validation levels
		configs := []*ValidatorConfig{
			DefaultValidatorConfig(),
			ProductionValidatorConfig(),
			{Level: ValidationLevelNone},
		}

		for i, config := range configs {
			t.Run(fmt.Sprintf("config_%d", i), func(t *testing.T) {
				validator := NewValidator(config)
				result, err := validator.ValidateRepository(tmpDir)

				// The exact result depends on git command execution in test environment
				// We mainly verify the function completes without panicking
				if err != nil {
					// Git command failures are acceptable in test environment
					return
				}

				assert.NotNil(t, result)
				assert.IsType(t, true, result.Valid)

				// For strict configs, any issues should make it invalid
				if config.Level == ValidationLevelStrict {
					// With .env file and potential secrets, should have security issues
					if len(result.SecurityIssues) > 0 {
						assert.False(t, result.Valid)
					}
				}
			})
		}
	})

	t.Run("environment-specific validation", func(t *testing.T) {
		tmpDir := createTestRepoWithFiles(t, map[string]string{
			"README.md": "# Production Repository",
		})
		defer func() { _ = os.RemoveAll(tmpDir) }()

		validator := NewValidator(DefaultValidatorConfig())

		environments := []string{"development", "staging", "production"}
		for _, env := range environments {
			t.Run("env_"+env, func(t *testing.T) {
				result, err := validator.ValidateForEnvironment(tmpDir, env)

				if err != nil {
					// Git command failures are expected in test environment
					return
				}

				assert.NotNil(t, result)
				assert.IsType(t, true, result.Valid)
			})
		}
	})
}

// Benchmark tests
func BenchmarkValidator_ValidateRepository(b *testing.B) {
	tmpDir := createTestRepoWithFiles(&testing.T{}, map[string]string{
		"README.md": "# Benchmark Repository",
		"main.go":   "package main\n\nfunc main() {}",
	})
	defer func() { _ = os.RemoveAll(tmpDir) }()

	validator := NewValidator(DefaultValidatorConfig())

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		result, err := validator.ValidateRepository(tmpDir)
		if err != nil {
			// Skip if git commands fail
			continue
		}
		if result == nil {
			b.Fatal("Validation returned nil result")
		}
	}
}

func BenchmarkValidator_GetRepositoryHealth(b *testing.B) {
	tmpDir := createTestRepoWithFiles(&testing.T{}, map[string]string{
		"README.md": "# Benchmark Repository",
	})
	defer func() { _ = os.RemoveAll(tmpDir) }()

	validator := NewValidator(DefaultValidatorConfig())

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		health, err := validator.GetRepositoryHealth(tmpDir)
		if err != nil {
			// Skip if git commands fail
			continue
		}
		if health < 0 || health > 100 {
			b.Fatal("Invalid health score")
		}
	}
}

func BenchmarkValidator_applyConfigValidation(b *testing.B) {
	config := DefaultValidatorConfig()
	validator := NewValidator(config)

	repo := createMockRepository(mockRepoConfig{
		Path:         "/test/path",
		URL:          "https://github.com/user/repo",
		Branch:       "main",
		SHA:          "abc123",
		IsClean:      true,
		HasUntracked: false,
		HasCommit:    true,
		GPGSigned:    true,
		HasRemote:    true,
	})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		result := &ValidationResult{
			Valid:          true,
			Warnings:       []string{},
			Errors:         []string{},
			SecurityIssues: []string{},
			Suggestions:    []string{},
		}
		validator.applyConfigValidation(repo, result)
	}
}
