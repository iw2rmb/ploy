package hydration

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	types "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

// Package hydration_test contains tests for GitFetcher that are designed to be
// deterministic and network-free to avoid flakiness from external dependencies.
//
// Design Decisions:
// - All Fetch tests use local test repositories created on the fly with file:// URLs.
// - This eliminates network flakiness and external reference drift (e.g., remote commits being deleted).
// - Test repositories are created in temporary directories and cleaned up automatically via t.TempDir().
// - The only external dependency is the git CLI, which is checked at test start via exec.LookPath.
//
// URL Usage:
// - Actual Fetch operations: file:// URLs pointing to local repos (zero network dependency).
// - Error validation tests: fake https:// URLs that never trigger network calls (fail validation early).
// - Cache key tests: https:// URLs for string manipulation only (no network calls).

func TestGitFetcher_Fetch(t *testing.T) {
	// Skip if git is not available.
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git command not found, skipping test")
	}

	// Create a shared test repository for commit pinning tests.
	// We must create this once and reuse it to ensure the commit SHA exists in the repo.
	sharedRepoWithCommits := setupTestGitRepoWithCommits(t)
	secondCommitSHA := getSecondCommitSHA(t, sharedRepoWithCommits)

	tests := []struct {
		name      string
		repo      *contracts.RepoMaterialization
		setup     func(t *testing.T) string
		wantErr   bool
		errSubstr string
	}{
		{
			name:      "nil repo",
			repo:      nil,
			setup:     func(t *testing.T) string { return "" },
			wantErr:   true,
			errSubstr: "repo materialization is required",
		},
		{
			name: "invalid repo URL",
			repo: &contracts.RepoMaterialization{
				URL:       types.RepoURL("invalid-url"),
				TargetRef: types.GitRef("main"),
			},
			setup:     func(t *testing.T) string { return t.TempDir() },
			wantErr:   true,
			errSubstr: "invalid repo",
		},
		{
			name: "missing target_ref and commit",
			repo: &contracts.RepoMaterialization{
				// Using a fake URL for validation tests; no network call is made
				// because validation fails before any git operations.
				URL: types.RepoURL("https://github.com/example/repo.git"),
			},
			setup:     func(t *testing.T) string { return t.TempDir() },
			wantErr:   true,
			errSubstr: "target_ref or commit is required",
		},
		{
			name: "valid repo with base_ref only",
			repo: &contracts.RepoMaterialization{
				URL:       types.RepoURL("file://" + setupTestGitRepo(t, "base")),
				BaseRef:   types.GitRef("main"),
				TargetRef: types.GitRef("main"),
			},
			setup:   func(t *testing.T) string { return t.TempDir() },
			wantErr: false,
		},
		{
			name: "valid repo with different target_ref",
			repo: &contracts.RepoMaterialization{
				URL:       types.RepoURL("file://" + setupTestGitRepo(t, "target")),
				BaseRef:   types.GitRef("main"),
				TargetRef: types.GitRef("feature"),
			},
			setup:   func(t *testing.T) string { return t.TempDir() },
			wantErr: false,
		},
		{
			name: "valid repo with commit_sha",
			repo: &contracts.RepoMaterialization{
				URL:       types.RepoURL("file://" + setupTestGitRepo(t, "commit")),
				BaseRef:   types.GitRef("main"),
				TargetRef: types.GitRef("main"),
				Commit:    types.CommitSHA("HEAD"),
			},
			setup:   func(t *testing.T) string { return t.TempDir() },
			wantErr: false,
		},
		{
			name: "shallow clone with base_ref only creates minimal history",
			repo: &contracts.RepoMaterialization{
				URL:       types.RepoURL("file://" + setupTestGitRepo(t, "shallow")),
				BaseRef:   types.GitRef("main"),
				TargetRef: types.GitRef("main"),
			},
			setup:   func(t *testing.T) string { return t.TempDir() },
			wantErr: false,
		},
		{
			name: "commit_sha pins to specific commit for deterministic base",
			repo: &contracts.RepoMaterialization{
				URL:       types.RepoURL("file://" + sharedRepoWithCommits),
				BaseRef:   types.GitRef("main"),
				TargetRef: types.GitRef("main"),
				Commit:    types.CommitSHA(secondCommitSHA),
			},
			setup:   func(t *testing.T) string { return t.TempDir() },
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			dest := tt.setup(t)

			fetcher, err := NewGitFetcher(GitFetcherOptions{})
			if err != nil {
				t.Fatalf("NewGitFetcher() error = %v", err)
			}

			err = fetcher.Fetch(ctx, tt.repo, dest)
			if (err != nil) != tt.wantErr {
				t.Errorf("Fetch() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.wantErr && tt.errSubstr != "" && !strings.Contains(err.Error(), tt.errSubstr) {
				t.Errorf("Fetch() error = %v, want substring %q", err, tt.errSubstr)
			}

			if !tt.wantErr {
				// Verify .git directory exists.
				gitDir := filepath.Join(dest, ".git")
				if _, err := os.Stat(gitDir); os.IsNotExist(err) {
					t.Errorf("expected .git directory at %s", gitDir)
				}
			}
		})
	}
}

