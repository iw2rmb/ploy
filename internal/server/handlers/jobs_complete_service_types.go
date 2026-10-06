package handlers

import (
	"fmt"
	"net/http"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/server/blobpersist"
	"github.com/iw2rmb/ploy/internal/server/events"
	"github.com/iw2rmb/ploy/internal/server/gitlabtokens"
	"github.com/iw2rmb/ploy/internal/store"
)

// completionInput holds validated completion payload and caller identity.
type completionInput struct {
	JobID        domaintypes.JobID
	NodeID       domaintypes.NodeID
	Status       domaintypes.JobStatus
	ExitCode     *int32
	StatsPayload JobStatsPayload
	StatsBytes   []byte
	RepoSHAOut   string
}

// completionService orchestrates job completion workflow.
type completionService struct {
	store         store.Store
	eventsService *events.Service
	blobpersist   *blobpersist.Service
	gitLabTokens  *gitlabtokens.Registry
}

func newCompletionService(st store.Store, eventsService *events.Service, bp *blobpersist.Service, registries ...*gitlabtokens.Registry) *completionService {
	var registry *gitlabtokens.Registry
	if len(registries) > 0 {
		registry = registries[0]
	}
	return &completionService{
		store:         st,
		eventsService: eventsService,
		blobpersist:   bp,
		gitLabTokens:  registry,
	}
}

type completionError struct {
	status  int
	message string
	cause   error
}

func (e *completionError) Error() string {
	if e.cause == nil {
		return e.message
	}
	return fmt.Sprintf("%s: %v", e.message, e.cause)
}

func (e *completionError) Unwrap() error { return e.cause }

func completeBadRequest(format string, args ...any) error {
	return newCompletionError(http.StatusBadRequest, fmt.Sprintf(format, args...), nil)
}

func completeForbidden(format string, args ...any) error {
	return newCompletionError(http.StatusForbidden, fmt.Sprintf(format, args...), nil)
}

func completeConflict(format string, args ...any) error {
	return newCompletionError(http.StatusConflict, fmt.Sprintf(format, args...), nil)
}

func completeNotFound(format string, args ...any) error {
	return newCompletionError(http.StatusNotFound, fmt.Sprintf(format, args...), nil)
}

func completeInternal(message string, err error) error {
	return newCompletionError(http.StatusInternalServerError, message, err)
}

func newCompletionError(status int, message string, cause error) error {
	return &completionError{status: status, message: message, cause: cause}
}
