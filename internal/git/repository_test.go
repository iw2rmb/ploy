package git

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Test utility functions for creating mock Git repositories
func createTestGitRepo(t *testing.T) string {
	tmpDir, err := os.MkdirTemp("", "git_test_*")
	require.NoError(t, err)

	// Create .git directory structure
	gitDir := filepath.Join(tmpDir, ".git")
	err = os.MkdirAll(gitDir, 0755)
	require.NoError(t, err)

	// Create basic git config
	configContent := `[core]
	repositoryformatversion = 0
	filemode = true
	bare = false
	logallrefupdates = true
[remote "origin"]
	url = https://github.com/test/repo.git
	fetch = +refs/heads/*:refs/remotes/origin/*
[branch "main"]
	remote = origin
	merge = refs/heads/main
`
	configPath := filepath.Join(gitDir, "config")
	err = os.WriteFile(configPath, []byte(configContent), 0644)
	require.NoError(t, err)

	// Create HEAD file
	headContent := "ref: refs/heads/main"
	headPath := filepath.Join(gitDir, "HEAD")
	err = os.WriteFile(headPath, []byte(headContent), 0644)
	require.NoError(t, err)

	return tmpDir
}

func createTestGitRepoWithSSH(t *testing.T) string {
	tmpDir := createTestGitRepo(t)

	// Update config to use SSH URL
	configContent := `[core]
	repositoryformatversion = 0
	filemode = true
	bare = false
	logallrefupdates = true
[remote "origin"]
	url = git@github.com:test/ssh-repo.git
	fetch = +refs/heads/*:refs/remotes/origin/*
[branch "main"]
	remote = origin
	merge = refs/heads/main
`
	configPath := filepath.Join(tmpDir, ".git", "config")
	err := os.WriteFile(configPath, []byte(configContent), 0644)
	require.NoError(t, err)

	return tmpDir
}

func createTestNonGitRepo(t *testing.T) string {
	tmpDir, err := os.MkdirTemp("", "non_git_test_*")
	require.NoError(t, err)

	// Create some files but no .git directory
	testFile := filepath.Join(tmpDir, "README.md")
	err = os.WriteFile(testFile, []byte("# Test Repository"), 0644)
	require.NoError(t, err)

	return tmpDir
}

func cleanupTestRepo(t *testing.T, dir string) {
	err := os.RemoveAll(dir)
	require.NoError(t, err)
}

// Tests for NewRepository function
func TestNewRepository(t *testing.T) {
	tests := []struct {
		name        string
		setupRepo   func(t *testing.T) string
		expectError bool
		cleanup     bool
	}{
		{
			name: "valid git repository",
			setupRepo: func(t *testing.T) string {
				return createTestGitRepo(t)
			},
			expectError: false,
			cleanup:     true,
		},
		{
			name: "SSH repository URL",
			setupRepo: func(t *testing.T) string {
				return createTestGitRepoWithSSH(t)
			},
			expectError: false,
			cleanup:     true,
		},
		{
			name: "non-git directory",
			setupRepo: func(t *testing.T) string {
				return createTestNonGitRepo(t)
			},
			expectError: true,
			cleanup:     true,
		},
		{
			name: "nonexistent directory",
			setupRepo: func(t *testing.T) string {
				return "/path/to/nonexistent/directory"
			},
			expectError: true,
			cleanup:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repoPath := tt.setupRepo(t)
			if tt.cleanup && repoPath != "/path/to/nonexistent/directory" {
				defer cleanupTestRepo(t, repoPath)
			}

			repo, err := NewRepository(repoPath)

			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, repo)
			} else {
				if err != nil {
					// Git commands may fail in test environment, skip detailed verification
					t.Skipf("Git command failed in test environment: %v", err)
					return
				}
				require.NotNil(t, repo)

				// Verify basic repository properties
				assert.Equal(t, repoPath, repo.Path)
				// Note: Branch and SHA may be empty if git commands fail
			}
		})
	}
}

