package handlers

import (
	"fmt"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/server/blobpersist"
	"github.com/iw2rmb/ploy/internal/server/events"
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

// completionResult is returned on successful completion.
type completionResult struct{}

// completionService orchestrates job completion workflow.
type completionService struct {
	store         store.Store
	eventsService *events.Service
	blobpersist   *blobpersist.Service
}

type completionServiceType string

const (
	completionServiceTypeGate completionServiceType = "gate"
	completionServiceTypeStep completionServiceType = "step"
)

func routeCompletionServiceType(jobType domaintypes.JobType) (completionServiceType, bool) {
	switch jobType {
	case domaintypes.JobTypePreGate, domaintypes.JobTypePostGate:
		return completionServiceTypeGate, true
	case domaintypes.JobTypeMig:
		return completionServiceTypeStep, true
	default:
		return "", false
	}
}

func newCompletionService(st store.Store, eventsService *events.Service, bp *blobpersist.Service) *completionService {
	return &completionService{
		store:         st,
		eventsService: eventsService,
		blobpersist:   bp,
	}
}

// completionBadRequest maps to HTTP 400.
type completionBadRequest struct{ Message string }

func (e *completionBadRequest) Error() string { return e.Message }

// completionForbidden maps to HTTP 403.
type completionForbidden struct{ Message string }

func (e *completionForbidden) Error() string { return e.Message }

// completionConflict maps to HTTP 409.
type completionConflict struct{ Message string }

func (e *completionConflict) Error() string { return e.Message }

// completionNotFound maps to HTTP 404.
type completionNotFound struct{ Message string }

func (e *completionNotFound) Error() string { return e.Message }

// completionInternal maps to HTTP 500.
type completionInternal struct {
	Message string
	Err     error
}

func (e *completionInternal) Error() string {
	if e.Err == nil {
		return e.Message
	}
	return fmt.Sprintf("%s: %v", e.Message, e.Err)
}

func (e *completionInternal) Unwrap() error { return e.Err }

func completeBadRequest(format string, args ...any) error {
	return &completionBadRequest{Message: fmt.Sprintf(format, args...)}
}

func completeForbidden(format string, args ...any) error {
	return &completionForbidden{Message: fmt.Sprintf(format, args...)}
}

func completeConflict(format string, args ...any) error {
	return &completionConflict{Message: fmt.Sprintf(format, args...)}
}

func completeNotFound(format string, args ...any) error {
	return &completionNotFound{Message: fmt.Sprintf(format, args...)}
}

func completeInternal(message string, err error) error {
	return &completionInternal{Message: message, Err: err}
}
