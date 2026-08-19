package speccatalog

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/gitexec"
)

func TestServiceList_ClonesDiscoversAndRefreshesDefaultBranch(t *testing.T) {
	remote := newSpecRemote(t, map[string]string{
		"scenarios/upgrade.yaml": "apiVersion: ploy.mig/v1alpha1\nname: upgrade-java\ndescription: Upgrade Java\n",
		"nested.yaml":            "wrapper:\n  apiVersion: ploy.mig/v1alpha1\n  name: nested-only\n",
		"sequence.yaml":          "- apiVersion: ploy.mig/v1alpha1\n- name: sequence\n",
	})
	service := newTestService(t, remote.url)

	first, err := service.List(context.Background())
	if err != nil {
		t.Fatalf("List() initial error = %v", err)
	}
	if len(first) != 1 || first[0].Name != "upgrade-java" || first[0].Description != "Upgrade Java" || first[0].Path != "scenarios/upgrade.yaml" {
		t.Fatalf("List() initial = %#v, want one discovered root mapping", first)
	}
	if !domaintypes.IsCanonicalFullCommitSHA(first[0].SHA) || first[0].CommittedAt.IsZero() || first[0].Source != strings.TrimSuffix(remote.url, ".git") {
		t.Fatalf("List() source identity = %#v", first[0])
	}

	writeTestFile(t, filepath.Join(service.repositories[0].checkout, "untracked.yaml"), "apiVersion: ploy.mig/v1alpha1\nname: untracked\n")
	remote.commit(t, "stable", map[string]*string{
		"scenarios/upgrade.yaml": nil,
		"scenarios/current.yaml": stringPointer("apiVersion: ploy.mig/v1alpha1\nname: current-java\ndescription: Current Java\n"),
	})
	remote.setDefaultBranch(t, "stable")

	second, err := service.List(context.Background())
	if err != nil {
		t.Fatalf("List() refreshed error = %v", err)
	}
	if len(second) != 1 || second[0].Name != "current-java" || second[0].Path != "scenarios/current.yaml" {
		t.Fatalf("List() refreshed = %#v, want changed default branch contents", second)
	}
	if second[0].SHA == first[0].SHA {
		t.Fatalf("List() SHA = %q, want changed commit", second[0].SHA)
	}
	if err := service.WithResolvedSource(context.Background(), "current-java", func(entry Entry, root string) error {
		content, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(entry.Path)))
		if err != nil {
			return err
		}
		if !strings.Contains(string(content), "name: current-java") || entry.SHA != second[0].SHA {
			return fmt.Errorf("resolved source = %q at %s", content, entry.SHA)
		}
		return nil
	}); err != nil {
		t.Fatalf("WithResolvedSource() error = %v", err)
	}
}

