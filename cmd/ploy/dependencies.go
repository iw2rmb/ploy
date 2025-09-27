package main

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strings"

	"github.com/iw2rmb/ploy/internal/workflow/aster"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
	"github.com/iw2rmb/ploy/internal/workflow/environments"
	"github.com/iw2rmb/ploy/internal/workflow/grid"
	"github.com/iw2rmb/ploy/internal/workflow/knowledgebase"
	"github.com/iw2rmb/ploy/internal/workflow/lanes"
	"github.com/iw2rmb/ploy/internal/workflow/manifests"
	"github.com/iw2rmb/ploy/internal/workflow/mods"
	"github.com/iw2rmb/ploy/internal/workflow/runner"
	"github.com/iw2rmb/ploy/internal/workflow/snapshots"
)

type runnerInvoker interface {
	Run(ctx context.Context, opts runner.Options) error
}

type runnerInvokerFunc func(context.Context, runner.Options) error

// Run executes the injected runner function implementation.
func (f runnerInvokerFunc) Run(ctx context.Context, opts runner.Options) error {
	return f(ctx, opts)
}

type eventsFactoryFunc func(tenant string) (runner.EventsClient, error)

type gridFactoryFunc func() (runner.GridClient, error)

type laneRegistry interface {
	Describe(name string, opts lanes.DescribeOptions) (lanes.Description, error)
}

type laneRegistryLoaderFunc func(dir string) (laneRegistry, error)

type knowledgeBaseAdvisorLoaderFunc func(path string) (mods.Advisor, error)

type snapshotRegistry interface {
	Plan(ctx context.Context, name string) (snapshots.PlanReport, error)
	Capture(ctx context.Context, name string, opts snapshots.CaptureOptions) (snapshots.CaptureResult, error)
}

type snapshotRegistryLoaderFunc func(dir string) (snapshotRegistry, error)

type manifestCompilerLoaderFunc func(dir string) (runner.ManifestCompiler, error)

type environmentService interface {
	Materialize(ctx context.Context, req environments.Request) (environments.Result, error)
}

type environmentFactoryFunc func(l laneRegistry, s snapshotRegistry) (environmentService, error)

type asterLocatorLoaderFunc func(dir string) (aster.Locator, error)

type laneCacheComposer struct {
	lanes laneRegistry
}

// Compose produces cache keys by delegating to the configured lane registry.
func (c laneCacheComposer) Compose(ctx context.Context, req runner.CacheComposeRequest) (string, error) {
	_ = ctx
	if c.lanes == nil {
		return "", fmt.Errorf("lane registry unavailable")
	}
	manifestVersion := req.Stage.Constraints.Manifest.Manifest.Version
	desc, err := c.lanes.Describe(req.Stage.Lane, lanes.DescribeOptions{
		ManifestVersion: manifestVersion,
		AsterToggles:    req.Stage.Aster.Toggles,
	})
	if err != nil {
		return "", err
	}
	return desc.CacheKey, nil
}

var (
	runnerExecutor runnerInvoker     = runnerInvokerFunc(runner.Run)
	eventsFactory  eventsFactoryFunc = defaultEventsFactory
	gridFactory    gridFactoryFunc   = defaultGridFactory
)

var (
	knowledgeBaseAdvisorLoader knowledgeBaseAdvisorLoaderFunc = defaultKnowledgeBaseAdvisorLoader
)

// defaultEventsFactory builds an events client, preferring JetStream when configured.
func defaultEventsFactory(tenant string) (runner.EventsClient, error) {
	trimmedTenant := strings.TrimSpace(tenant)
	if trimmedTenant == "" {
		return nil, fmt.Errorf("tenant is required for events client")
	}
	jetstreamURL := strings.TrimSpace(os.Getenv("JETSTREAM_URL"))
	if jetstreamURL != "" {
		client, err := contracts.NewJetStreamClient(contracts.JetStreamOptions{
			URL:    jetstreamURL,
			Tenant: trimmedTenant,
		})
		if err != nil {
			return nil, err
		}
		return client, nil
	}
	return contracts.NewInMemoryBus(trimmedTenant), nil
}