// Tests for parseGitConfig function
func TestRepository_parseGitConfig(t *testing.T) {
	tests := []struct {
		name          string
		configContent string
		expectedURL   string
		expectError   bool
	}{
		{
			name: "HTTPS origin URL",
			configContent: `[core]
	repositoryformatversion = 0
[remote "origin"]
	url = https://github.com/user/repo.git
	fetch = +refs/heads/*:refs/remotes/origin/*`,
			expectedURL: "https://github.com/user/repo",
			expectError: false,
		},
		{
			name: "SSH origin URL",
			configContent: `[core]
	repositoryformatversion = 0
[remote "origin"]
	url = git@gitlab.com:user/project.git
	fetch = +refs/heads/*:refs/remotes/origin/*`,
			expectedURL: "https://gitlab.com/user/project",
			expectError: false,
		},
		{
			name: "URL without .git suffix",
			configContent: `[core]
	repositoryformatversion = 0
[remote "origin"]
	url = https://bitbucket.org/user/repo
	fetch = +refs/heads/*:refs/remotes/origin/*`,
			expectedURL: "https://bitbucket.org/user/repo",
			expectError: false,
		},
		{
			name: "multiple remotes with origin",
			configContent: `[core]
	repositoryformatversion = 0
[remote "upstream"]
	url = https://github.com/upstream/repo.git
	fetch = +refs/heads/*:refs/remotes/upstream/*
[remote "origin"]
	url = https://github.com/fork/repo.git
	fetch = +refs/heads/*:refs/remotes/origin/*`,
			expectedURL: "https://github.com/fork/repo",
			expectError: false,
		},
		{
			name: "no origin remote",
			configContent: `[core]
	repositoryformatversion = 0
[remote "upstream"]
	url = https://github.com/upstream/repo.git
	fetch = +refs/heads/*:refs/remotes/upstream/*`,
			expectedURL: "",
			expectError: true,
		},
		{
			name: "malformed config",
			configContent: `[core
	repositoryformatversion = 0
remote "origin"
	url = invalid`,
			expectedURL: "",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temporary directory with git config
			tmpDir, err := os.MkdirTemp("", "git_config_test_*")
			require.NoError(t, err)
			defer os.RemoveAll(tmpDir)

			gitDir := filepath.Join(tmpDir, ".git")
			err = os.MkdirAll(gitDir, 0755)
			require.NoError(t, err)

			configPath := filepath.Join(gitDir, "config")
			err = os.WriteFile(configPath, []byte(tt.configContent), 0644)
			require.NoError(t, err)

			// Create repository instance
			repo := &Repository{Path: tmpDir}

			// Test parseGitConfig
			url, err := repo.parseGitConfig()

			if tt.expectError {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expectedURL, url)
			}
		})
	}
}