func TestServiceList_SortsAcrossRepositoriesAndResolvesSelectors(t *testing.T) {
	firstRemote := newSpecRemote(t, map[string]string{
		"z.yaml": "apiVersion: ploy.mig/v1alpha1\nname: duplicate\n",
		"a.yaml": "apiVersion: ploy.mig/v1alpha1\nname: alpha\n",
	})
	secondRemote := newSpecRemote(t, map[string]string{
		"nested/b.yaml": "apiVersion: ploy.mig/v1alpha1\nname: duplicate\n",
	})
	service, err := New(Options{
		Repositories: []domaintypes.RepoURL{domaintypes.RepoURL(secondRemote.url), domaintypes.RepoURL(firstRemote.url)},
		CacheDir:     t.TempDir(),
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	entries, err := service.List(context.Background())
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(entries) != 3 || entries[0].Name != "alpha" || entries[1].Name != "duplicate" || entries[2].Name != "duplicate" || entries[1].Source > entries[2].Source {
		t.Fatalf("List() = %#v, want name/source/path order", entries)
	}

	if _, err := service.Resolve(context.Background(), "duplicate"); err == nil {
		t.Fatal("Resolve(duplicate) succeeded, want ambiguity")
	} else {
		var ambiguous *AmbiguousError
		if !errors.As(err, &ambiguous) || len(ambiguous.Choices) != 2 {
			t.Fatalf("Resolve(duplicate) error = %v, want two-choice ambiguity", err)
		}
	}
	fullQualifier := strings.Trim(domaintypes.NormalizeRepoURLSchemless(entries[1].Source), "/") + ":duplicate"
	if resolved, err := service.Resolve(context.Background(), fullQualifier); err != nil || resolved.Source != entries[1].Source {
		t.Fatalf("Resolve(%q) = %#v, %v; want source %q", fullQualifier, resolved, err, entries[1].Source)
	}

	selectorEntries := []Entry{
		{Name: "duplicate", Source: "https://git.one.example/acme/scenarios", Path: "one.yaml"},
		{Name: "duplicate", Source: "https://git.two.example/team/scenarios", Path: "two.yaml"},
	}
	qualified := "git.one.example/acme/scenarios:duplicate"
	resolved, err := resolveEntries(selectorEntries, qualified)
	if err != nil {
		t.Fatalf("resolveEntries(%q) error = %v", qualified, err)
	}
	if resolved.Source != selectorEntries[0].Source {
		t.Fatalf("resolveEntries(%q) = %#v, want source %q", qualified, resolved, selectorEntries[0].Source)
	}

	repoQualified := "acme/scenarios:duplicate"
	if _, err := resolveEntries(selectorEntries, repoQualified); err != nil {
		t.Fatalf("resolveEntries(%q) error = %v", repoQualified, err)
	}
}

func TestServiceList_EmptyAndRefreshFailureDoNotUseStaleCache(t *testing.T) {
	empty, err := New(Options{})
	if err != nil {
		t.Fatalf("New(empty) error = %v", err)
	}
	if _, err := empty.List(context.Background()); !errors.Is(err, ErrNoRepositories) {
		t.Fatalf("List(empty) error = %v, want ErrNoRepositories", err)
	}

	remote := newSpecRemote(t, map[string]string{
		"spec.yaml": "apiVersion: ploy.mig/v1alpha1\nname: available\n",
	})
	service := newTestService(t, remote.url)
	if _, err := service.List(context.Background()); err != nil {
		t.Fatalf("List() initial error = %v", err)
	}
	if err := os.Rename(remote.bare, remote.bare+".unavailable"); err != nil {
		t.Fatalf("make remote unavailable: %v", err)
	}
	if entries, err := service.List(context.Background()); err == nil || entries != nil || !strings.Contains(err.Error(), service.repositories[0].source) {
		t.Fatalf("List() after remote failure = %#v, %v; want no stale entries and credential-free source", entries, err)
	}
}

func TestService_CleansCredentialBearingRepositoryURL(t *testing.T) {
	service, err := New(Options{
		Repositories: []domaintypes.RepoURL{"https://user:secret@git.example.com/acme/scenarios.git"},
		CacheDir:     t.TempDir(),
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if strings.Contains(service.repositories[0].source, "user") || strings.Contains(service.repositories[0].source, "secret") {
		t.Fatalf("catalog source %q contains credentials", service.repositories[0].source)
	}
	runner := &captureFailRunner{}
	service.repositories[0].runner = runner
	_, err = service.List(context.Background())
	if err == nil {
		t.Fatal("List() succeeded, want clone failure")
	}
	if strings.Contains(err.Error(), "user") || strings.Contains(err.Error(), "secret") || strings.Contains(strings.Join(runner.args, " "), "secret") {
		t.Fatalf("clone failure exposed credentials: error=%q args=%q", err, runner.args)
	}
	if got := strings.Join(runner.args, " "); !strings.Contains(got, "https://git.example.com/acme/scenarios.git") {
		t.Fatalf("git args = %q, want credential-free clone URL", got)
	}
}

func TestNew_DefaultsCacheForConfiguredRepositories(t *testing.T) {
	service, err := New(Options{Repositories: []domaintypes.RepoURL{"https://git.example.com/acme/scenarios.git"}})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if !strings.HasPrefix(service.repositories[0].checkout, filepath.Clean(os.TempDir())+string(filepath.Separator)) {
		t.Fatalf("checkout = %q, want beneath temporary cache root", service.repositories[0].checkout)
	}
}

func TestRepositoryRefresh_ConcurrentCallersShareGitMutation(t *testing.T) {
	remote := newSpecRemote(t, map[string]string{
		"spec.yaml": "apiVersion: ploy.mig/v1alpha1\nname: shared\n",
	})
	service := newTestService(t, remote.url)
	if _, err := service.List(context.Background()); err != nil {
		t.Fatalf("List() initial error = %v", err)
	}

	runner := &blockingFetchRunner{
		delegate: gitexec.ExecRunner{},
		started:  make(chan struct{}),
		release:  make(chan struct{}),
	}
	service.repositories[0].runner = runner

	const callers = 8
	errs := make(chan error, callers)
	go func() {
		_, err := service.List(context.Background())
		errs <- err
	}()
	<-runner.started
	for range callers - 1 {
		go func() {
			_, err := service.List(context.Background())
			errs <- err
		}()
	}
	runtime.Gosched()
	time.Sleep(25 * time.Millisecond)
	close(runner.release)
	for range callers {
		if err := <-errs; err != nil {
			t.Fatalf("List() concurrent error = %v", err)
		}
	}
	if runner.fetches != 1 {
		t.Fatalf("git fetch calls = %d, want one shared refresh", runner.fetches)
	}
}

func TestResolveEntries_RejectsUnsupportedSelectors(t *testing.T) {
	entries := []Entry{{Name: "upgrade-java", Source: "https://git.example.com/acme/scenarios", Path: "upgrade.yaml"}}
	for _, selector := range []string{"", "Upgrade", "upgrade-java@01234567", "acme:upgrade-java", "git.example.com//scenarios:upgrade-java"} {
		if _, err := resolveEntries(entries, selector); err == nil {
			t.Fatalf("resolveEntries(%q) succeeded, want invalid selector", selector)
		}
	}
	if _, err := resolveEntries(entries, "missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("resolveEntries(missing) error = %v, want ErrNotFound", err)
	}
}

type blockingFetchRunner struct {
	delegate gitexec.Runner
	started  chan struct{}
	release  chan struct{}
	once     sync.Once
	fetches  int
	mu       sync.Mutex
}

type captureFailRunner struct {
	args []string
}

func (r *captureFailRunner) Run(_ context.Context, req gitexec.Request) (gitexec.Result, error) {
	r.args = append([]string(nil), req.Args...)
	return gitexec.Result{}, errors.New("remote unavailable")
}

func (r *blockingFetchRunner) Run(ctx context.Context, req gitexec.Request) (gitexec.Result, error) {
	if len(req.Args) > 0 && req.Args[0] == "fetch" {
		r.mu.Lock()
		r.fetches++
		r.mu.Unlock()
		r.once.Do(func() { close(r.started) })
		select {
		case <-ctx.Done():
			return gitexec.Result{}, ctx.Err()
		case <-r.release:
		}
	}
	return r.delegate.Run(ctx, req)
}

type specRemote struct {
	work string
	bare string
	url  string
}

func newSpecRemote(t *testing.T, files map[string]string) *specRemote {
	t.Helper()
	root := t.TempDir()
	work := filepath.Join(root, "work")
	bare := filepath.Join(root, "group", "scenarios.git")
	if err := os.MkdirAll(work, 0o755); err != nil {
		t.Fatalf("create worktree: %v", err)
	}
	runTestGit(t, work, "init", "-b", "main")
	runTestGit(t, work, "config", "user.email", "spec@example.test")
	runTestGit(t, work, "config", "user.name", "Spec Tester")
	for path, content := range files {
		writeTestFile(t, filepath.Join(work, path), content)
	}
	runTestGit(t, work, "add", ".")
	runTestGit(t, work, "commit", "-m", "initial specs")
	if err := os.MkdirAll(filepath.Dir(bare), 0o755); err != nil {
		t.Fatalf("create bare parent: %v", err)
	}
	runTestGit(t, root, "clone", "--bare", work, bare)
	runTestGit(t, work, "remote", "add", "origin", (&url.URL{Scheme: "file", Path: bare}).String())
	return &specRemote{work: work, bare: bare, url: (&url.URL{Scheme: "file", Path: bare}).String()}
}

func (r *specRemote) commit(t *testing.T, branch string, changes map[string]*string) {
	t.Helper()
	runTestGit(t, r.work, "checkout", "-b", branch)
	for path, content := range changes {
		path = filepath.Join(r.work, path)
		if content == nil {
			if err := os.Remove(path); err != nil {
				t.Fatalf("remove %s: %v", path, err)
			}
			continue
		}
		writeTestFile(t, path, *content)
	}
	runTestGit(t, r.work, "add", "-A")
	runTestGit(t, r.work, "commit", "-m", "update specs")
	runTestGit(t, r.work, "push", "origin", branch)
}

func (r *specRemote) setDefaultBranch(t *testing.T, branch string) {
	t.Helper()
	runTestGit(t, "", "--git-dir", r.bare, "symbolic-ref", "HEAD", "refs/heads/"+branch)
}

func newTestService(t *testing.T, repositoryURL string) *Service {
	t.Helper()
	service, err := New(Options{
		Repositories: []domaintypes.RepoURL{domaintypes.RepoURL(repositoryURL)},
		CacheDir:     t.TempDir(),
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	return service
}

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create parent for %s: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func runTestGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	result, err := gitexec.Execute(t.Context(), gitexec.Request{Dir: dir, Env: []string{"GIT_CONFIG_NOSYSTEM=1"}, Args: args})
	if err != nil {
		output := append(result.Stdout, result.Stderr...)
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, output)
	}
}

func stringPointer(value string) *string {
	return &value
}

func ExampleAmbiguousError() {
	err := (&AmbiguousError{
		Selector: "upgrade",
		Choices: []Entry{
			{Source: "https://git.example.com/acme/one", Path: "one.yaml"},
			{Source: "https://git.example.com/acme/two", Path: "two.yaml"},
		},
	}).Error()
	fmt.Println(err)
	// Output: named spec selector upgrade is ambiguous: https://git.example.com/acme/one:one.yaml, https://git.example.com/acme/two:two.yaml
}
