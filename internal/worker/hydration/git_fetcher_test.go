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

func TestGitFetcher_Fetch(t *testing.T) {
	// Skip if git is not available.
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git command not found, skipping test")
	}

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
				URL:       types.RepoURL("file://" + setupTestGitRepoWithCommits(t)),
				BaseRef:   types.GitRef("main"),
				TargetRef: types.GitRef("main"),
				Commit:    types.CommitSHA(getSecondCommitSHA(t, setupTestGitRepoWithCommits(t))),
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

// setupTestGitRepo creates a temporary git repository for testing.
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

// setupTestGitRepoWithCommits creates a test repository with multiple commits for testing commit_sha pinning.
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

// getSecondCommitSHA returns the SHA of the second commit in a repository.
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

// runCmd executes a command in the specified directory.
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
			name: "with PublishSnapshot enabled",
			opts: GitFetcherOptions{
				PublishSnapshot: true,
			},
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

// TestGitFetcher_BaseHydrationStrategy validates the shallow clone base hydration strategy.
func TestGitFetcher_BaseHydrationStrategy(t *testing.T) {
	// Skip if git is not available.
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git command not found, skipping test")
	}

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
