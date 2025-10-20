package main

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/iw2rmb/ploy/internal/workflow/aster"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
	"github.com/iw2rmb/ploy/internal/workflow/runner"
)

func TestHandleModRunSupportsAutoTicket(t *testing.T) {
	t.Setenv(gridIDEnv, "")
	t.Setenv(gridIDFallbackEnv, "")
	t.Setenv(gridAPIKeyEnv, "")
	t.Setenv(gridAPIKeyFallbackEnv, "")
	t.Setenv(gridClientStateEnv, t.TempDir())
	withStubWorkspacePreparer(t)

	fakeRunner := &recordingRunner{}
	prevRunner := runnerExecutor
	prevEventsFactory := eventsFactory
	prevManifestLoader := manifestRegistryLoader
	prevManifestDir := manifestConfigDir
	prevLocatorLoader := asterLocatorLoader
	prevAsterDir := asterConfigDir
	prevJobComposerFactory := jobComposerFactory
	prevCacheComposerFactory := cacheComposerFactory
	defer func() {
		runnerExecutor = prevRunner
		eventsFactory = prevEventsFactory
		manifestRegistryLoader = prevManifestLoader
		manifestConfigDir = prevManifestDir
		asterLocatorLoader = prevLocatorLoader
		asterConfigDir = prevAsterDir
		jobComposerFactory = prevJobComposerFactory
		cacheComposerFactory = prevCacheComposerFactory
	}()

	runnerExecutor = fakeRunner
	eventsFactory = func(tenant string) (runner.EventsClient, error) { return contracts.NewInMemoryBus(tenant), nil }
	manifestRegistryLoader = func(dir string) (runner.ManifestCompiler, error) {
		return &stubManifestCompiler{compiled: defaultManifestPayload()}, nil
	}
	manifestConfigDir = "ignored"
	asterLocatorLoader = func(dir string) (aster.Locator, error) { return &recordingLocator{dir: dir}, nil }
	asterConfigDir = "ignored"
	jobComposerFactory = func() runner.JobComposer { return runner.NewStaticJobComposer() }
	cacheComposerFactory = func() runner.CacheComposer { return runner.NewDefaultCacheComposer() }

	err := handleModRun([]string{"--tenant", "acme", "--ticket", "auto"}, io.Discard)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fakeRunner.opts.Ticket != "" {
		t.Fatalf("expected empty ticket for auto claim, got %q", fakeRunner.opts.Ticket)
	}
	if fakeRunner.opts.Tenant != "acme" {
		t.Fatalf("unexpected tenant: %s", fakeRunner.opts.Tenant)
	}
	if fakeRunner.opts.JobComposer == nil {
		t.Fatal("expected job composer to be configured")
	}
	if fakeRunner.opts.CacheComposer == nil {
		t.Fatal("expected cache composer to be configured")
	}
}

func TestHandleModRunPropagatesRunnerError(t *testing.T) {
	t.Setenv(gridIDEnv, "")
	t.Setenv(gridIDFallbackEnv, "")
	t.Setenv(gridAPIKeyEnv, "")
	t.Setenv(gridAPIKeyFallbackEnv, "")
	t.Setenv(gridClientStateEnv, t.TempDir())
	withStubWorkspacePreparer(t)

	fakeRunner := &recordingRunner{err: errors.New("boom")}
	prevRunner := runnerExecutor
	prevEventsFactory := eventsFactory
	prevManifestLoader := manifestRegistryLoader
	prevManifestDir := manifestConfigDir
	prevLocatorLoader := asterLocatorLoader
	prevAsterDir := asterConfigDir
	prevJobComposerFactory := jobComposerFactory
	prevCacheComposerFactory := cacheComposerFactory
	defer func() {
		runnerExecutor = prevRunner
		eventsFactory = prevEventsFactory
		manifestRegistryLoader = prevManifestLoader
		manifestConfigDir = prevManifestDir
		asterLocatorLoader = prevLocatorLoader
		asterConfigDir = prevAsterDir
		jobComposerFactory = prevJobComposerFactory
		cacheComposerFactory = prevCacheComposerFactory
	}()

	runnerExecutor = fakeRunner
	eventsFactory = func(tenant string) (runner.EventsClient, error) { return contracts.NewInMemoryBus(tenant), nil }
	manifestRegistryLoader = func(dir string) (runner.ManifestCompiler, error) {
		return &stubManifestCompiler{compiled: defaultManifestPayload()}, nil
	}
	manifestConfigDir = "ignored"
	asterLocatorLoader = func(dir string) (aster.Locator, error) { return &recordingLocator{dir: dir}, nil }
	asterConfigDir = "ignored"
	jobComposerFactory = func() runner.JobComposer { return runner.NewStaticJobComposer() }
	cacheComposerFactory = func() runner.CacheComposer { return runner.NewDefaultCacheComposer() }

	err := handleModRun([]string{"--tenant", "acme", "--ticket", "ticket-123"}, io.Discard)
	if !errors.Is(err, fakeRunner.err) {
		t.Fatalf("expected runner error, got %v", err)
	}
}