// setupTestGitRepo creates a local temporary git repository for testing GitFetcher
// without network dependencies. This eliminates flakiness from external references
// (e.g., remote commits being force-pushed or deleted).
//
// The repository is created in a temporary directory managed by t.TempDir(),
// ensuring automatic cleanup. Tests use file:// URLs to fetch from these local repos,
// making the test suite fully deterministic and network-free.
//
// Parameters:
//   - variant: determines the repo structure ("target" creates a feature branch, others create main only)
//
// Returns the absolute path to the created repository (use with file:// prefix for Fetch tests).
func setupTestGitRepo(t *testing.T, variant string) string {
	t.Helper()

	repoDir := t.TempDir()

	// Initialize git repo.
	runCmd(t, repoDir, "git", "init")
	// Ensure the default branch is 'main' for deterministic tests across environments.
	runCmd(t, repoDir, "git", "checkout", "-b", "main")
	runCmd(t, repoDir, "git", "config", "user.name", "Test User")
	runCmd(t, repoDir, "git", "config", "user.email", "test@example.com")

	// Create initial commit on main.
	readme := filepath.Join(repoDir, "README.md")
	if err := os.WriteFile(readme, []byte("# Test Repo\n"), 0644); err != nil {
		t.Fatalf("failed to write README: %v", err)
	}
	runCmd(t, repoDir, "git", "add", "README.md")
	runCmd(t, repoDir, "git", "commit", "-m", "Initial commit")

	// For target variant, create a feature branch.
	if variant == "target" {
		runCmd(t, repoDir, "git", "checkout", "-b", "feature")
		feature := filepath.Join(repoDir, "feature.txt")
		if err := os.WriteFile(feature, []byte("feature content\n"), 0644); err != nil {
			t.Fatalf("failed to write feature.txt: %v", err)
		}
		runCmd(t, repoDir, "git", "add", "feature.txt")
		runCmd(t, repoDir, "git", "commit", "-m", "Add feature")
		runCmd(t, repoDir, "git", "checkout", "main")
	}

	return repoDir
}

// setupTestGitRepoWithCommits creates a local test repository with multiple commits
// for testing commit_sha pinning without network dependencies. This ensures
// deterministic behavior when testing commit-specific fetches.
//
// The repository contains three commits on the main branch, allowing tests to
// verify that GitFetcher can pin to specific historical commits. All commits
// are local, eliminating any risk of external reference drift.
//
// Returns the absolute path to the created repository (use with file:// prefix).
func setupTestGitRepoWithCommits(t *testing.T) string {
	t.Helper()

	repoDir := t.TempDir()

	// Initialize git repo.
	runCmd(t, repoDir, "git", "init")
	runCmd(t, repoDir, "git", "checkout", "-b", "main")
	runCmd(t, repoDir, "git", "config", "user.name", "Test User")
	runCmd(t, repoDir, "git", "config", "user.email", "test@example.com")

	// Create first commit.
	readme := filepath.Join(repoDir, "README.md")
	if err := os.WriteFile(readme, []byte("# First commit\n"), 0644); err != nil {
		t.Fatalf("failed to write README: %v", err)
	}
	runCmd(t, repoDir, "git", "add", "README.md")
	runCmd(t, repoDir, "git", "commit", "-m", "First commit")

	// Create second commit.
	if err := os.WriteFile(readme, []byte("# Second commit\n"), 0644); err != nil {
		t.Fatalf("failed to update README: %v", err)
	}
	runCmd(t, repoDir, "git", "add", "README.md")
	runCmd(t, repoDir, "git", "commit", "-m", "Second commit")

	// Create third commit.
	if err := os.WriteFile(readme, []byte("# Third commit\n"), 0644); err != nil {
		t.Fatalf("failed to update README: %v", err)
	}
	runCmd(t, repoDir, "git", "add", "README.md")
	runCmd(t, repoDir, "git", "commit", "-m", "Third commit")

	return repoDir
}

