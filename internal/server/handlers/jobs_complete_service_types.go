package handlers

import (
	"fmt"

	"github.com/iw2rmb/ploy/internal/blobstore"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/server"
	"github.com/iw2rmb/ploy/internal/server/blobpersist"
	"github.com/iw2rmb/ploy/internal/store"
)

// CompleteJobInput holds validated completion payload and caller identity.
type CompleteJobInput struct {
	JobID        domaintypes.JobID
	NodeID       domaintypes.NodeID
	Status       domaintypes.JobStatus
	ExitCode     *int32
	StatsPayload JobStatsPayload
	StatsBytes   []byte
	RepoSHAOut   string
}

// CompleteJobResult is returned on successful completion.
type CompleteJobResult struct{}

// CompleteJobService orchestrates job completion workflow.
type CompleteJobService struct {
	store          store.Store
	eventsService  *server.EventsService
	blobpersist    *blobpersist.Service
	gateProfilesBS blobstore.Store
}

type completeJobServiceType string

const (
	completeJobServiceTypeGate completeJobServiceType = "gate"
	completeJobServiceTypeStep completeJobServiceType = "step"
	completeJobServiceTypeSBOM completeJobServiceType = "sbom"
)

func routeCompleteJobServiceType(jobType domaintypes.JobType) (completeJobServiceType, bool) {
	switch jobType {
	case domaintypes.JobTypePreGate, domaintypes.JobTypePostGate:
		return completeJobServiceTypeGate, true
	case domaintypes.JobTypeMig:
		return completeJobServiceTypeStep, true
	case domaintypes.JobTypeSBOM:
		return completeJobServiceTypeSBOM, true
	default:
		return "", false
	}
}

func NewCompleteJobService(st store.Store, eventsService *server.EventsService, bp *blobpersist.Service, gateProfilesBS blobstore.Store) *CompleteJobService {
	return &CompleteJobService{
		store:          st,
		eventsService:  eventsService,
		blobpersist:    bp,
		gateProfilesBS: gateProfilesBS,
	}
}

// CompleteJobBadRequest maps to HTTP 400.
type CompleteJobBadRequest struct{ Message string }

func (e *CompleteJobBadRequest) Error() string { return e.Message }

// CompleteJobForbidden maps to HTTP 403.
type CompleteJobForbidden struct{ Message string }

func (e *CompleteJobForbidden) Error() string { return e.Message }

// CompleteJobConflict maps to HTTP 409.
type CompleteJobConflict struct{ Message string }

func (e *CompleteJobConflict) Error() string { return e.Message }

// CompleteJobNotFound maps to HTTP 404.
type CompleteJobNotFound struct{ Message string }

func (e *CompleteJobNotFound) Error() string { return e.Message }

// CompleteJobInternal maps to HTTP 500.
type CompleteJobInternal struct {
	Message string
	Err     error
}

func (e *CompleteJobInternal) Error() string {
	if e.Err == nil {
		return e.Message
	}
	return fmt.Sprintf("%s: %v", e.Message, e.Err)
}

func (e *CompleteJobInternal) Unwrap() error { return e.Err }

func completeBadRequest(format string, args ...any) error {
	return &CompleteJobBadRequest{Message: fmt.Sprintf(format, args...)}
}

func completeForbidden(format string, args ...any) error {
	return &CompleteJobForbidden{Message: fmt.Sprintf(format, args...)}
}

func completeConflict(format string, args ...any) error {
	return &CompleteJobConflict{Message: fmt.Sprintf(format, args...)}
}

func completeNotFound(format string, args ...any) error {
	return &CompleteJobNotFound{Message: fmt.Sprintf(format, args...)}
}

func completeInternal(message string, err error) error {
	return &CompleteJobInternal{Message: message, Err: err}
}
