package hydration

import "errors"

// Stub implementations for node worker hydration package.
// These are minimal placeholders to allow compilation until full implementation.

// GitFetcherOptions holds configuration for the git fetcher.
type GitFetcherOptions struct {
	Publisher       interface{}
	PublishSnapshot bool
}

// GitFetcher fetches git repositories.
type GitFetcher interface{}

type gitFetcher struct{}

// NewGitFetcher creates a new git fetcher.
func NewGitFetcher(opts GitFetcherOptions) (GitFetcher, error) {
	_ = opts
	return &gitFetcher{}, errors.New("not implemented")
}
