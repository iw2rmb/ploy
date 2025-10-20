package main

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/iw2rmb/ploy/internal/workflow/environments"
	"github.com/iw2rmb/ploy/internal/workflow/manifests"
	"github.com/iw2rmb/ploy/internal/workflow/runner"
)

func TestHandleEnvironmentMaterializeRequiresCommit(t *testing.T) {
	prevFactory := environmentServiceFactory
	defer func() { environmentServiceFactory = prevFactory }()

	environmentServiceFactory = func(s snapshotRegistry) (environmentService, error) {
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

	environmentServiceFactory = func(s snapshotRegistry) (environmentService, error) {
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
	prevSnapshotLoader := snapshotRegistryLoader
	prevSnapshotDir := snapshotConfigDir
	defer func() {
		environmentServiceFactory = prevFactory
		manifestRegistryLoader = prevManifestLoader
		manifestConfigDir = prevManifestDir
		snapshotRegistryLoader = prevSnapshotLoader
		snapshotConfigDir = prevSnapshotDir
	}()

	recorder := &recordingEnvironmentService{
		result: environments.Result{
			App:       "commit-app",
			CommitSHA: "deadbeef",
			DryRun:    true,
			Snapshots: []environments.SnapshotStatus{{Name: "commit-db"}},
			Caches:    []environments.CacheStatus{{Lane: "go-native", CacheKey: "go/go-native@commit=deadbeef@snapshot=none@manifest=2025-09-26@aster=plan", Hydrated: false}},
		},
	}

	environmentServiceFactory = func(s snapshotRegistry) (environmentService, error) {
		return recorder, nil
	}

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

func TestHandleEnvironmentMaterializePropagatesServiceError(t *testing.T) {
	prevFactory := environmentServiceFactory
	prevSnapshotLoader := snapshotRegistryLoader
	prevSnapshotDir := snapshotConfigDir
	defer func() { environmentServiceFactory = prevFactory }()
	defer func() {
		snapshotRegistryLoader = prevSnapshotLoader
		snapshotConfigDir = prevSnapshotDir
	}()

	sentinel := errors.New("boom")
	environmentServiceFactory = func(s snapshotRegistry) (environmentService, error) {
		return &recordingEnvironmentService{err: sentinel}, nil
	}
	snapshotRegistryLoader = func(dir string) (snapshotRegistry, error) { return &fakeSnapshotRegistry{}, nil }
	snapshotConfigDir = "ignored"

	manifestRegistryLoader = func(dir string) (runner.ManifestCompiler, error) {
		return &stubManifestCompiler{compiled: defaultManifestPayload()}, nil
	}

	err := handleEnvironmentMaterialize([]string{"deadbeef", "--app", "commit-app", "--tenant", "acme"}, io.Discard)
	if !errors.Is(err, sentinel) {
		t.Fatalf("expected service error, got %v", err)
	}
}