// getSecondCommitSHA returns the SHA of the second commit (HEAD~1) in a local
// test repository. This helper is used to test commit pinning functionality
// with deterministic local references instead of hard-coded external commit SHAs
// that might drift or be deleted.
func getSecondCommitSHA(t *testing.T, repoDir string) string {
	t.Helper()
	cmd := exec.Command("git", "rev-parse", "HEAD~1")
	cmd.Dir = repoDir
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("failed to get second commit SHA: %v (output: %s)", err, string(output))
	}
	return strings.TrimSpace(string(output))
}

// runCmd executes a git command in the specified directory. This helper is used
// by test fixture setup functions to initialize local test repositories without
// network dependencies.
func runCmd(t *testing.T, dir string, name string, args ...string) {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("command %s %v failed: %v (output: %s)", name, args, err, string(output))
	}
}

func TestNewGitFetcher(t *testing.T) {
	tests := []struct {
		name    string
		opts    GitFetcherOptions
		wantErr bool
	}{
		{
			name:    "default options",
			opts:    GitFetcherOptions{},
			wantErr: false,
		},
		{
			name:    "with CacheDir set",
			opts:    GitFetcherOptions{CacheDir: "/tmp/cache"},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NewGitFetcher(tt.opts)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewGitFetcher() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && got == nil {
				t.Error("NewGitFetcher() returned nil")
			}
		})
	}
}

