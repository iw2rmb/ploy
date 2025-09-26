package environments

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/iw2rmb/ploy/internal/workflow/contracts"
	"github.com/iw2rmb/ploy/internal/workflow/lanes"
	"github.com/iw2rmb/ploy/internal/workflow/manifests"
	"github.com/iw2rmb/ploy/internal/workflow/snapshots"
)

// Hydrator executes cache hydration for a given lane/cache key pair.
// Implementations may prime caches, dispatch Grid jobs, or simply
// record the requested hydration in memory for tests.
type Hydrator interface {
	HydrateCache(ctx context.Context, lane string, cacheKey string) error
}

type laneDescriber interface {
	Describe(name string, opts lanes.DescribeOptions) (lanes.Description, error)
}

type snapshotPlanner interface {
	Plan(ctx context.Context, name string) (snapshots.PlanReport, error)
	Capture(ctx context.Context, name string, opts snapshots.CaptureOptions) (snapshots.CaptureResult, error)
}

// ServiceOptions configures Service dependencies.
type ServiceOptions struct {
	Lanes     laneDescriber
	Snapshots snapshotPlanner
	Hydrator  Hydrator
}

type Service struct {
	lanes     laneDescriber
	snapshots snapshotPlanner
	hydrator  Hydrator
}

// NewService wires the environment materialization service.
func NewService(opts ServiceOptions) Service {
	return Service{
		lanes:     opts.Lanes,
		snapshots: opts.Snapshots,
		hydrator:  opts.Hydrator,
	}
}

// Request captures the inputs required to materialize a commit-scoped environment.
type Request struct {
	CommitSHA    string
	App          string
	Tenant       string
	DryRun       bool
	Manifest     manifests.Compilation
	ManifestRef  contracts.ManifestReference
	AsterToggles []string
}

// Result summarizes the resources planned or hydrated for the environment.
type Result struct {
	CommitSHA    string
	App          string
	Tenant       string
	DryRun       bool
	Manifest     manifests.Compilation
	ManifestRef  contracts.ManifestReference
	AsterToggles []string
	Snapshots    []SnapshotStatus
	Caches       []CacheStatus
}

// SnapshotStatus reflects the availability/attachment state of a snapshot fixture.
type SnapshotStatus struct {
	Name        string
	Plan        snapshots.PlanReport
	Attached    bool
	Fingerprint string
	ArtifactCID string
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
	trimmedTenant := strings.TrimSpace(req.Tenant)
	if !req.DryRun && trimmedTenant == "" {
		return Result{}, errors.New("tenant is required when executing materialization")
	}

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

	snapshotNames := extractSnapshotNames(req.Manifest)
	snapshotsStatus := make([]SnapshotStatus, 0, len(snapshotNames))

	for _, snapshotName := range snapshotNames {
		plan, err := s.snapshots.Plan(ctx, snapshotName)
		if err != nil {
			return Result{}, fmt.Errorf("plan snapshot %s: %w", snapshotName, err)
		}
		status := SnapshotStatus{Name: snapshotName, Plan: plan}
		if !req.DryRun {
			capture, err := s.snapshots.Capture(ctx, snapshotName, snapshots.CaptureOptions{Tenant: trimmedTenant, TicketID: ticketIdentifier(trimmedApp, trimmedCommit)})
			if err != nil {
				return Result{}, fmt.Errorf("capture snapshot %s: %w", snapshotName, err)
			}
			status.Attached = true
			status.Fingerprint = capture.Fingerprint
			status.ArtifactCID = capture.ArtifactCID
		}
		snapshotsStatus = append(snapshotsStatus, status)
	}

	toggles := mergeAsterToggles(req.Manifest, req.AsterToggles)
	snapshotFingerprint := summarizeSnapshotFingerprint(snapshotsStatus)
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

		desc, err := s.lanes.Describe(trimmedLane, lanes.DescribeOptions{
			CommitSHA:           trimmedCommit,
			SnapshotFingerprint: snapshotFingerprint,
			ManifestVersion:     manifestVersion,
			AsterToggles:        toggles,
		})
		if err != nil {
			return Result{}, fmt.Errorf("describe lane %s: %w", trimmedLane, err)
		}

		status := CacheStatus{Lane: trimmedLane, CacheKey: desc.CacheKey, Hydrated: !req.DryRun}
		if !req.DryRun {
			if err := s.hydrator.HydrateCache(ctx, trimmedLane, desc.CacheKey); err != nil {
				return Result{}, fmt.Errorf("hydrate lane %s: %w", trimmedLane, err)
			}
		}
		cacheStatuses = append(cacheStatuses, status)
	}

	return Result{
		CommitSHA:    trimmedCommit,
		App:          trimmedApp,
		Tenant:       trimmedTenant,
		DryRun:       req.DryRun,
		Manifest:     req.Manifest,
		ManifestRef:  manifestRef,
		AsterToggles: toggles,
		Snapshots:    snapshotsStatus,
		Caches:       cacheStatuses,
	}, nil
}

func (s Service) validate() error {
	if s.lanes == nil {
		return errors.New("lane registry is required")
	}
	if s.snapshots == nil {
		return errors.New("snapshot registry is required")
	}
	if s.hydrator == nil {
		return errors.New("hydrator is required")
	}
	return nil
}

func extractSnapshotNames(comp manifests.Compilation) []string {
	set := make(map[string]struct{})
	for _, fixture := range comp.Fixtures.Required {
		if name, ok := parseSnapshotReference(fixture.Reference); ok {
			set[name] = struct{}{}
		}
	}
	for _, fixture := range comp.Fixtures.Optional {
		if name, ok := parseSnapshotReference(fixture.Reference); ok {
			set[name] = struct{}{}
		}
	}
	if len(set) == 0 {
		return nil
	}
	result := make([]string, 0, len(set))
	for name := range set {
		result = append(result, name)
	}
	sort.Strings(result)
	return result
}

func parseSnapshotReference(ref string) (string, bool) {
	trimmed := strings.TrimSpace(ref)
	const prefix = "snapshot:"
	if !strings.HasPrefix(trimmed, prefix) {
		return "", false
	}
	name := strings.TrimSpace(trimmed[len(prefix):])
	if name == "" {
		return "", false
	}
	return name, true
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

func summarizeSnapshotFingerprint(snapshots []SnapshotStatus) string {
	if len(snapshots) == 0 {
		return "none"
	}
	values := make([]string, 0, len(snapshots))
	for _, snap := range snapshots {
		candidate := strings.TrimSpace(snap.Fingerprint)
		if candidate == "" {
			candidate = "snapshot:" + snap.Name
		}
		values = append(values, candidate)
	}
	sort.Strings(values)
	return strings.Join(values, "+")
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
