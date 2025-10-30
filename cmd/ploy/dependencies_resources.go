package main

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"strings"

	"github.com/iw2rmb/ploy/internal/workflow/aster"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
	"github.com/iw2rmb/ploy/internal/workflow/environments"
	"github.com/iw2rmb/ploy/internal/workflow/knowledgebase"
	"github.com/iw2rmb/ploy/internal/workflow/manifests"
	"github.com/iw2rmb/ploy/internal/workflow/mods"
	"github.com/iw2rmb/ploy/internal/workflow/runner"
)


var (
	manifestRegistryLoader manifestCompilerLoaderFunc = loadManifestCompiler
	manifestConfigDir                                 = "configs/manifests"

	knowledgeBaseCatalogPath = "configs/knowledge-base/catalog.json"

	asterLocatorLoader asterLocatorLoaderFunc = loadAsterLocator
	asterConfigDir                            = "configs/aster"

	environmentServiceFactory environmentFactoryFunc = newEnvironmentService

	manifestSchemaPath = "docs/schemas/integration_manifest.schema.json"
)

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

func newEnvironmentService() (environmentService, error) {
    hydrator := environments.NewInMemoryHydrator()
    return environments.NewService(environments.ServiceOptions{Hydrator: hydrator}), nil
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
