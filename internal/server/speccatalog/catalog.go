package speccatalog

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/gitauth"
)

var (
	ErrNoRepositories = errors.New("no spec repositories configured")
	ErrNotFound       = errors.New("named spec not found")
)

// Entry identifies one named spec at the repository commit used for discovery.
type Entry struct {
	Name        string
	Description string
	Source      string
	Path        string
	SHA         string
}

// Catalog refreshes and resolves the configured Git-backed named-spec catalog.
type Catalog interface {
	List(context.Context) ([]Entry, error)
	Resolve(context.Context, string) (Entry, error)
}

type Options struct {
	Repositories []domaintypes.RepoURL
	CacheDir     string
	Auth         gitauth.Options
}

type Service struct {
	repositories []*repository
}

type repository struct {
	cloneURL string
	source   string
	checkout string
	auth     gitauth.Options
	runner   gitRunner

	mu       sync.Mutex
	inFlight *refreshCall
}

type refreshCall struct {
	done    chan struct{}
	entries []Entry
	err     error
}

func New(opts Options) (*Service, error) {
	cacheDir := strings.TrimSpace(opts.CacheDir)
	if cacheDir == "" {
		cacheDir = filepath.Join(os.TempDir(), fmt.Sprintf("ployd-%d", os.Getpid()))
	}

	repositories := make([]*repository, 0, len(opts.Repositories))
	for _, configuredURL := range opts.Repositories {
		cloneURL := strings.TrimSpace(configuredURL.String())
		prepared := gitauth.PrepareURL(cloneURL, gitauth.Options{})
		source := domaintypes.NormalizeRepoURL(prepared.URL)
		if source == "" {
			return nil, errors.New("spec repository URL is empty")
		}
		digest := sha256.Sum256([]byte(source))
		repositories = append(repositories, &repository{
			cloneURL: cloneURL,
			source:   source,
			checkout: filepath.Join(cacheDir, "spec-repositories", fmt.Sprintf("%x", digest)),
			auth:     opts.Auth,
			runner:   execGitRunner{},
		})
	}
	sort.Slice(repositories, func(i, j int) bool {
		return repositories[i].source < repositories[j].source
	})
	return &Service{repositories: repositories}, nil
}

func (s *Service) List(ctx context.Context) ([]Entry, error) {
	if len(s.repositories) == 0 {
		return nil, ErrNoRepositories
	}

	type result struct {
		index   int
		entries []Entry
		err     error
	}
	results := make(chan result, len(s.repositories))
	for i, repo := range s.repositories {
		go func() {
			entries, err := repo.refresh(ctx)
			results <- result{index: i, entries: entries, err: err}
		}()
	}

	byRepository := make([]result, len(s.repositories))
	for range s.repositories {
		result := <-results
		byRepository[result.index] = result
	}
	var entries []Entry
	for i, result := range byRepository {
		if result.err != nil {
			return nil, fmt.Errorf("refresh spec repository %s: %w", s.repositories[i].source, result.err)
		}
		entries = append(entries, result.entries...)
	}
	sortEntries(entries)
	return entries, nil
}

func (s *Service) Resolve(ctx context.Context, selector string) (Entry, error) {
	entries, err := s.List(ctx)
	if err != nil {
		return Entry{}, err
	}
	return resolveEntries(entries, selector)
}

func (r *repository) refresh(ctx context.Context) ([]Entry, error) {
	r.mu.Lock()
	if call := r.inFlight; call != nil {
		r.mu.Unlock()
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-call.done:
			return cloneEntries(call.entries), call.err
		}
	}
	call := &refreshCall{done: make(chan struct{})}
	r.inFlight = call
	r.mu.Unlock()

	call.entries, call.err = r.refreshNow(ctx)

	r.mu.Lock()
	r.inFlight = nil
	close(call.done)
	r.mu.Unlock()
	return cloneEntries(call.entries), call.err
}

func cloneEntries(entries []Entry) []Entry {
	return append([]Entry(nil), entries...)
}

func sortEntries(entries []Entry) {
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Name != entries[j].Name {
			return entries[i].Name < entries[j].Name
		}
		if entries[i].Source != entries[j].Source {
			return entries[i].Source < entries[j].Source
		}
		return entries[i].Path < entries[j].Path
	})
}
