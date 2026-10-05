package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/jackc/pgx/v5"
)

// ActiveRepoRunError identifies the run that prevents another launch or restart.
type ActiveRepoRunError struct {
	RepoID  types.RepoID
	RepoURL string
	RunID   types.RunID
}

func (e *ActiveRepoRunError) Error() string {
	return fmt.Sprintf("repository %s already has an active migration; current Run ID: %s", e.RepoURL, e.RunID)
}

func lockRunRepositories(ctx context.Context, q *Queries, repoIDs []types.RepoID) error {
	// Lock existing repository rows across replicas. Stable order prevents
	// overlapping waves from deadlocking; NO KEY UPDATE permits FK checks.
	rows, err := q.db.Query(ctx, `
		SELECT id FROM repos
		WHERE id = ANY($1::text[])
		ORDER BY id
		FOR NO KEY UPDATE
	`, repoIDs)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
	}
	return rows.Err()
}

// The caller must hold the repository lock until the run and jobs commit.
func requireRepoWithoutActiveRun(ctx context.Context, q *Queries, repoID types.RepoID) error {
	conflict := &ActiveRepoRunError{RepoID: repoID}
	err := q.db.QueryRow(ctx, `
		SELECT runs.id, repos.url
		FROM runs JOIN repos ON repos.id = runs.repo_id
		WHERE runs.repo_id = $1 AND runs.status IN ('Queued', 'Running')
		ORDER BY runs.created_at, runs.id
		LIMIT 1
	`, repoID).Scan(&conflict.RunID, &conflict.RepoURL)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("check active repository run: %w", err)
	}
	return conflict
}
