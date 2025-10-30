package environments

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/iw2rmb/ploy/internal/workflow/contracts"
	"github.com/iw2rmb/ploy/internal/workflow/manifests"
	"github.com/iw2rmb/ploy/internal/workflow/runner"
)

// Hydrator executes cache hydration for a given lane/cache key pair.
// Implementations may prime caches, dispatch Grid jobs, or simply
// record the requested hydration in memory for tests.
type Hydrator interface {
	HydrateCache(ctx context.Context, lane string, cacheKey string) error
}

// ServiceOptions configures Service dependencies.
type ServiceOptions struct {
	Hydrator  Hydrator
}

type Service struct {
	hydrator  Hydrator
}

// NewService wires the environment materialization service.
func NewService(opts ServiceOptions) Service { return Service{hydrator: opts.Hydrator} }

// Request captures the inputs required to materialize a commit-scoped environment.
type Request struct {
    CommitSHA    string
    App          string
    DryRun       bool
    Manifest     manifests.Compilation
    ManifestRef  contracts.ManifestReference
    AsterEnabled bool
    AsterToggles []string
}

// Result summarizes the resources planned or hydrated for the environment.
type Result struct {
    CommitSHA    string
    App          string
    DryRun       bool
    Manifest     manifests.Compilation
    ManifestRef  contracts.ManifestReference
	AsterToggles []string
	Caches       []CacheStatus
}

// CacheStatus reflects the hydration state of a lane cache for the environment.
type CacheStatus struct {
	Lane     string
	CacheKey string
	Hydrated bool
}

// Materialize plans and optionally hydrates the resources required for a commit-scoped environment.
func (s Service) Materialize(ctx context.Context, req Request) (Result, error) {
	if err := s.validate(); err != nil {
		return Result{}, err
	}

	trimmedCommit := strings.TrimSpace(req.CommitSHA)
	if trimmedCommit == "" {
		return Result{}, errors.New("commit SHA is required")
	}
	trimmedApp := strings.TrimSpace(req.App)
	if trimmedApp == "" {
		return Result{}, errors.New("app identifier is required")
	}
    // tenant removed

	manifestMeta := req.Manifest.Manifest
	if strings.TrimSpace(manifestMeta.Name) == "" {
		return Result{}, errors.New("manifest name is required")
	}
	if strings.TrimSpace(manifestMeta.Version) == "" {
		return Result{}, errors.New("manifest version is required")
	}

	manifestRef := req.ManifestRef
	if strings.TrimSpace(manifestRef.Name) == "" {
		manifestRef.Name = manifestMeta.Name
	}
	if strings.TrimSpace(manifestRef.Version) == "" {
		manifestRef.Version = manifestMeta.Version
	}

	// Snapshot planning/capture removed from environment materialization.

	var toggles []string
	if req.AsterEnabled {
		toggles = mergeAsterToggles(req.Manifest, req.AsterToggles)
	}
	manifestVersion := manifestMeta.Version

	cacheStatuses := make([]CacheStatus, 0, len(req.Manifest.Lanes.Required))
	seenLanes := make(map[string]struct{}, len(req.Manifest.Lanes.Required))

	for _, lane := range req.Manifest.Lanes.Required {
		trimmedLane := strings.TrimSpace(lane.Name)
		if trimmedLane == "" {
			continue
		}
		if _, exists := seenLanes[trimmedLane]; exists {
			continue
		}
		seenLanes[trimmedLane] = struct{}{}

		cacheNamespace, err := runner.CacheNamespaceForLane(trimmedLane)
		if err != nil {
			return Result{}, fmt.Errorf("resolve cache namespace for lane %s: %w", trimmedLane, err)
		}

		cacheKey := composeCacheKey(cacheNamespace, trimmedLane, trimmedCommit, manifestVersion, toggles)

		status := CacheStatus{Lane: trimmedLane, CacheKey: cacheKey, Hydrated: !req.DryRun}
		if !req.DryRun {
			if err := s.hydrator.HydrateCache(ctx, trimmedLane, cacheKey); err != nil {
				return Result{}, fmt.Errorf("hydrate lane %s: %w", trimmedLane, err)
			}
		}
		cacheStatuses = append(cacheStatuses, status)
	}

    return Result{
        CommitSHA:    trimmedCommit,
        App:          trimmedApp,
        DryRun:       req.DryRun,
        Manifest:     req.Manifest,
        ManifestRef:  manifestRef,
		AsterToggles: toggles,
		Caches:       cacheStatuses,
	}, nil
}

func (s Service) validate() error {
	if s.hydrator == nil {
		return errors.New("hydrator is required")
	}
	return nil
}

func mergeAsterToggles(comp manifests.Compilation, extras []string) []string {
	set := make(map[string]struct{})
	for _, value := range comp.Aster.Required {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			set[trimmed] = struct{}{}
		}
	}
	for _, value := range extras {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			set[trimmed] = struct{}{}
		}
	}
	if len(set) == 0 {
		return nil
	}
	result := make([]string, 0, len(set))
	for value := range set {
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func composeCacheKey(namespace, lane, commit, manifest string, toggles []string) string {
	ns := strings.TrimSpace(namespace)
	if ns == "" {
		ns = strings.TrimSpace(lane)
	}
	laneName := strings.TrimSpace(lane)
	if laneName == "" {
		laneName = ns
	}
	toggleComponent := formatToggleComponent(toggles)
	return fmt.Sprintf("%s/%s@commit=%s@manifest=%s@aster=%s",
		ns,
		laneName,
		sanitizeCacheComponent(commit),
		sanitizeCacheComponent(manifest),
		toggleComponent,
	)
}

func sanitizeCacheComponent(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "none"
	}
	return trimmed
}

func formatToggleComponent(values []string) string {
	if len(values) == 0 {
		return "none"
	}
	clean := make([]string, 0, len(values))
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			clean = append(clean, trimmed)
		}
	}
	if len(clean) == 0 {
		return "none"
	}
	sort.Strings(clean)
	return strings.Join(clean, "+")
}

func ticketIdentifier(app, commit string) string {
	trimmedApp := sanitizeIdentifier(app)
	trimmedCommit := strings.TrimSpace(commit)
	if len(trimmedCommit) > 12 {
		trimmedCommit = trimmedCommit[:12]
	}
	if trimmedCommit == "" {
		trimmedCommit = "unknown"
	}
	return fmt.Sprintf("env-%s-%s", trimmedApp, trimmedCommit)
}

func sanitizeIdentifier(value string) string {
	trimmed := strings.TrimSpace(strings.ToLower(value))
	if trimmed == "" {
		return "environment"
	}
	builder := strings.Builder{}
	for _, r := range trimmed {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			builder.WriteRune(r)
			continue
		}
		if r == ' ' || r == '_' {
			builder.WriteRune('-')
		}
	}
	result := builder.String()
	if result == "" {
		return "environment"
	}
	return result
}
