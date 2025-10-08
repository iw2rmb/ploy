package main

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/iw2rmb/ploy/internal/workflow/aster"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
	"github.com/iw2rmb/ploy/internal/workflow/environments"
	"github.com/iw2rmb/ploy/internal/workflow/knowledgebase"
	"github.com/iw2rmb/ploy/internal/workflow/lanes"
	"github.com/iw2rmb/ploy/internal/workflow/manifests"
	"github.com/iw2rmb/ploy/internal/workflow/mods"
	"github.com/iw2rmb/ploy/internal/workflow/runner"
	"github.com/iw2rmb/ploy/internal/workflow/snapshots"
)

var (
	laneRegistryLoader laneRegistryLoaderFunc = loadLaneRegistry
	laneConfigDir                             = ""

	snapshotRegistryLoader snapshotRegistryLoaderFunc = loadSnapshotRegistry
	snapshotConfigDir                                 = "configs/snapshots"

	manifestRegistryLoader manifestCompilerLoaderFunc = loadManifestCompiler
	manifestConfigDir                                 = "configs/manifests"

	knowledgeBaseCatalogPath = "configs/knowledge-base/catalog.json"

	asterLocatorLoader asterLocatorLoaderFunc = loadAsterLocator
	asterConfigDir                            = "configs/aster"

	environmentServiceFactory environmentFactoryFunc = newEnvironmentService

	manifestSchemaPath = "docs/schemas/integration_manifest.schema.json"
)

func loadLaneRegistry(dir string) (laneRegistry, error) {
	trimmed := strings.TrimSpace(dir)
	if trimmed != "" && !strings.EqualFold(trimmed, "auto") {
		return lanes.LoadDirectory(trimmed)
	}

	candidates := resolveLaneDirectories("")
	if len(candidates) == 0 {
		return nil, fmt.Errorf("lane catalog path not configured; set %s or configure GRID_* environment variables", lanesCatalogEnv)
	}

	var missing []string
	for _, candidate := range candidates {
		registry, err := lanes.LoadDirectory(candidate)
		if err == nil {
			return registry, nil
		}
		if errors.Is(err, os.ErrNotExist) || errors.Is(err, fs.ErrNotExist) {
			missing = append(missing, candidate)
			continue
		}
		return nil, err
	}

	return nil, fmt.Errorf("no lane definitions found (searched %v); set %s or configure GRID_* environment variables", missing, lanesCatalogEnv)
}

func loadSnapshotRegistry(dir string) (snapshotRegistry, error) {
	opts := snapshots.LoadOptions{}
	cfg, _ := resolveIntegrationConfig(context.Background())
	gateway := strings.TrimSpace(cfg.IPFSGateway)
	if gateway != "" {
		publisher, err := snapshots.NewIPFSGatewayPublisher(gateway, snapshots.IPFSGatewayOptions{Pin: true})
		if err != nil {
			return nil, err
		}
		opts.ArtifactPublisher = publisher
	}
	jetstreamURL := strings.TrimSpace(cfg.JetStreamURL)
	if jetstreamURL != "" {
		metadataPublisher, err := snapshots.NewJetStreamMetadataPublisher(jetstreamURL, snapshots.JetStreamMetadataOptions{})
		if err != nil {
			return nil, err
		}
		opts.MetadataPublisher = metadataPublisher
	}
	return snapshots.LoadDirectory(dir, opts)
}

func loadManifestCompiler(dir string) (runner.ManifestCompiler, error) {
	registry, err := manifests.LoadDirectory(dir)
	if err != nil {
		return nil, err
	}
	return registryCompiler{registry: registry}, nil
}

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

func loadAsterLocator(dir string) (aster.Locator, error) {
	return aster.NewFilesystemLocator(dir)
}

func newEnvironmentService(l laneRegistry, s snapshotRegistry) (environmentService, error) {
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

func resolveLaneDirectories(dir string) []string {
	trimmed := strings.TrimSpace(dir)
	if trimmed != "" && !strings.EqualFold(trimmed, "auto") {
		return []string{trimmed}
	}

	seen := map[string]struct{}{}
	var candidates []string

	add := func(path string) {
		clean := strings.TrimSpace(path)
		if clean == "" {
			return
		}
		abs := filepath.Clean(clean)
		if _, exists := seen[abs]; exists {
			return
		}
		seen[abs] = struct{}{}
		candidates = append(candidates, abs)
	}

	for _, part := range filepath.SplitList(os.Getenv(lanesCatalogEnv)) {
		add(part)
	}

	gridID := sanitizePathComponent(os.Getenv(gridIDEnv))

	if configDir, err := os.UserConfigDir(); err == nil {
		base := filepath.Join(configDir, "ploy", "lanes")
		add(base)
		if gridID != "" {
			add(filepath.Join(base, gridID))
		}
	}
	if homeDir, err := os.UserHomeDir(); err == nil {
		base := filepath.Join(homeDir, ".ploy", "lanes")
		add(base)
		if gridID != "" {
			add(filepath.Join(base, gridID))
		}
	}
	if wd, err := os.Getwd(); err == nil {
		add(filepath.Join(wd, "..", "ploy-lanes-catalog"))
	}
	if exePath, err := os.Executable(); err == nil {
		exeDir := filepath.Dir(exePath)
		add(filepath.Join(exeDir, "..", "ploy-lanes-catalog"))
	}

	return candidates
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
