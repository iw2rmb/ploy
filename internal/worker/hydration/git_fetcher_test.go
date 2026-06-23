package hydration

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	types "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/gitauth"
	"github.com/iw2rmb/ploy/internal/testutil/gitrepo"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

func TestGitFetcher_Fetch(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git command not found, skipping test")
	}

	repoWithFeatureBranch := setupRepoWithFeatureBranch(t)
	repoWithCommits := setupRepoWithCommits(t)
	secondCommitSHA := gitrepo.RevParse(t, repoWithCommits, "HEAD~1")

	tests := []struct {
		name      string
		repo      *contracts.RepoMaterialization
		dest      string
		wantErr   bool
		errSubstr string
		wantHead  string
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
				URL: types.RepoURL("invalid-url"),
			},
			dest:      t.TempDir(),
			wantErr:   true,
			errSubstr: "invalid repo",
		},
		{
			name: "missing base_ref and commit",
			repo: &contracts.RepoMaterialization{
				URL: types.RepoURL("https://github.com/example/repo.git"),
			},
			dest:      t.TempDir(),
			wantErr:   true,
			errSubstr: "base_ref or commit is required",
		},
		{
			name: "valid repo with base_ref",
			repo: &contracts.RepoMaterialization{
				URL:     types.RepoURL("file://" + repoWithFeatureBranch),
				BaseRef: types.GitRef("main"),
			},
			dest: t.TempDir(),
		},
		{
			name: "valid repo with base_ref commit sha",
			repo: &contracts.RepoMaterialization{
				URL:     types.RepoURL("file://" + repoWithCommits),
				BaseRef: types.GitRef(secondCommitSHA),
			},
			dest:     t.TempDir(),
			wantHead: secondCommitSHA,
		},
		{
			name: "valid repo with commit_sha HEAD",
			repo: &contracts.RepoMaterialization{
				URL:     types.RepoURL("file://" + gitrepo.SetupBasic(t)),
				BaseRef: types.GitRef("main"),
				Commit:  types.CommitSHA("HEAD"),
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

			err = fetcher.Fetch(context.Background(), tt.repo, tt.dest, gitauth.Options{})
			if (err != nil) != tt.wantErr {
				t.Errorf("Fetch() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.wantErr && tt.errSubstr != "" && !strings.Contains(err.Error(), tt.errSubstr) {
				t.Errorf("Fetch() error = %v, want substring %q", err, tt.errSubstr)
			}

			if !tt.wantErr {
				gitrepo.AssertRepo(t, tt.dest)
				if tt.wantHead != "" {
					got := gitrepo.RevParse(t, tt.dest, "HEAD")
					if got != tt.wantHead {
						t.Fatalf("HEAD = %s, want %s", got, tt.wantHead)
					}
				}
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

	t.Run("cache hit sanitizes credentialed origin urls", func(t *testing.T) {
		t.Parallel()

		repoDir := gitrepo.SetupBasic(t)
		cacheDir := t.TempDir()
		dest := t.TempDir()
		cleanURL := "https://gitlab.example.com/group/repo.git"
		credentialedURL := "https://oauth2:glpat-secret@gitlab.example.com/group/repo.git"
		commitSHA := gitrepo.RevParse(t, repoDir, "HEAD")
		cachedClone := mustCacheClonePath(t, cacheDir, cleanURL, commitSHA)

		if err := os.MkdirAll(filepath.Dir(cachedClone), 0o750); err != nil {
			t.Fatalf("create cache parent: %v", err)
		}
		gitrepo.Run(t, cacheDir, "clone", "file://"+repoDir, cachedClone)
		gitrepo.Run(t, cachedClone, "remote", "set-url", "origin", credentialedURL)

		fetcher, err := NewGitFetcher(GitFetcherOptions{CacheDir: cacheDir})
		if err != nil {
			t.Fatalf("NewGitFetcher() error = %v", err)
		}
		repo := &contracts.RepoMaterialization{
			URL:     types.RepoURL(cleanURL),
			BaseRef: types.GitRef("main"),
			Commit:  types.CommitSHA(commitSHA),
		}

		if err := fetcher.Fetch(context.Background(), repo, dest, gitauth.Options{
			GitLabPAT:    "glpat-secret",
			GitLabDomain: "gitlab.example.com",
		}); err != nil {
			t.Fatalf("Fetch() error = %v", err)
		}

		assertRemoteOriginURL(t, cachedClone, cleanURL)
		assertRemoteOriginURL(t, dest, cleanURL)
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
		if err := fetcher.Fetch(ctx, makeRepo(repo1Dir, "main", gitrepo.RevParse(t, repo1Dir, "HEAD")), dest1, gitauth.Options{}); err != nil {
			t.Fatalf("Fetch() repo1 error = %v", err)
		}
		if err := fetcher.Fetch(ctx, makeRepo(repo2Dir, "main", gitrepo.RevParse(t, repo2Dir, "HEAD")), dest2, gitauth.Options{}); err != nil {
			t.Fatalf("Fetch() repo2 error = %v", err)
		}

		assertCacheEntryCount(t, cacheDir, 2)
	})

	t.Run("same repo and commit with different base_ref reuses cache entry", func(t *testing.T) {
		t.Parallel()

		repoDir := setupRepoWithFeatureBranch(t)
		commitSHA := gitrepo.RevParse(t, repoDir, "main")
		cacheDir := t.TempDir()
		dest1 := t.TempDir()
		dest2 := t.TempDir()

		fetcher, err := NewGitFetcher(GitFetcherOptions{CacheDir: cacheDir})
		if err != nil {
			t.Fatalf("NewGitFetcher() error = %v", err)
		}

		ctx := context.Background()
		if err := fetcher.Fetch(ctx, makeRepo(repoDir, "main", commitSHA), dest1, gitauth.Options{}); err != nil {
			t.Fatalf("Fetch() main error = %v", err)
		}
		if err := fetcher.Fetch(ctx, makeRepo(repoDir, "feature", commitSHA), dest2, gitauth.Options{}); err != nil {
			t.Fatalf("Fetch() feature error = %v", err)
		}

		assertCacheEntryCount(t, cacheDir, 1)
	})

	t.Run("different commit_sha gets separate cache entry", func(t *testing.T) {
		t.Parallel()

		repoDir := setupRepoWithCommits(t)
		latestCommitSHA := gitrepo.RevParse(t, repoDir, "HEAD")
		secondCommitSHA := gitrepo.RevParse(t, repoDir, "HEAD~1")
		cacheDir := t.TempDir()
		dest1 := t.TempDir()
		dest2 := t.TempDir()

		fetcher, err := NewGitFetcher(GitFetcherOptions{CacheDir: cacheDir})
		if err != nil {
			t.Fatalf("NewGitFetcher() error = %v", err)
		}

		ctx := context.Background()
		if err := fetcher.Fetch(ctx, makeRepo(repoDir, "main", latestCommitSHA), dest1, gitauth.Options{}); err != nil {
			t.Fatalf("Fetch() latest commit error = %v", err)
		}
		if err := fetcher.Fetch(ctx, makeRepo(repoDir, "main", secondCommitSHA), dest2, gitauth.Options{}); err != nil {
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

		if err := fetcher.Fetch(context.Background(), makeRepo(repoDir, "main", ""), dest, gitauth.Options{}); err != nil {
			t.Fatalf("Fetch() error = %v", err)
		}

		gitrepo.AssertRepo(t, dest)
	})
}

func TestCacheClonePath(t *testing.T) {
	sha := "0123456789abcdef0123456789abcdef01234567"
	tests := []struct {
		name    string
		url     string
		commit  string
		want    string
		wantErr bool
	}{
		{
			name:   "https nested namespace",
			url:    "https://github.com/org/team/repo.git",
			commit: sha,
			want:   filepath.Join("cache", "git-clones", "github.com", "org", "team", "repo", sha),
		},
		{
			name:   "credentials stripped",
			url:    "https://oauth2:glpat-secret@gitlab.example.com/group/repo.git/",
			commit: strings.ToUpper(sha),
			want:   filepath.Join("cache", "git-clones", "gitlab.example.com", "group", "repo", sha),
		},
		{
			name:   "ssh scheme strips user",
			url:    "ssh://git@gitlab.example.com/group/repo.git",
			commit: sha,
			want:   filepath.Join("cache", "git-clones", "gitlab.example.com", "group", "repo", sha),
		},
		{
			name:   "file scheme",
			url:    "file:///tmp/group/repo.git",
			commit: sha,
			want:   filepath.Join("cache", "git-clones", "_file", "tmp", "group", "repo", sha),
		},
		{
			name:    "short commit rejected",
			url:     "https://github.com/org/repo.git",
			commit:  "01234567",
			wantErr: true,
		},
		{
			name:    "path traversal rejected",
			url:     "https://github.com/org/../repo.git",
			commit:  sha,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := cacheClonePath("cache", tt.url, tt.commit)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("cacheClonePath() error = nil, want error")
				}
				return
			}
			if err != nil {
				t.Fatalf("cacheClonePath() error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("cacheClonePath() = %q, want %q", got, tt.want)
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

		if err := fetcher.Fetch(context.Background(), makeRepo(repoDir, "main", ""), dest, gitauth.Options{}); err != nil {
			t.Fatalf("Fetch() on already hydrated dest error = %v", err)
		}

		gitrepo.AssertRepo(t, dest)
	})

	t.Run("existing hydrated destination with wrong commit is rebuilt", func(t *testing.T) {
		repoDir := setupRepoWithCommits(t)
		secondCommitSHA := gitrepo.RevParse(t, repoDir, "HEAD~1")
		parent := t.TempDir()
		dest := filepath.Join(parent, "clone")

		gitrepo.Run(t, parent, "clone", "file://"+repoDir, "clone")

		fetcher, err := NewGitFetcher(GitFetcherOptions{})
		if err != nil {
			t.Fatalf("NewGitFetcher() error = %v", err)
		}

		if err := fetcher.Fetch(context.Background(), makeRepo(repoDir, "main", secondCommitSHA), dest, gitauth.Options{}); err != nil {
			t.Fatalf("Fetch() error = %v", err)
		}

		currentSHA := gitrepo.RevParse(t, dest, "HEAD")
		if currentSHA != secondCommitSHA {
			t.Errorf("expected commit %s, got %s", secondCommitSHA, currentSHA)
		}
	})

	t.Run("shallow clone creates minimal history", func(t *testing.T) {
		repoDir := setupRepoWithCommits(t)
		dest := t.TempDir()

		fetcher, err := NewGitFetcher(GitFetcherOptions{})
		if err != nil {
			t.Fatalf("NewGitFetcher() error = %v", err)
		}

		if err := fetcher.Fetch(context.Background(), makeRepo(repoDir, "main", ""), dest, gitauth.Options{}); err != nil {
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

		if err := fetcher.Fetch(context.Background(), makeRepo(repoDir, "main", secondCommitSHA), dest, gitauth.Options{}); err != nil {
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

		if err := fetcher.Fetch(context.Background(), makeRepo(repoDir, "main", ""), dest, gitauth.Options{}); err != nil {
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

func makeRepo(dir, baseRef, commit string) *contracts.RepoMaterialization {
	r := &contracts.RepoMaterialization{
		URL:     types.RepoURL("file://" + dir),
		BaseRef: types.GitRef(baseRef),
	}
	if commit != "" {
		r.Commit = types.CommitSHA(commit)
	}
	return r
}

func assertCacheEntryCount(t *testing.T, cacheDir string, want int) {
	t.Helper()
	got := 0
	err := filepath.WalkDir(filepath.Join(cacheDir, "git-clones"), func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			return nil
		}
		if d.Name() == ".git" {
			got++
			return filepath.SkipDir
		}
		return nil
	})
	if err != nil {
		t.Fatalf("failed to walk cache directory: %v", err)
	}
	if got != want {
		t.Errorf("expected %d cache entries, got %d", want, got)
	}
}

func mustCacheClonePath(t *testing.T, cacheDir, repoURL, commitSHA string) string {
	t.Helper()
	path, err := cacheClonePath(cacheDir, repoURL, commitSHA)
	if err != nil {
		t.Fatalf("cacheClonePath() error = %v", err)
	}
	return path
}

func assertRemoteOriginURL(t *testing.T, repoDir, want string) {
	t.Helper()
	got := strings.TrimSpace(string(gitrepo.Run(t, repoDir, "remote", "get-url", "origin")))
	if got != want {
		t.Fatalf("origin URL=%q, want %q", got, want)
	}
}
