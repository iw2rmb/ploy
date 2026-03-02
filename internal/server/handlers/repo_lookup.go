package handlers

import (
	"context"
	"fmt"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
)

func repoURLForID(ctx context.Context, st store.Store, repoID domaintypes.RepoID) (string, error) {
	repo, err := st.GetRepo(ctx, repoID)
	if err != nil {
		return "", fmt.Errorf("get repo %s: %w", repoID, err)
	}
	return repo.Url, nil
}
