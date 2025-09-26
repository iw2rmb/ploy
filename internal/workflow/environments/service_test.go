package environments

import (
	"context"
	"errors"
	"sort"
	"testing"

	"github.com/iw2rmb/ploy/internal/workflow/contracts"
	"github.com/iw2rmb/ploy/internal/workflow/lanes"
	"github.com/iw2rmb/ploy/internal/workflow/manifests"
	"github.com/iw2rmb/ploy/internal/workflow/snapshots"
)

type stubLaneRegistry struct {
	specs map[string]lanes.Spec
	calls []laneCall
}

type laneCall struct {
	name string
	opts lanes.DescribeOptions
}

func newStubLaneRegistry(specs map[string]lanes.Spec) *stubLaneRegistry {
	return &stubLaneRegistry{specs: specs}
}

func (s *stubLaneRegistry) Describe(name string, opts lanes.DescribeOptions) (lanes.Description, error) {
	spec, ok := s.specs[name]
	if !ok {
		return lanes.Description{}, errors.New("unknown lane")
	}
	key, err := lanes.ComposeCacheKey(lanes.CacheKeyRequest{Lane: spec, DescribeOptions: opts})
	if err != nil {
		return lanes.Description{}, err
	}
	s.calls = append(s.calls, laneCall{name: name, opts: opts})
	return lanes.Description{Lane: spec, CacheKey: key, Parameters: opts}, nil
}

type stubSnapshotRegistry struct {
	plans        map[string]snapshots.PlanReport
	captures     map[string]snapshots.CaptureResult
	planCalls    []string
	captureCalls []captureCall
}

type captureCall struct {
	name string
	opts snapshots.CaptureOptions
}

func newStubSnapshotRegistry() *stubSnapshotRegistry {
	return &stubSnapshotRegistry{
		plans:    make(map[string]snapshots.PlanReport),
		captures: make(map[string]snapshots.CaptureResult),
	}
}

func (s *stubSnapshotRegistry) Plan(ctx context.Context, name string) (snapshots.PlanReport, error) {
	_ = ctx
	report, ok := s.plans[name]
	if !ok {
		return snapshots.PlanReport{}, errors.New("plan missing")
	}
	s.planCalls = append(s.planCalls, name)
	return report, nil
}

func (s *stubSnapshotRegistry) Capture(ctx context.Context, name string, opts snapshots.CaptureOptions) (snapshots.CaptureResult, error) {
	_ = ctx
	result, ok := s.captures[name]
	if !ok {
		return snapshots.CaptureResult{}, errors.New("capture missing")
	}
	s.captureCalls = append(s.captureCalls, captureCall{name: name, opts: opts})
	return result, nil
}

type recordingHydrator struct {
	calls []HydrationCall
}

type HydrationCall struct {
	Lane     string
	CacheKey string
}

func (r *recordingHydrator) HydrateCache(ctx context.Context, lane string, cacheKey string) error {
	_ = ctx
	r.calls = append(r.calls, HydrationCall{Lane: lane, CacheKey: cacheKey})
	return nil
}