// TestGitFetcher_CacheDir validates the cache directory functionality.
func TestGitFetcher_CacheDir(t *testing.T) {
	// Skip if git is not available.
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git command not found, skipping test")
	}

	// rsync is required for copyGitClone used by the cache path.
	if _, err := exec.LookPath("rsync"); err != nil {
		t.Skip("rsync command not found, skipping cache dir tests")
	}

	t.Run("cache miss performs fresh clone", func(t *testing.T) {
		t.Parallel()

		// Setup: Create a test repo and cache directory.
		repoDir := setupTestGitRepo(t, "cache-miss")
		cacheDir := t.TempDir()
		dest := t.TempDir()

		fetcher, err := NewGitFetcher(GitFetcherOptions{
			CacheDir: cacheDir,
		})
		if err != nil {
			t.Fatalf("NewGitFetcher() error = %v", err)
		}

		// Fetch with caching enabled (first fetch, should be a cache miss).
		repo := &contracts.RepoMaterialization{
			URL:       types.RepoURL("file://" + repoDir),
			BaseRef:   types.GitRef("main"),
			TargetRef: types.GitRef("main"),
		}

		ctx := context.Background()
		if err := fetcher.Fetch(ctx, repo, dest); err != nil {
			t.Fatalf("Fetch() error = %v", err)
		}

		// Verify .git directory exists in dest.
		gitDir := filepath.Join(dest, ".git")
		if _, err := os.Stat(gitDir); os.IsNotExist(err) {
			t.Errorf("expected .git directory at %s", gitDir)
		}

		// Verify cache was populated (cache directory should exist).
		cacheCloneDir := filepath.Join(cacheDir, "git-clones")
		if _, err := os.Stat(cacheCloneDir); os.IsNotExist(err) {
			t.Errorf("expected cache directory at %s", cacheCloneDir)
		}
	})

	t.Run("cache hit reuses cached clone", func(t *testing.T) {
		t.Parallel()

		// Setup: Create a test repo and cache directory.
		repoDir := setupTestGitRepo(t, "cache-hit")
		cacheDir := t.TempDir()
		dest1 := t.TempDir()
		dest2 := t.TempDir()

		fetcher, err := NewGitFetcher(GitFetcherOptions{
			CacheDir: cacheDir,
		})
		if err != nil {
			t.Fatalf("NewGitFetcher() error = %v", err)
		}

		repo := &contracts.RepoMaterialization{
			URL:       types.RepoURL("file://" + repoDir),
			BaseRef:   types.GitRef("main"),
			TargetRef: types.GitRef("main"),
		}

		ctx := context.Background()

		// First fetch: populate cache.
		if err := fetcher.Fetch(ctx, repo, dest1); err != nil {
			t.Fatalf("Fetch() first call error = %v", err)
		}

		// Second fetch: should reuse cache.
		if err := fetcher.Fetch(ctx, repo, dest2); err != nil {
			t.Fatalf("Fetch() second call error = %v", err)
		}

		// Verify both destinations have .git directories.
		for i, dest := range []string{dest1, dest2} {
			gitDir := filepath.Join(dest, ".git")
			if _, err := os.Stat(gitDir); os.IsNotExist(err) {
				t.Errorf("dest%d: expected .git directory at %s", i+1, gitDir)
			}
		}

		// Verify both destinations have the same content (README.md).
		readme1Path := filepath.Join(dest1, "README.md")
		readme2Path := filepath.Join(dest2, "README.md")
		content1, err := os.ReadFile(readme1Path)
		if err != nil {
			t.Fatalf("failed to read README.md from dest1: %v", err)
		}
		content2, err := os.ReadFile(readme2Path)
		if err != nil {
			t.Fatalf("failed to read README.md from dest2: %v", err)
		}
		if string(content1) != string(content2) {
			t.Errorf("cached clone content mismatch: %q vs %q", string(content1), string(content2))
		}
	})

	t.Run("different repos get separate cache entries", func(t *testing.T) {
		t.Parallel()

		// Setup: Create two different test repos.
		repo1Dir := setupTestGitRepo(t, "repo1")
		repo2Dir := setupTestGitRepo(t, "repo2")
		cacheDir := t.TempDir()
		dest1 := t.TempDir()
		dest2 := t.TempDir()

		fetcher, err := NewGitFetcher(GitFetcherOptions{
			CacheDir: cacheDir,
		})
		if err != nil {
			t.Fatalf("NewGitFetcher() error = %v", err)
		}

		ctx := context.Background()

		// Fetch repo1.
		repo1 := &contracts.RepoMaterialization{
			URL:       types.RepoURL("file://" + repo1Dir),
			BaseRef:   types.GitRef("main"),
			TargetRef: types.GitRef("main"),
		}
		if err := fetcher.Fetch(ctx, repo1, dest1); err != nil {
			t.Fatalf("Fetch() repo1 error = %v", err)
		}

		// Fetch repo2.
		repo2 := &contracts.RepoMaterialization{
			URL:       types.RepoURL("file://" + repo2Dir),
			BaseRef:   types.GitRef("main"),
			TargetRef: types.GitRef("main"),
		}
		if err := fetcher.Fetch(ctx, repo2, dest2); err != nil {
			t.Fatalf("Fetch() repo2 error = %v", err)
		}

		// Verify cache has two separate entries (two subdirectories under git-clones).
		cacheCloneDir := filepath.Join(cacheDir, "git-clones")
		entries, err := os.ReadDir(cacheCloneDir)
		if err != nil {
			t.Fatalf("failed to read cache directory: %v", err)
		}
		if len(entries) != 2 {
			t.Errorf("expected 2 cache entries, got %d", len(entries))
		}
	})

	t.Run("different commit_sha gets separate cache entry", func(t *testing.T) {
		t.Parallel()

		// Setup: Create a repo with multiple commits.
		repoDir := setupTestGitRepoWithCommits(t)
		secondCommitSHA := getSecondCommitSHA(t, repoDir)
		cacheDir := t.TempDir()
		dest1 := t.TempDir()
		dest2 := t.TempDir()

		fetcher, err := NewGitFetcher(GitFetcherOptions{
			CacheDir: cacheDir,
		})
		if err != nil {
			t.Fatalf("NewGitFetcher() error = %v", err)
		}

		ctx := context.Background()

		// Fetch with no commit_sha (latest commit).
		repo1 := &contracts.RepoMaterialization{
			URL:       types.RepoURL("file://" + repoDir),
			BaseRef:   types.GitRef("main"),
			TargetRef: types.GitRef("main"),
		}
		if err := fetcher.Fetch(ctx, repo1, dest1); err != nil {
			t.Fatalf("Fetch() latest commit error = %v", err)
		}

		// Fetch with specific commit_sha.
		repo2 := &contracts.RepoMaterialization{
			URL:       types.RepoURL("file://" + repoDir),
			BaseRef:   types.GitRef("main"),
			TargetRef: types.GitRef("main"),
			Commit:    types.CommitSHA(secondCommitSHA),
		}
		if err := fetcher.Fetch(ctx, repo2, dest2); err != nil {
			t.Fatalf("Fetch() specific commit error = %v", err)
		}

		// Verify cache has two separate entries.
		cacheCloneDir := filepath.Join(cacheDir, "git-clones")
		entries, err := os.ReadDir(cacheCloneDir)
		if err != nil {
			t.Fatalf("failed to read cache directory: %v", err)
		}
		if len(entries) != 2 {
			t.Errorf("expected 2 cache entries for different commits, got %d", len(entries))
		}

		// Verify dest1 has the latest commit content.
		readme1Path := filepath.Join(dest1, "README.md")
		content1, err := os.ReadFile(readme1Path)
		if err != nil {
			t.Fatalf("failed to read README.md from dest1: %v", err)
		}
		expectedContent1 := "# Third commit\n"
		if string(content1) != expectedContent1 {
			t.Errorf("dest1 expected %q, got %q", expectedContent1, string(content1))
		}

		// Verify dest2 has the second commit content.
		readme2Path := filepath.Join(dest2, "README.md")
		content2, err := os.ReadFile(readme2Path)
		if err != nil {
			t.Fatalf("failed to read README.md from dest2: %v", err)
		}
		expectedContent2 := "# Second commit\n"
		if string(content2) != expectedContent2 {
			t.Errorf("dest2 expected %q, got %q", expectedContent2, string(content2))
		}
	})

	t.Run("no cache dir disables caching", func(t *testing.T) {
		t.Parallel()

		// Setup: Create a test repo with no cache directory.
		repoDir := setupTestGitRepo(t, "no-cache")
		dest := t.TempDir()

		fetcher, err := NewGitFetcher(GitFetcherOptions{
			CacheDir: "", // Caching disabled.
		})
		if err != nil {
			t.Fatalf("NewGitFetcher() error = %v", err)
		}

		repo := &contracts.RepoMaterialization{
			URL:       types.RepoURL("file://" + repoDir),
			BaseRef:   types.GitRef("main"),
			TargetRef: types.GitRef("main"),
		}

		ctx := context.Background()
		if err := fetcher.Fetch(ctx, repo, dest); err != nil {
			t.Fatalf("Fetch() error = %v", err)
		}

		// Verify .git directory exists in dest.
		gitDir := filepath.Join(dest, ".git")
		if _, err := os.Stat(gitDir); os.IsNotExist(err) {
			t.Errorf("expected .git directory at %s", gitDir)
		}

		// No cache directory should exist since caching is disabled.
		// We don't have a way to assert this directly, but the test passes if no errors occur.
	})
}