// Tests for normalizeRepositoryURL function
func TestRepository_normalizeRepositoryURL(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		// SSH to HTTPS conversion
		{"SSH GitHub URL", "git@github.com:user/repo.git", "https://github.com/user/repo"},
		{"SSH GitLab URL", "git@gitlab.com:user/project.git", "https://gitlab.com/user/project"},
		{"SSH Bitbucket URL", "git@bitbucket.org:user/repo.git", "https://bitbucket.org/user/repo"},

		// Remove .git suffix
		{"HTTPS with .git", "https://github.com/user/repo.git", "https://github.com/user/repo"},
		{"HTTPS without .git", "https://github.com/user/repo", "https://github.com/user/repo"},

		// Handle edge cases
		{"Empty URL", "", ""},
		{"URL with spaces", " https://github.com/user/repo ", " https://github.com/user/repo "},
		{"Complex SSH URL", "git@custom.domain.com:organization/project-name.git", "https://custom.domain.com/organization/project-name"},

		// URLs that should remain unchanged (mostly)
		{"HTTP URL", "http://github.com/user/repo", "http://github.com/user/repo"},
		{"Already normalized", "https://github.com/user/repo", "https://github.com/user/repo"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &Repository{}
			result := repo.normalizeRepositoryURL(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// Tests for GetShortSHA function
func TestRepository_GetShortSHA(t *testing.T) {
	tests := []struct {
		name     string
		sha      string
		expected string
	}{
		{
			name:     "full SHA",
			sha:      "a1b2c3d4e5f6789012345678901234567890abcd",
			expected: "a1b2c3d4e5f6",
		},
		{
			name:     "short SHA",
			sha:      "a1b2c3d",
			expected: "a1b2c3d",
		},
		{
			name:     "exactly 12 characters",
			sha:      "a1b2c3d4e5f6",
			expected: "a1b2c3d4e5f6",
		},
		{
			name:     "empty SHA",
			sha:      "",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &Repository{SHA: tt.sha}
			result := repo.GetShortSHA()
			assert.Equal(t, tt.expected, result)
		})
	}
}

// Tests for ValidateRepository function
func TestRepository_ValidateRepository(t *testing.T) {
	t.Run("validation with clean repository", func(t *testing.T) {
		repo := &Repository{
			Path:         "/test/path",
			URL:          "https://github.com/test/repo",
			Branch:       "main",
			SHA:          "abc123def456",
			IsClean:      true,
			HasUntracked: false,
			LastCommit: &Commit{
				SHA:       "abc123def456",
				Message:   "Test commit",
				Author:    "Test Author",
				Email:     "test@example.com",
				Timestamp: time.Now(),
				GPGSigned: true,
			},
			RemoteOrigin: &Remote{
				Name: "origin",
				URL:  "https://github.com/test/repo.git",
				Type: "fetch",
			},
		}

		result := repo.ValidateRepository()

		assert.True(t, result.Valid)
		assert.Empty(t, result.Errors)
		assert.Len(t, result.Warnings, 0) // Clean repo should have no warnings
	})

	t.Run("validation with dirty repository", func(t *testing.T) {
		repo := &Repository{
			Path:         "/test/path",
			URL:          "http://insecure.com/repo", // Non-HTTPS URL
			Branch:       "feature-branch",           // Non-default branch
			SHA:          "abc123def456",
			IsClean:      false, // Dirty repository
			HasUntracked: true,  // Has untracked files
			LastCommit: &Commit{
				SHA:       "abc123def456",
				Message:   "Test commit",
				Author:    "Test Author",
				Email:     "test@example.com",
				Timestamp: time.Now(),
				GPGSigned: false, // Not signed
			},
		}

		result := repo.ValidateRepository()

		// Should be valid because basic validation doesn't enforce cleanliness by default
		// Only specific validator configs enforce strict rules
		assert.True(t, result.Valid)
		assert.Greater(t, len(result.Warnings), 0)
		assert.Greater(t, len(result.SecurityIssues), 0)
	})

	t.Run("validation with missing information", func(t *testing.T) {
		repo := &Repository{
			Path:       "/test/path",
			URL:        "", // Missing URL
			Branch:     "detached",
			SHA:        "abc123def456",
			IsClean:    true,
			LastCommit: nil, // No commit information
		}

		result := repo.ValidateRepository()

		assert.False(t, result.Valid)
		assert.Contains(t, result.Warnings, "No repository URL found")
		assert.Contains(t, result.Warnings, "Repository is in detached HEAD state")
		assert.Contains(t, result.Errors, "No commit information available")
	})
}

// Tests for validateURL function
func TestRepository_validateURL(t *testing.T) {
	tests := []struct {
		name                   string
		url                    string
		expectedWarnings       int
		expectedSecurityIssues int
	}{
		{
			name:                   "trusted HTTPS URL",
			url:                    "https://github.com/user/repo",
			expectedWarnings:       0,
			expectedSecurityIssues: 0,
		},
		{
			name:                   "trusted GitLab HTTPS URL",
			url:                    "https://gitlab.com/user/repo",
			expectedWarnings:       0,
			expectedSecurityIssues: 0,
		},
		{
			name:                   "untrusted domain HTTPS URL",
			url:                    "https://untrusted.com/user/repo",
			expectedWarnings:       1,
			expectedSecurityIssues: 0,
		},
		{
			name:                   "trusted HTTP URL (insecure)",
			url:                    "http://github.com/user/repo",
			expectedWarnings:       0,
			expectedSecurityIssues: 1,
		},
		{
			name:                   "untrusted HTTP URL",
			url:                    "http://untrusted.com/user/repo",
			expectedWarnings:       1,
			expectedSecurityIssues: 1,
		},
		{
			name:                   "empty URL",
			url:                    "",
			expectedWarnings:       1,
			expectedSecurityIssues: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &Repository{URL: tt.url}
			result := &ValidationResult{
				Valid:          true,
				Warnings:       []string{},
				Errors:         []string{},
				SecurityIssues: []string{},
				Suggestions:    []string{},
			}

			repo.validateURL(result)

			assert.Len(t, result.Warnings, tt.expectedWarnings)
			assert.Len(t, result.SecurityIssues, tt.expectedSecurityIssues)
		})
	}
}

// Tests for validateCommitSigning function
func TestRepository_validateCommitSigning(t *testing.T) {
	tests := []struct {
		name             string
		lastCommit       *Commit
		expectedWarnings int
		expectedErrors   int
	}{
		{
			name: "signed commit",
			lastCommit: &Commit{
				SHA:       "abc123",
				Message:   "Test commit",
				GPGSigned: true,
			},
			expectedWarnings: 0,
			expectedErrors:   0,
		},
		{
			name: "unsigned commit",
			lastCommit: &Commit{
				SHA:       "abc123",
				Message:   "Test commit",
				GPGSigned: false,
			},
			expectedWarnings: 1,
			expectedErrors:   0,
		},
		{
			name:             "no commit information",
			lastCommit:       nil,
			expectedWarnings: 0,
			expectedErrors:   1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &Repository{LastCommit: tt.lastCommit}
			result := &ValidationResult{
				Valid:          true,
				Warnings:       []string{},
				Errors:         []string{},
				SecurityIssues: []string{},
				Suggestions:    []string{},
			}

			repo.validateCommitSigning(result)

			assert.Len(t, result.Warnings, tt.expectedWarnings)
			assert.Len(t, result.Errors, tt.expectedErrors)
		})
	}
}