func TestServiceDryRunPlansResources(t *testing.T) {
	manifest := manifests.Compilation{
		Manifest: manifests.Metadata{Name: "commit-app", Version: "2025-09-26"},
		Fixtures: manifests.FixtureSet{
			Required: []manifests.Fixture{
				{Name: "postgres", Reference: "snapshot:commit-db"},
				{Name: "redis", Reference: "snapshot:commit-cache"},
			},
		},
		Lanes: manifests.LaneSet{
			Required: []manifests.Lane{{Name: "go-native"}, {Name: "node-wasm"}},
		},
		Aster: manifests.AsterSet{Required: []string{"plan"}},
	}

	snapshotRegistry := newStubSnapshotRegistry()
	snapshotRegistry.plans["commit-db"] = snapshots.PlanReport{SnapshotName: "commit-db", Engine: "postgres"}
	snapshotRegistry.plans["commit-cache"] = snapshots.PlanReport{SnapshotName: "commit-cache", Engine: "redis"}

	laneRegistry := newStubLaneRegistry(map[string]lanes.Spec{
		"go-native": {Name: "go-native", CacheNamespace: "go", Commands: lanes.Commands{Build: []string{"go", "build"}, Test: []string{"go", "test"}}},
		"node-wasm": {Name: "node-wasm", CacheNamespace: "node", Commands: lanes.Commands{Build: []string{"pnpm", "build"}, Test: []string{"pnpm", "test"}}},
	})

	hydrator := &recordingHydrator{}

	svc := NewService(ServiceOptions{
		Lanes:     laneRegistry,
		Snapshots: snapshotRegistry,
		Hydrator:  hydrator,
	})

	result, err := svc.Materialize(context.Background(), Request{
		CommitSHA:   "deadbeef",
		App:         "commit-app",
		Tenant:      "acme",
		DryRun:      true,
		Manifest:    manifest,
		ManifestRef: contracts.ManifestReference{Name: "commit-app", Version: "2025-09-26"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.DryRun {
		t.Fatalf("expected dry-run result")
	}
	if len(result.Snapshots) != 2 {
		t.Fatalf("expected two snapshots, got %d", len(result.Snapshots))
	}
	names := []string{result.Snapshots[0].Name, result.Snapshots[1].Name}
	sort.Strings(names)
	if names[0] != "commit-cache" || names[1] != "commit-db" {
		t.Fatalf("unexpected snapshot names: %v", names)
	}
	for _, snap := range result.Snapshots {
		if snap.Attached {
			t.Fatalf("expected dry-run snapshots to be unattached")
		}
		if snap.Fingerprint != "" {
			t.Fatalf("expected no fingerprint in dry-run, got %q", snap.Fingerprint)
		}
	}

	if len(result.Caches) != 2 {
		t.Fatalf("expected two caches, got %d", len(result.Caches))
	}
	for _, cache := range result.Caches {
		if cache.Hydrated {
			t.Fatalf("expected dry-run caches to be unhydrated")
		}
		if cache.CacheKey == "" {
			t.Fatal("expected cache key to be populated")
		}
	}

	if len(snapshotRegistry.captureCalls) != 0 {
		t.Fatalf("expected no captures during dry-run, got %d", len(snapshotRegistry.captureCalls))
	}
	if len(snapshotRegistry.planCalls) != 2 {
		t.Fatalf("expected plan calls for both snapshots, got %v", snapshotRegistry.planCalls)
	}
	if len(hydrator.calls) != 0 {
		t.Fatalf("expected no hydrator calls in dry-run")
	}
}

func TestServiceMaterializeHydratesResources(t *testing.T) {
	manifest := manifests.Compilation{
		Manifest: manifests.Metadata{Name: "commit-app", Version: "2025-09-26"},
		Fixtures: manifests.FixtureSet{
			Required: []manifests.Fixture{
				{Name: "postgres", Reference: "snapshot:commit-db"},
			},
		},
		Lanes: manifests.LaneSet{
			Required: []manifests.Lane{{Name: "go-native"}},
		},
		Aster: manifests.AsterSet{Required: []string{"plan"}},
	}

	snapshotsRegistry := newStubSnapshotRegistry()
	snapshotsRegistry.plans["commit-db"] = snapshots.PlanReport{SnapshotName: "commit-db", Engine: "postgres"}
	snapshotsRegistry.captures["commit-db"] = snapshots.CaptureResult{
		Fingerprint: "fp-123",
		ArtifactCID: "cid-123",
		Metadata:    snapshots.SnapshotMetadata{SnapshotName: "commit-db"},
	}

	laneRegistry := newStubLaneRegistry(map[string]lanes.Spec{
		"go-native": {Name: "go-native", CacheNamespace: "go", Commands: lanes.Commands{Build: []string{"go", "build"}, Test: []string{"go", "test"}}},
	})

	hydrator := &recordingHydrator{}

	svc := NewService(ServiceOptions{
		Lanes:     laneRegistry,
		Snapshots: snapshotsRegistry,
		Hydrator:  hydrator,
	})

	result, err := svc.Materialize(context.Background(), Request{
		CommitSHA:    "deadbeef",
		App:          "commit-app",
		Tenant:       "acme",
		DryRun:       false,
		Manifest:     manifest,
		ManifestRef:  contracts.ManifestReference{Name: "commit-app", Version: "2025-09-26"},
		AsterToggles: []string{"ml"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.DryRun {
		t.Fatalf("expected execution mode")
	}
	if len(result.Snapshots) != 1 || !result.Snapshots[0].Attached {
		t.Fatalf("expected snapshot to be attached: %+v", result.Snapshots)
	}
	if result.Snapshots[0].Fingerprint != "fp-123" {
		t.Fatalf("unexpected fingerprint: %s", result.Snapshots[0].Fingerprint)
	}

	if len(result.Caches) != 1 {
		t.Fatalf("expected one cache entry, got %d", len(result.Caches))
	}
	cache := result.Caches[0]
	if !cache.Hydrated {
		t.Fatalf("expected cache to be hydrated")
	}
	if cache.CacheKey == "" {
		t.Fatal("expected cache key to be set")
	}
	if len(hydrator.calls) != 1 {
		t.Fatalf("expected hydrator to be invoked once, got %d", len(hydrator.calls))
	}
	if hydrator.calls[0].Lane != "go-native" {
		t.Fatalf("unexpected lane in hydration call: %+v", hydrator.calls[0])
	}
	if len(snapshotsRegistry.captureCalls) != 1 {
		t.Fatalf("expected capture to be invoked once, got %d", len(snapshotsRegistry.captureCalls))
	}
	if snapshotsRegistry.captureCalls[0].opts.Tenant != "acme" {
		t.Fatalf("expected tenant to be passed to capture, got %+v", snapshotsRegistry.captureCalls[0].opts)
	}
}

func TestServiceMaterializeFailsWhenSnapshotMissing(t *testing.T) {
	manifest := manifests.Compilation{
		Manifest: manifests.Metadata{Name: "commit-app", Version: "2025-09-26"},
		Fixtures: manifests.FixtureSet{
			Required: []manifests.Fixture{{Name: "postgres", Reference: "snapshot:missing"}},
		},
		Lanes: manifests.LaneSet{
			Required: []manifests.Lane{{Name: "go-native"}},
		},
	}

	snapshotsRegistry := newStubSnapshotRegistry()
	laneRegistry := newStubLaneRegistry(map[string]lanes.Spec{
		"go-native": {Name: "go-native", CacheNamespace: "go", Commands: lanes.Commands{Build: []string{"go", "build"}, Test: []string{"go", "test"}}},
	})

	svc := NewService(ServiceOptions{
		Lanes:     laneRegistry,
		Snapshots: snapshotsRegistry,
		Hydrator:  &recordingHydrator{},
	})

	_, err := svc.Materialize(context.Background(), Request{
		CommitSHA:   "deadbeef",
		App:         "commit-app",
		Tenant:      "acme",
		Manifest:    manifest,
		ManifestRef: contracts.ManifestReference{Name: "commit-app", Version: "2025-09-26"},
		DryRun:      true,
	})
	if err == nil {
		t.Fatal("expected error when snapshot plan missing")
	}
}