// TestComputeCacheKey validates the cache key generation logic.
func TestComputeCacheKey(t *testing.T) {
	tests := []struct {
		name     string
		url1     string
		baseRef1 string
		commit1  string
		url2     string
		baseRef2 string
		commit2  string
		wantSame bool
	}{
		{
			name:     "same inputs produce same key",
			url1:     "https://github.com/example/repo.git",
			baseRef1: "main",
			commit1:  "abc123",
			url2:     "https://github.com/example/repo.git",
			baseRef2: "main",
			commit2:  "abc123",
			wantSame: true,
		},
		{
			name:     "URL normalization: trailing slash ignored",
			url1:     "https://github.com/example/repo.git",
			baseRef1: "main",
			commit1:  "",
			url2:     "https://github.com/example/repo.git/",
			baseRef2: "main",
			commit2:  "",
			wantSame: true,
		},
		{
			name:     "URL normalization: .git suffix ignored",
			url1:     "https://github.com/example/repo",
			baseRef1: "main",
			commit1:  "",
			url2:     "https://github.com/example/repo.git",
			baseRef2: "main",
			commit2:  "",
			wantSame: true,
		},
		{
			name:     "different base_ref produces different key",
			url1:     "https://github.com/example/repo.git",
			baseRef1: "main",
			commit1:  "",
			url2:     "https://github.com/example/repo.git",
			baseRef2: "develop",
			commit2:  "",
			wantSame: false,
		},
		{
			name:     "different commit_sha produces different key",
			url1:     "https://github.com/example/repo.git",
			baseRef1: "main",
			commit1:  "abc123",
			url2:     "https://github.com/example/repo.git",
			baseRef2: "main",
			commit2:  "def456",
			wantSame: false,
		},
		{
			name:     "different URL produces different key",
			url1:     "https://github.com/example/repo1.git",
			baseRef1: "main",
			commit1:  "",
			url2:     "https://github.com/example/repo2.git",
			baseRef2: "main",
			commit2:  "",
			wantSame: false,
		},
		{
			name:     "empty vs non-empty commit produces different key",
			url1:     "https://github.com/example/repo.git",
			baseRef1: "main",
			commit1:  "",
			url2:     "https://github.com/example/repo.git",
			baseRef2: "main",
			commit2:  "abc123",
			wantSame: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key1 := computeCacheKey(tt.url1, tt.baseRef1, tt.commit1)
			key2 := computeCacheKey(tt.url2, tt.baseRef2, tt.commit2)

			if tt.wantSame {
				if key1 != key2 {
					t.Errorf("expected same cache key, got %q and %q", key1, key2)
				}
			} else {
				if key1 == key2 {
					t.Errorf("expected different cache keys, both got %q", key1)
				}
			}

			// Verify key is filesystem-safe (hex string).
			if len(key1) != 64 { // SHA256 hex = 64 chars
				t.Errorf("expected 64-char hex string, got %d chars: %q", len(key1), key1)
			}
		})
	}
}

