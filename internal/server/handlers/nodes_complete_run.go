package handlers

import (
	"context"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/server"
	"github.com/iw2rmb/ploy/internal/server/recovery"
	"github.com/iw2rmb/ploy/internal/store"
)

// maybeUpdateRunRepoStatus delegates to the canonical recovery reconciliation path.
func maybeUpdateRunRepoStatus(
	ctx context.Context,
	st store.Store,
	runID domaintypes.RunID,
	repoID domaintypes.MigRepoID,
	attempt int32,
) (bool, error) {
	return recovery.MaybeUpdateRunRepoStatus(ctx, st, runID, repoID, attempt)
}

// maybeCompleteRunIfAllReposTerminal delegates to the canonical recovery reconciliation path.
func maybeCompleteRunIfAllReposTerminal(ctx context.Context, st store.Store, eventsService *server.EventsService, run store.Run, runID domaintypes.RunID) error {
	return recovery.MaybeCompleteRunIfAllReposTerminal(ctx, st, eventsService, run, runID)
}
