package hydration

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

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
				URL:       "invalid-url",
				TargetRef: "main",
			},
			setup:     func(t *testing.T) string { return t.TempDir() },
			wantErr:   true,
			errSubstr: "invalid repo",
		},
		{
			name: "missing target_ref and commit",
			repo: &contracts.RepoMaterialization{
				URL: "https://github.com/example/repo.git",
			},
			setup:     func(t *testing.T) string { return t.TempDir() },
			wantErr:   true,
			errSubstr: "target_ref or commit is required",
		},
		{
			name: "valid repo with base_ref only",
			repo: &contracts.RepoMaterialization{
				URL:       "file://" + setupTestGitRepo(t, "base"),
				BaseRef:   "main",
				TargetRef: "main",
			},
			setup:   func(t *testing.T) string { return t.TempDir() },
			wantErr: false,
		},
		{
			name: "valid repo with different target_ref",
			repo: &contracts.RepoMaterialization{
				URL:       "file://" + setupTestGitRepo(t, "target"),
				BaseRef:   "main",
				TargetRef: "feature",
			},
			setup:   func(t *testing.T) string { return t.TempDir() },
			wantErr: false,
		},
		{
			name: "valid repo with commit_sha",
			repo: &contracts.RepoMaterialization{
				URL:       "file://" + setupTestGitRepo(t, "commit"),
				BaseRef:   "main",
				TargetRef: "main",
				Commit:    "HEAD",
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
			name: "with publisher",
			opts: GitFetcherOptions{
				Publisher:       &struct{}{},
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
