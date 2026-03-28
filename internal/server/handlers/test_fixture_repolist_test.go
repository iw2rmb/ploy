package handlers

import (
	"context"

	"github.com/iw2rmb/ploy/internal/store"
)

// repoListStore is a focused mock for repo listing handler tests.
type repoListStore struct {
	store.Store
	listDistinctRepos mockCall[string, []store.ListDistinctReposRow]
	listRunsForRepo   mockCall[store.ListRunsForRepoParams, []store.ListRunsForRepoRow]
}

func (m *repoListStore) ListDistinctRepos(ctx context.Context, filter string) ([]store.ListDistinctReposRow, error) {
	return m.listDistinctRepos.record(filter)
}

func (m *repoListStore) ListRunsForRepo(ctx context.Context, arg store.ListRunsForRepoParams) ([]store.ListRunsForRepoRow, error) {
	return m.listRunsForRepo.record(arg)
}
