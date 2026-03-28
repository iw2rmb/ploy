package hydration

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	types "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/testutil/gitrepo"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

func TestGitFetcher_Fetch(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git command not found, skipping test")
	}

	repoWithFeatureBranch := setupRepoWithFeatureBranch(t)

	tests := []struct {
		name      string
		repo      *contracts.RepoMaterialization
		dest      string
		wantErr   bool
		errSubstr string
	}{
		{
			name:      "nil repo",
			repo:      nil,
			wantErr:   true,
			errSubstr: "repo materialization is required",
		},
		{
			name: "invalid repo URL",
			repo: &contracts.RepoMaterialization{
				URL:       types.RepoURL("invalid-url"),
				TargetRef: types.GitRef("main"),
			},
			dest:      t.TempDir(),
			wantErr:   true,
			errSubstr: "invalid repo",
		},
		{
			name: "missing target_ref and commit",
			repo: &contracts.RepoMaterialization{
				URL: types.RepoURL("https://github.com/example/repo.git"),
			},
			dest:      t.TempDir(),
			wantErr:   true,
			errSubstr: "target_ref or commit is required",
		},
		{
			name: "valid repo with different target_ref",
			repo: &contracts.RepoMaterialization{
				URL:       types.RepoURL("file://" + repoWithFeatureBranch),
				BaseRef:   types.GitRef("main"),
				TargetRef: types.GitRef("feature"),
			},
			dest: t.TempDir(),
		},
		{
			name: "valid repo with commit_sha HEAD",
			repo: &contracts.RepoMaterialization{
				URL:       types.RepoURL("file://" + gitrepo.SetupBasic(t)),
				BaseRef:   types.GitRef("main"),
				TargetRef: types.GitRef("main"),
				Commit:    types.CommitSHA("HEAD"),
			},
			dest: t.TempDir(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fetcher, err := NewGitFetcher(GitFetcherOptions{})
			if err != nil {
				t.Fatalf("NewGitFetcher() error = %v", err)
			}

			err = fetcher.Fetch(context.Background(), tt.repo, tt.dest)
			if (err != nil) != tt.wantErr {
				t.Errorf("Fetch() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.wantErr && tt.errSubstr != "" && !strings.Contains(err.Error(), tt.errSubstr) {
				t.Errorf("Fetch() error = %v, want substring %q", err, tt.errSubstr)
			}

			if !tt.wantErr {
				gitrepo.AssertRepo(t, tt.dest)
			}
		})
	}
}