// TestGitFetcher_BaseHydrationStrategy validates the shallow clone base hydration strategy.
func TestGitFetcher_BaseHydrationStrategy(t *testing.T) {
	// Skip if git is not available.
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git command not found, skipping test")
	}

	t.Run("existing hydrated destination is reused without re-clone", func(t *testing.T) {
		// Setup: Create a repo and manually clone it to a destination directory.
		repoDir := setupTestGitRepo(t, "already-hydrated")
		parent := t.TempDir()
		dest := filepath.Join(parent, "clone")

		// Initial clone: this simulates a workspace that has already been hydrated
		// for the given repo URL.
		runCmd(t, parent, "git", "clone", "file://"+repoDir, "clone")

		fetcher, err := NewGitFetcher(GitFetcherOptions{})
		if err != nil {
			t.Fatalf("NewGitFetcher() error = %v", err)
		}

		repo := &contracts.RepoMaterialization{
			URL:       types.RepoURL("file://" + repoDir),
			BaseRef:   types.GitRef("main"),
			TargetRef: types.GitRef("main"),
		}

		ctx := context.Background()
		if err := fetcher.Fetch(ctx, repo, dest); err != nil {
			t.Fatalf("Fetch() on already hydrated dest error = %v", err)
		}

		// Sanity check: destination still has a .git directory.
		gitDir := filepath.Join(dest, ".git")
		if _, err := os.Stat(gitDir); os.IsNotExist(err) {
			t.Errorf("expected .git directory at %s after reuse", gitDir)
		}
	})

	t.Run("shallow clone creates minimal history", func(t *testing.T) {
		// Setup: Create a repo with multiple commits.
		repoDir := setupTestGitRepoWithCommits(t)
		dest := t.TempDir()

		fetcher, err := NewGitFetcher(GitFetcherOptions{})
		if err != nil {
			t.Fatalf("NewGitFetcher() error = %v", err)
		}

		// Fetch with shallow clone (no commit_sha, so base_ref only).
		repo := &contracts.RepoMaterialization{
			URL:       types.RepoURL("file://" + repoDir),
			BaseRef:   types.GitRef("main"),
			TargetRef: types.GitRef("main"),
		}

		ctx := context.Background()
		if err := fetcher.Fetch(ctx, repo, dest); err != nil {
			t.Fatalf("Fetch() error = %v", err)
		}

		// Verify shallow clone: should have only 1 commit in history.
		cmd := exec.Command("git", "rev-list", "--count", "HEAD")
		cmd.Dir = dest
		output, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("failed to count commits: %v (output: %s)", err, string(output))
		}

		commitCount := strings.TrimSpace(string(output))
		if commitCount != "1" {
			t.Errorf("expected 1 commit in shallow clone, got %s", commitCount)
		}
	})

	t.Run("commit_sha pins to specific commit", func(t *testing.T) {
		// Setup: Create a repo with multiple commits.
		repoDir := setupTestGitRepoWithCommits(t)
		secondCommitSHA := getSecondCommitSHA(t, repoDir)
		dest := t.TempDir()

		fetcher, err := NewGitFetcher(GitFetcherOptions{})
		if err != nil {
			t.Fatalf("NewGitFetcher() error = %v", err)
		}

		// Fetch with specific commit_sha.
		repo := &contracts.RepoMaterialization{
			URL:       types.RepoURL("file://" + repoDir),
			BaseRef:   types.GitRef("main"),
			TargetRef: types.GitRef("main"),
			Commit:    types.CommitSHA(secondCommitSHA),
		}

		ctx := context.Background()
		if err := fetcher.Fetch(ctx, repo, dest); err != nil {
			t.Fatalf("Fetch() error = %v", err)
		}

		// Verify checked out commit matches the requested SHA.
		cmd := exec.Command("git", "rev-parse", "HEAD")
		cmd.Dir = dest
		output, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("failed to get current commit: %v (output: %s)", err, string(output))
		}

		currentCommitSHA := strings.TrimSpace(string(output))
		if currentCommitSHA != secondCommitSHA {
			t.Errorf("expected commit %s, got %s", secondCommitSHA, currentCommitSHA)
		}

		// Verify the workspace content matches the second commit.
		readmePath := filepath.Join(dest, "README.md")
		content, err := os.ReadFile(readmePath)
		if err != nil {
			t.Fatalf("failed to read README.md: %v", err)
		}

		expectedContent := "# Second commit\n"
		if string(content) != expectedContent {
			t.Errorf("expected README content %q, got %q", expectedContent, string(content))
		}
	})

	t.Run("base_ref without commit_sha uses latest", func(t *testing.T) {
		// Setup: Create a repo with multiple commits.
		repoDir := setupTestGitRepoWithCommits(t)
		dest := t.TempDir()

		fetcher, err := NewGitFetcher(GitFetcherOptions{})
		if err != nil {
			t.Fatalf("NewGitFetcher() error = %v", err)
		}

		// Fetch without commit_sha (should get latest commit on base_ref).
		repo := &contracts.RepoMaterialization{
			URL:       types.RepoURL("file://" + repoDir),
			BaseRef:   types.GitRef("main"),
			TargetRef: types.GitRef("main"),
		}

		ctx := context.Background()
		if err := fetcher.Fetch(ctx, repo, dest); err != nil {
			t.Fatalf("Fetch() error = %v", err)
		}

		// Verify the workspace content matches the third (latest) commit.
		readmePath := filepath.Join(dest, "README.md")
		content, err := os.ReadFile(readmePath)
		if err != nil {
			t.Fatalf("failed to read README.md: %v", err)
		}

		expectedContent := "# Third commit\n"
		if string(content) != expectedContent {
			t.Errorf("expected README content %q, got %q", expectedContent, string(content))
		}
	})
}
