package main

import (
	"bytes"
	"errors"
	"github.com/iw2rmb/ploy/internal/workflow/environments"
	"github.com/iw2rmb/ploy/internal/workflow/manifests"
	"github.com/iw2rmb/ploy/internal/workflow/runner"
	"io"
	"strings"
	"testing"
)

func TestHandleEnvironmentMaterializeRequiresCommit(t *testing.T) {
	prevFactory := environmentServiceFactory
	defer func() { environmentServiceFactory = prevFactory }()

	environmentServiceFactory = func(l laneRegistry, s snapshotRegistry) (environmentService, error) {
		return &recordingEnvironmentService{}, nil
	}

	buf := &bytes.Buffer{}
	err := handleEnvironmentMaterialize([]string{"--app", "commit-app", "--tenant", "acme"}, buf)
	if err == nil {
		t.Fatal("expected error when commit SHA is missing")
	}
	if !strings.Contains(buf.String(), "Usage: ploy environment materialize") {
		t.Fatalf("expected usage output, got %q", buf.String())
	}
}

func TestHandleEnvironmentMaterializeRequiresApp(t *testing.T) {
	prevFactory := environmentServiceFactory
	defer func() { environmentServiceFactory = prevFactory }()

	environmentServiceFactory = func(l laneRegistry, s snapshotRegistry) (environmentService, error) {
		return &recordingEnvironmentService{}, nil
	}

	buf := &bytes.Buffer{}
	err := handleEnvironmentMaterialize([]string{"deadbeef", "--tenant", "acme"}, buf)
	if err == nil {
		t.Fatal("expected error when app is missing")
	}
	if !strings.Contains(buf.String(), "Usage: ploy environment materialize") {
		t.Fatalf("expected usage output, got %q", buf.String())
	}
}