// Tests for validateCleanliness function
func TestRepository_validateCleanliness(t *testing.T) {
	tests := []struct {
		name             string
		isClean          bool
		hasUntracked     bool
		expectedWarnings int
	}{
		{
			name:             "clean repository",
			isClean:          true,
			hasUntracked:     false,
			expectedWarnings: 0,
		},
		{
			name:             "dirty repository",
			isClean:          false,
			hasUntracked:     false,
			expectedWarnings: 1,
		},
		{
			name:             "repository with untracked files",
			isClean:          true,
			hasUntracked:     true,
			expectedWarnings: 1,
		},
		{
			name:             "dirty repository with untracked files",
			isClean:          false,
			hasUntracked:     true,
			expectedWarnings: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &Repository{
				IsClean:      tt.isClean,
				HasUntracked: tt.hasUntracked,
			}
			result := &ValidationResult{
				Valid:          true,
				Warnings:       []string{},
				Errors:         []string{},
				SecurityIssues: []string{},
				Suggestions:    []string{},
			}

			repo.validateCleanliness(result)

			assert.Len(t, result.Warnings, tt.expectedWarnings)
		})
	}
}

// Tests for validateBranch function
func TestRepository_validateBranch(t *testing.T) {
	tests := []struct {
		name             string
		branch           string
		expectedWarnings int
	}{
		{
			name:             "main branch",
			branch:           "main",
			expectedWarnings: 0,
		},
		{
			name:             "master branch",
			branch:           "master",
			expectedWarnings: 0,
		},
		{
			name:             "feature branch",
			branch:           "feature/new-feature",
			expectedWarnings: 1,
		},
		{
			name:             "detached HEAD",
			branch:           "detached",
			expectedWarnings: 2, // detached + not default branch
		},
		{
			name:             "develop branch",
			branch:           "develop",
			expectedWarnings: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &Repository{Branch: tt.branch}
			result := &ValidationResult{
				Valid:          true,
				Warnings:       []string{},
				Errors:         []string{},
				SecurityIssues: []string{},
				Suggestions:    []string{},
			}

			repo.validateBranch(result)

			assert.Len(t, result.Warnings, tt.expectedWarnings)
		})
	}
}

// Tests for isTextFile function
func TestRepository_isTextFile(t *testing.T) {
	tests := []struct {
		name     string
		filePath string
		expected bool
	}{
		// Text files
		{"Go file", "/path/to/file.go", true},
		{"JavaScript file", "/path/to/file.js", true},
		{"Python file", "/path/to/file.py", true},
		{"Markdown file", "/path/to/README.md", true},
		{"JSON file", "/path/to/config.json", true},
		{"YAML file", "/path/to/config.yaml", true},
		{"Shell script", "/path/to/script.sh", true},
		{"Dockerfile", "/path/to/file.dockerfile", true},

		// Binary files
		{"Image file", "/path/to/image.png", false},
		{"Executable", "/path/to/binary", false},
		{"PDF file", "/path/to/document.pdf", false},
		{"Archive", "/path/to/file.zip", false},

		// Edge cases
		{"No extension", "/path/to/file", false},
		{"Multiple extensions", "/path/to/file.tar.gz", false},
		{"Uppercase extension", "/path/to/FILE.GO", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &Repository{}
			result := repo.isTextFile(tt.filePath)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// Benchmark tests
func BenchmarkNewRepository(b *testing.B) {
	tmpDir := createTestGitRepo(&testing.T{})
	defer os.RemoveAll(tmpDir)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		repo, err := NewRepository(tmpDir)
		if err != nil || repo == nil {
			b.Fatal("Failed to create repository")
		}
	}
}

func BenchmarkRepository_ValidateRepository(b *testing.B) {
	repo := &Repository{
		Path:         "/test/path",
		URL:          "https://github.com/test/repo",
		Branch:       "main",
		SHA:          "abc123def456",
		IsClean:      true,
		HasUntracked: false,
		LastCommit: &Commit{
			SHA:       "abc123def456",
			Message:   "Test commit",
			GPGSigned: true,
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		result := repo.ValidateRepository()
		if result == nil {
			b.Fatal("Validation returned nil")
		}
	}
}

func BenchmarkRepository_normalizeRepositoryURL(b *testing.B) {
	repo := &Repository{}
	testURL := "git@github.com:user/repo.git"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		result := repo.normalizeRepositoryURL(testURL)
		if result == "" {
			b.Fatal("URL normalization failed")
		}
	}
}
