package runner

import "errors"

var (
	ErrTicketRequired              = errors.New("ticket is required")
	ErrEventsClientRequired        = errors.New("events client is required")
	ErrGridClientRequired          = errors.New("grid client is required")
	ErrGridCancellationUnsupported = errors.New("grid cancellation unsupported")
	ErrPlannerRequired             = errors.New("planner is required")
	ErrManifestCompilerRequired    = errors.New("manifest compiler is required")
	ErrTicketValidationFailed      = errors.New("ticket payload failed validation")
	ErrCheckpointValidationFailed  = errors.New("checkpoint payload failed validation")
	ErrArtifactValidationFailed    = errors.New("artifact envelope failed validation")
	ErrStageFailed                 = errors.New("workflow stage failed")
	ErrLaneRequired                = errors.New("lane is required")
	ErrAsterLocatorRequired        = errors.New("aster locator is required")
)