func TestHandleEnvironmentMaterializeInvokesService(t *testing.T) {
	t.Setenv("PLOY_ASTER_ENABLE", "1")
	prevFactory := environmentServiceFactory
	prevManifestLoader := manifestRegistryLoader
	prevManifestDir := manifestConfigDir
	prevLaneLoader := laneRegistryLoader
	prevLaneDir := laneConfigDir
	prevSnapshotLoader := snapshotRegistryLoader
	prevSnapshotDir := snapshotConfigDir
	defer func() {
		environmentServiceFactory = prevFactory
		manifestRegistryLoader = prevManifestLoader
		manifestConfigDir = prevManifestDir
		laneRegistryLoader = prevLaneLoader
		laneConfigDir = prevLaneDir
		snapshotRegistryLoader = prevSnapshotLoader
		snapshotConfigDir = prevSnapshotDir
	}()

	recorder := &recordingEnvironmentService{
		result: environments.Result{
			App:       "commit-app",
			CommitSHA: "deadbeef",
			DryRun:    true,
			Snapshots: []environments.SnapshotStatus{{Name: "commit-db"}},
			Caches:    []environments.CacheStatus{{Lane: "go-native", CacheKey: "go/go-native@commit=deadbeef@snapshot=pending@manifest=2025-09-26@aster=plan", Hydrated: false}},
		},
	}

	environmentServiceFactory = func(l laneRegistry, s snapshotRegistry) (environmentService, error) {
		return recorder, nil
	}

	laneRegistryLoader = func(dir string) (laneRegistry, error) { return nil, nil }
	laneConfigDir = "ignored"
	snapshotRegistryLoader = func(dir string) (snapshotRegistry, error) { return nil, nil }
	snapshotConfigDir = "ignored"

	manifestRegistryLoader = func(dir string) (runner.ManifestCompiler, error) {
		return &stubManifestCompiler{compiled: manifests.Compilation{
			Manifest:        manifests.Metadata{Name: "commit-app", Version: "2025-09-26"},
			ManifestVersion: "v2",
			Fixtures:        manifests.FixtureSet{Required: []manifests.Fixture{{Name: "postgres", Reference: "snapshot:commit-db"}}},
		}}, nil
	}
	manifestConfigDir = "ignored"

	buf := &bytes.Buffer{}
	err := handleEnvironmentMaterialize([]string{"deadbeef", "--app", "commit-app", "--tenant", "acme", "--dry-run", "--aster", "lint"}, buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if recorder.request.CommitSHA != "deadbeef" {
		t.Fatalf("unexpected commit: %s", recorder.request.CommitSHA)
	}
	if recorder.request.App != "commit-app" {
		t.Fatalf("unexpected app: %s", recorder.request.App)
	}
	if !recorder.request.DryRun {
		t.Fatal("expected dry-run request")
	}
	if !recorder.request.AsterEnabled {
		t.Fatal("expected aster to be enabled when flag is set")
	}
	if len(recorder.request.AsterToggles) != 1 || recorder.request.AsterToggles[0] != "lint" {
		t.Fatalf("unexpected aster toggles: %v", recorder.request.AsterToggles)
	}

	output := buf.String()
	for _, fragment := range []string{"Environment: commit-app", "Mode: dry-run", "commit-db", "go-native"} {
		if !strings.Contains(output, fragment) {
			t.Fatalf("expected output to contain %q, got %q", fragment, output)
		}
	}
}

func TestHandleEnvironmentMaterializeIgnoresAsterWhenFlagDisabled(t *testing.T) {
	prevFactory := environmentServiceFactory
	prevManifestLoader := manifestRegistryLoader
	prevManifestDir := manifestConfigDir
	prevLaneLoader := laneRegistryLoader
	prevLaneDir := laneConfigDir
	prevSnapshotLoader := snapshotRegistryLoader
	prevSnapshotDir := snapshotConfigDir
	defer func() {
		environmentServiceFactory = prevFactory
		manifestRegistryLoader = prevManifestLoader
		manifestConfigDir = prevManifestDir
		laneRegistryLoader = prevLaneLoader
		laneConfigDir = prevLaneDir
		snapshotRegistryLoader = prevSnapshotLoader
		snapshotConfigDir = prevSnapshotDir
	}()

	recorder := &recordingEnvironmentService{}
	environmentServiceFactory = func(l laneRegistry, s snapshotRegistry) (environmentService, error) {
		return recorder, nil
	}
	laneRegistryLoader = func(dir string) (laneRegistry, error) { return nil, nil }
	laneConfigDir = "ignored"
	snapshotRegistryLoader = func(dir string) (snapshotRegistry, error) { return nil, nil }
	snapshotConfigDir = "ignored"
	manifestRegistryLoader = func(dir string) (runner.ManifestCompiler, error) {
		return &stubManifestCompiler{compiled: manifests.Compilation{Manifest: manifests.Metadata{Name: "commit-app", Version: "2025-09-26"}, ManifestVersion: "v2"}}, nil
	}
	manifestConfigDir = "ignored"

	if err := handleEnvironmentMaterialize([]string{"deadbeef", "--app", "commit-app", "--tenant", "acme", "--aster", "lint"}, io.Discard); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if recorder.request.AsterEnabled {
		t.Fatal("expected aster to be disabled without feature flag")
	}
	if len(recorder.request.AsterToggles) != 0 {
		t.Fatalf("expected no aster toggles when flag disabled, got %v", recorder.request.AsterToggles)
	}
}

func TestHandleEnvironmentMaterializePropagatesError(t *testing.T) {
	prevFactory := environmentServiceFactory
	prevManifestLoader := manifestRegistryLoader
	prevManifestDir := manifestConfigDir
	prevLaneLoader := laneRegistryLoader
	prevLaneDir := laneConfigDir
	prevSnapshotLoader := snapshotRegistryLoader
	prevSnapshotDir := snapshotConfigDir
	defer func() {
		environmentServiceFactory = prevFactory
		manifestRegistryLoader = prevManifestLoader
		manifestConfigDir = prevManifestDir
		laneRegistryLoader = prevLaneLoader
		laneConfigDir = prevLaneDir
		snapshotRegistryLoader = prevSnapshotLoader
		snapshotConfigDir = prevSnapshotDir
	}()

	recorder := &recordingEnvironmentService{err: errors.New("boom")}
	environmentServiceFactory = func(l laneRegistry, s snapshotRegistry) (environmentService, error) {
		return recorder, nil
	}

	manifestRegistryLoader = func(dir string) (runner.ManifestCompiler, error) {
		return &stubManifestCompiler{compiled: manifests.Compilation{
			Manifest:        manifests.Metadata{Name: "commit-app", Version: "2025-09-26"},
			ManifestVersion: "v2",
		}}, nil
	}
	manifestConfigDir = "ignored"
	laneRegistryLoader = func(dir string) (laneRegistry, error) { return nil, nil }
	laneConfigDir = "ignored"
	snapshotRegistryLoader = func(dir string) (snapshotRegistry, error) { return nil, nil }
	snapshotConfigDir = "ignored"

	err := handleEnvironmentMaterialize([]string{"deadbeef", "--app", "commit-app", "--tenant", "acme"}, io.Discard)
	if !errors.Is(err, recorder.err) {
		t.Fatalf("expected service error, got %v", err)
	}
}