// defaultGridFactory returns either an in-memory grid client or the configured endpoint client.
func defaultGridFactory() (runner.GridClient, error) {
	endpoint := strings.TrimSpace(os.Getenv("GRID_ENDPOINT"))
	if endpoint == "" {
		return runner.NewInMemoryGrid(), nil
	}
	client, err := grid.NewClient(grid.Options{Endpoint: endpoint})
	if err != nil {
		return nil, err
	}
	return client, nil
}

var (
	laneRegistryLoader laneRegistryLoaderFunc = func(dir string) (laneRegistry, error) {
		return lanes.LoadDirectory(dir)
	}
	laneConfigDir = "configs/lanes"

	snapshotRegistryLoader snapshotRegistryLoaderFunc = func(dir string) (snapshotRegistry, error) {
		opts := snapshots.LoadOptions{}
		gateway := strings.TrimSpace(os.Getenv("IPFS_GATEWAY"))
		if gateway != "" {
			publisher, err := snapshots.NewIPFSGatewayPublisher(gateway, snapshots.IPFSGatewayOptions{Pin: true})
			if err != nil {
				return nil, err
			}
			opts.ArtifactPublisher = publisher
		}
		jetstreamURL := strings.TrimSpace(os.Getenv("JETSTREAM_URL"))
		if jetstreamURL != "" {
			metadataPublisher, err := snapshots.NewJetStreamMetadataPublisher(jetstreamURL, snapshots.JetStreamMetadataOptions{})
			if err != nil {
				return nil, err
			}
			opts.MetadataPublisher = metadataPublisher
		}
		return snapshots.LoadDirectory(dir, opts)
	}
	snapshotConfigDir = "configs/snapshots"

	manifestRegistryLoader manifestCompilerLoaderFunc = func(dir string) (runner.ManifestCompiler, error) {
		registry, err := manifests.LoadDirectory(dir)
		if err != nil {
			return nil, err
		}
		return registryCompiler{registry: registry}, nil
	}
	manifestConfigDir = "configs/manifests"

	knowledgeBaseCatalogPath = "configs/knowledge-base/catalog.json"

	asterLocatorLoader asterLocatorLoaderFunc = func(dir string) (aster.Locator, error) {
		return aster.NewFilesystemLocator(dir)
	}
	asterConfigDir = "configs/aster"

	environmentServiceFactory environmentFactoryFunc = func(l laneRegistry, s snapshotRegistry) (environmentService, error) {
		if l == nil {
			return nil, fmt.Errorf("environment lane registry missing")
		}
		if s == nil {
			return nil, fmt.Errorf("environment snapshot registry missing")
		}
		hydrator := environments.NewInMemoryHydrator()
		return environments.NewService(environments.ServiceOptions{
			Lanes:     l,
			Snapshots: s,
			Hydrator:  hydrator,
		}), nil
	}

	manifestSchemaPath = "docs/schemas/integration_manifest.schema.json"
)

// defaultKnowledgeBaseAdvisorLoader constructs a knowledge base advisor when a catalog is available.
func defaultKnowledgeBaseAdvisorLoader(path string) (mods.Advisor, error) {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return nil, nil
	}
	catalog, err := knowledgebase.LoadCatalogFile(trimmed)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	advisor, err := knowledgebase.NewAdvisor(knowledgebase.Options{Catalog: catalog})
	if err != nil {
		return nil, err
	}
	return advisor, nil
}

type registryCompiler struct {
	registry *manifests.Registry
}

// Compile resolves a manifest reference using the loaded registry instance.
func (r registryCompiler) Compile(ctx context.Context, ref contracts.ManifestReference) (manifests.Compilation, error) {
	_ = ctx
	if r.registry == nil {
		return manifests.Compilation{}, fmt.Errorf("compile manifest: registry missing")
	}
	comp, err := r.registry.Compile(manifests.CompileOptions{Name: ref.Name, Version: ref.Version})
	if err != nil {
		return manifests.Compilation{}, err
	}
	return comp, nil
}