func TestGitFetcher_CacheDir(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git command not found, skipping test")
	}
	if _, err := exec.LookPath("rsync"); err != nil {
		t.Skip("rsync command not found, skipping cache dir tests")
	}

	t.Run("cache miss performs fresh clone", func(t *testing.T) {
		t.Parallel()

		repoDir := gitrepo.SetupBasic(t)
		cacheDir := t.TempDir()
		dest := t.TempDir()

		fetcher, err := NewGitFetcher(GitFetcherOptions{CacheDir: cacheDir})
		if err != nil {
			t.Fatalf("NewGitFetcher() error = %v", err)
		}

		repo := makeRepo(repoDir, "main", "main", "")
		if err := fetcher.Fetch(context.Background(), repo, dest); err != nil {
			t.Fatalf("Fetch() error = %v", err)
		}

		gitrepo.AssertRepo(t, dest)

		cacheCloneDir := filepath.Join(cacheDir, "git-clones")
		if _, err := os.Stat(cacheCloneDir); os.IsNotExist(err) {
			t.Errorf("expected cache directory at %s", cacheCloneDir)
		}
	})

	t.Run("cache hit reuses cached clone", func(t *testing.T) {
		t.Parallel()

		repoDir := gitrepo.SetupBasic(t)
		cacheDir := t.TempDir()
		dest1 := t.TempDir()
		dest2 := t.TempDir()

		fetcher, err := NewGitFetcher(GitFetcherOptions{CacheDir: cacheDir})
		if err != nil {
			t.Fatalf("NewGitFetcher() error = %v", err)
		}

		repo := makeRepo(repoDir, "main", "main", "")
		ctx := context.Background()

		if err := fetcher.Fetch(ctx, repo, dest1); err != nil {
			t.Fatalf("Fetch() first call error = %v", err)
		}
		if err := fetcher.Fetch(ctx, repo, dest2); err != nil {
			t.Fatalf("Fetch() second call error = %v", err)
		}

		for _, dest := range []string{dest1, dest2} {
			gitrepo.AssertRepo(t, dest)
		}

		readme1 := filepath.Join(dest1, "README.md")
		readme2 := filepath.Join(dest2, "README.md")
		content1, err := os.ReadFile(readme1)
		if err != nil {
			t.Fatalf("failed to read README.md from dest1: %v", err)
		}
		content2, err := os.ReadFile(readme2)
		if err != nil {
			t.Fatalf("failed to read README.md from dest2: %v", err)
		}
		if string(content1) != string(content2) {
			t.Errorf("cached clone content mismatch: %q vs %q", string(content1), string(content2))
		}
	})

	t.Run("different repos get separate cache entries", func(t *testing.T) {
		t.Parallel()

		repo1Dir := gitrepo.SetupBasic(t)
		repo2Dir := gitrepo.SetupBasic(t)
		cacheDir := t.TempDir()
		dest1 := t.TempDir()
		dest2 := t.TempDir()

		fetcher, err := NewGitFetcher(GitFetcherOptions{CacheDir: cacheDir})
		if err != nil {
			t.Fatalf("NewGitFetcher() error = %v", err)
		}

		ctx := context.Background()
		if err := fetcher.Fetch(ctx, makeRepo(repo1Dir, "main", "main", ""), dest1); err != nil {
			t.Fatalf("Fetch() repo1 error = %v", err)
		}
		if err := fetcher.Fetch(ctx, makeRepo(repo2Dir, "main", "main", ""), dest2); err != nil {
			t.Fatalf("Fetch() repo2 error = %v", err)
		}

		assertCacheEntryCount(t, cacheDir, 2)
	})

	t.Run("different commit_sha gets separate cache entry", func(t *testing.T) {
		t.Parallel()

		repoDir := setupRepoWithCommits(t)
		secondCommitSHA := gitrepo.RevParse(t, repoDir, "HEAD~1")
		cacheDir := t.TempDir()
		dest1 := t.TempDir()
		dest2 := t.TempDir()

		fetcher, err := NewGitFetcher(GitFetcherOptions{CacheDir: cacheDir})
		if err != nil {
			t.Fatalf("NewGitFetcher() error = %v", err)
		}

		ctx := context.Background()
		if err := fetcher.Fetch(ctx, makeRepo(repoDir, "main", "main", ""), dest1); err != nil {
			t.Fatalf("Fetch() latest commit error = %v", err)
		}
		if err := fetcher.Fetch(ctx, makeRepo(repoDir, "main", "main", secondCommitSHA), dest2); err != nil {
			t.Fatalf("Fetch() specific commit error = %v", err)
		}

		assertCacheEntryCount(t, cacheDir, 2)
		gitrepo.AssertFileContent(t, filepath.Join(dest1, "README.md"), "# Third commit\n")
		gitrepo.AssertFileContent(t, filepath.Join(dest2, "README.md"), "# Second commit\n")
	})

	t.Run("no cache dir disables caching", func(t *testing.T) {
		t.Parallel()

		repoDir := gitrepo.SetupBasic(t)
		dest := t.TempDir()

		fetcher, err := NewGitFetcher(GitFetcherOptions{})
		if err != nil {
			t.Fatalf("NewGitFetcher() error = %v", err)
		}

		if err := fetcher.Fetch(context.Background(), makeRepo(repoDir, "main", "main", ""), dest); err != nil {
			t.Fatalf("Fetch() error = %v", err)
		}

		gitrepo.AssertRepo(t, dest)
	})
}

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
			url2:     "https://github.com/example/repo.git/",
			baseRef2: "main",
			wantSame: true,
		},
		{
			name:     "URL normalization: .git suffix ignored",
			url1:     "https://github.com/example/repo",
			baseRef1: "main",
			url2:     "https://github.com/example/repo.git",
			baseRef2: "main",
			wantSame: true,
		},
		{
			name:     "different base_ref produces different key",
			url1:     "https://github.com/example/repo.git",
			baseRef1: "main",
			url2:     "https://github.com/example/repo.git",
			baseRef2: "develop",
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
			url2:     "https://github.com/example/repo2.git",
			baseRef2: "main",
			wantSame: false,
		},
		{
			name:     "empty vs non-empty commit produces different key",
			url1:     "https://github.com/example/repo.git",
			baseRef1: "main",
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

			if tt.wantSame && key1 != key2 {
				t.Errorf("expected same cache key, got %q and %q", key1, key2)
			}
			if !tt.wantSame && key1 == key2 {
				t.Errorf("expected different cache keys, both got %q", key1)
			}
			if len(key1) != 64 {
				t.Errorf("expected 64-char hex string, got %d chars: %q", len(key1), key1)
			}
		})
	}
}