func TestHandleModRunPropagatesManifestLoaderError(t *testing.T) {
	t.Setenv(gridIDEnv, "")
	t.Setenv(gridIDFallbackEnv, "")
	t.Setenv(gridAPIKeyEnv, "")
	t.Setenv(gridAPIKeyFallbackEnv, "")
	t.Setenv(gridClientStateEnv, t.TempDir())
	withStubWorkspacePreparer(t)

	prevLoader := manifestRegistryLoader
	prevDir := manifestConfigDir
	defer func() {
		manifestRegistryLoader = prevLoader
		manifestConfigDir = prevDir
	}()

	manifestRegistryLoader = func(dir string) (runner.ManifestCompiler, error) {
		return nil, errors.New("manifest load failed")
	}
	manifestConfigDir = "ignored"

	err := handleModRun([]string{"--tenant", "acme", "--ticket", "ticket-123"}, io.Discard)
	if err == nil {
		t.Fatal("expected manifest loader error")
	}
	if !strings.Contains(err.Error(), "manifest") {
		t.Fatalf("expected manifest error context, got %v", err)
	}
}

func TestHandleModRunRequiresTenant(t *testing.T) {
	buf := &bytes.Buffer{}
	err := handleModRun([]string{"--ticket", "auto"}, buf)
	if err == nil {
		t.Fatal("expected error for missing tenant")
	}
	if !strings.Contains(buf.String(), "Usage: ploy mod run") {
		t.Fatalf("expected usage output, got %q", buf.String())
	}
}

func TestHandleModRunTrimsExplicitTicket(t *testing.T) {
	t.Setenv(gridIDEnv, "")
	t.Setenv(gridIDFallbackEnv, "")
	t.Setenv(gridAPIKeyEnv, "")
	t.Setenv(gridAPIKeyFallbackEnv, "")
	t.Setenv(gridClientStateEnv, t.TempDir())
	withStubWorkspacePreparer(t)

	fakeRunner := &recordingRunner{}
	prevRunner := runnerExecutor
	prevEventsFactory := eventsFactory
	prevManifestLoader := manifestRegistryLoader
	prevManifestDir := manifestConfigDir
	prevLocatorLoader := asterLocatorLoader
	prevAsterDir := asterConfigDir
	prevJobComposerFactory := jobComposerFactory
	prevCacheComposerFactory := cacheComposerFactory
	defer func() {
		runnerExecutor = prevRunner
		eventsFactory = prevEventsFactory
		manifestRegistryLoader = prevManifestLoader
		manifestConfigDir = prevManifestDir
		asterLocatorLoader = prevLocatorLoader
		asterConfigDir = prevAsterDir
		jobComposerFactory = prevJobComposerFactory
		cacheComposerFactory = prevCacheComposerFactory
	}()

	runnerExecutor = fakeRunner
	eventsFactory = func(tenant string) (runner.EventsClient, error) { return contracts.NewInMemoryBus(tenant), nil }
	manifestRegistryLoader = func(dir string) (runner.ManifestCompiler, error) {
		return &stubManifestCompiler{compiled: defaultManifestPayload()}, nil
	}
	manifestConfigDir = "ignored"
	asterLocatorLoader = func(dir string) (aster.Locator, error) { return &recordingLocator{dir: dir}, nil }
	asterConfigDir = "ignored"
	jobComposerFactory = func() runner.JobComposer { return runner.NewStaticJobComposer() }
	cacheComposerFactory = func() runner.CacheComposer { return runner.NewDefaultCacheComposer() }

	err := handleModRun([]string{"--tenant", "acme", "--ticket", "  ticket-456  "}, io.Discard)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fakeRunner.opts.Ticket != "ticket-456" {
		t.Fatalf("expected trimmed ticket, got %q", fakeRunner.opts.Ticket)
	}
}
