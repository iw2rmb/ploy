package buildgate

import (
	"context"
	"errors"
)

var (
	// ErrStaticCheckRegistryNil indicates the registry has not been configured.
	ErrStaticCheckRegistryNil = errors.New("buildgate: static check registry not configured")
	// ErrStaticCheckAdapterNotFound is returned when a static check adapter is missing.
	ErrStaticCheckAdapterNotFound = errors.New("buildgate: static check adapter not found")
)

// StaticCheckAdapter executes language specific static analysis tooling.
type StaticCheckAdapter interface {
	Metadata() StaticCheckAdapterMetadata
	Run(ctx context.Context, req StaticCheckRequest) (StaticCheckResult, error)
}

// StaticCheckAdapterMetadata describes the adapter configuration exposed to the registry.
type StaticCheckAdapterMetadata struct {
	Language        string
	Tool            string
	DefaultSeverity SeverityLevel
}

// SeverityLevel expresses a diagnostic severity threshold.
type SeverityLevel string

const (
	SeverityInfo    SeverityLevel = "info"
	SeverityWarning SeverityLevel = "warning"
	SeverityError   SeverityLevel = "error"
)

// StaticCheckLaneConfig configures lane defaults for a language.
type StaticCheckLaneConfig struct {
	Enabled        bool
	FailOnSeverity SeverityLevel
	Options        map[string]string
}

// StaticCheckManifest captures manifest overrides for static checks.
type StaticCheckManifest struct {
	Languages map[string]StaticCheckManifestLanguage
}

// StaticCheckManifestLanguage describes per-language manifest configuration.
type StaticCheckManifestLanguage struct {
	Enabled        *bool
	FailOnSeverity string
	Options        map[string]string
}

// StaticCheckSpec describes a registry execution request.
type StaticCheckSpec struct {
	LaneDefaults  map[string]StaticCheckLaneConfig
	Manifest      StaticCheckManifest
	SkipLanguages []string
}

// StaticCheckRequest supplies adapter execution context.
type StaticCheckRequest struct {
	FailOnSeverity SeverityLevel
	Options        map[string]string
}

// StaticCheckResult captures adapter execution results.
type StaticCheckResult struct {
	Failures []StaticCheckFailure
}