func TestGitFetcher_BaseHydrationStrategy(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git command not found, skipping test")
	}

	t.Run("existing hydrated destination is reused without re-clone", func(t *testing.T) {
		repoDir := gitrepo.SetupBasic(t)
		parent := t.TempDir()
		dest := filepath.Join(parent, "clone")

		gitrepo.Run(t, parent, "clone", "file://"+repoDir, "clone")

		fetcher, err := NewGitFetcher(GitFetcherOptions{})
		if err != nil {
			t.Fatalf("NewGitFetcher() error = %v", err)
		}

		if err := fetcher.Fetch(context.Background(), makeRepo(repoDir, "main", "main", ""), dest); err != nil {
			t.Fatalf("Fetch() on already hydrated dest error = %v", err)
		}

		gitrepo.AssertRepo(t, dest)
	})

	t.Run("shallow clone creates minimal history", func(t *testing.T) {
		repoDir := setupRepoWithCommits(t)
		dest := t.TempDir()

		fetcher, err := NewGitFetcher(GitFetcherOptions{})
		if err != nil {
			t.Fatalf("NewGitFetcher() error = %v", err)
		}

		if err := fetcher.Fetch(context.Background(), makeRepo(repoDir, "main", "main", ""), dest); err != nil {
			t.Fatalf("Fetch() error = %v", err)
		}

		output := gitrepo.Run(t, dest, "rev-list", "--count", "HEAD")
		if commitCount := strings.TrimSpace(string(output)); commitCount != "1" {
			t.Errorf("expected 1 commit in shallow clone, got %s", commitCount)
		}
	})

	t.Run("commit_sha pins to specific commit", func(t *testing.T) {
		repoDir := setupRepoWithCommits(t)
		secondCommitSHA := gitrepo.RevParse(t, repoDir, "HEAD~1")
		dest := t.TempDir()

		fetcher, err := NewGitFetcher(GitFetcherOptions{})
		if err != nil {
			t.Fatalf("NewGitFetcher() error = %v", err)
		}

		if err := fetcher.Fetch(context.Background(), makeRepo(repoDir, "main", "main", secondCommitSHA), dest); err != nil {
			t.Fatalf("Fetch() error = %v", err)
		}

		currentSHA := gitrepo.RevParse(t, dest, "HEAD")
		if currentSHA != secondCommitSHA {
			t.Errorf("expected commit %s, got %s", secondCommitSHA, currentSHA)
		}
		gitrepo.AssertFileContent(t, filepath.Join(dest, "README.md"), "# Second commit\n")
	})

	t.Run("base_ref without commit_sha uses latest", func(t *testing.T) {
		repoDir := setupRepoWithCommits(t)
		dest := t.TempDir()

		fetcher, err := NewGitFetcher(GitFetcherOptions{})
		if err != nil {
			t.Fatalf("NewGitFetcher() error = %v", err)
		}

		if err := fetcher.Fetch(context.Background(), makeRepo(repoDir, "main", "main", ""), dest); err != nil {
			t.Fatalf("Fetch() error = %v", err)
		}

		gitrepo.AssertFileContent(t, filepath.Join(dest, "README.md"), "# Third commit\n")
	})
}

// --- helpers ---

// setupRepoWithFeatureBranch creates a repo with main + feature branch.
func setupRepoWithFeatureBranch(t *testing.T) string {
	t.Helper()
	repoDir := gitrepo.SetupBasic(t)
	gitrepo.Run(t, repoDir, "checkout", "-b", "feature")
	gitrepo.WriteFile(t, filepath.Join(repoDir, "feature.txt"), "feature content\n")
	gitrepo.CommitAll(t, repoDir, "Add feature")
	gitrepo.Run(t, repoDir, "checkout", "main")
	return repoDir
}

// setupRepoWithCommits creates a repo with three commits on main.
func setupRepoWithCommits(t *testing.T) string {
	t.Helper()
	repoDir := t.TempDir()
	gitrepo.InitMainBranch(t, repoDir)
	readme := filepath.Join(repoDir, "README.md")
	for _, msg := range []string{"First commit", "Second commit", "Third commit"} {
		gitrepo.WriteFile(t, readme, "# "+msg+"\n")
		gitrepo.CommitAll(t, repoDir, msg)
	}
	return repoDir
}

func makeRepo(dir, baseRef, targetRef, commit string) *contracts.RepoMaterialization {
	r := &contracts.RepoMaterialization{
		URL:       types.RepoURL("file://" + dir),
		BaseRef:   types.GitRef(baseRef),
		TargetRef: types.GitRef(targetRef),
	}
	if commit != "" {
		r.Commit = types.CommitSHA(commit)
	}
	return r
}

func assertCacheEntryCount(t *testing.T, cacheDir string, want int) {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join(cacheDir, "git-clones"))
	if err != nil {
		t.Fatalf("failed to read cache directory: %v", err)
	}
	if len(entries) != want {
		t.Errorf("expected %d cache entries, got %d", want, len(entries))
	}
}
