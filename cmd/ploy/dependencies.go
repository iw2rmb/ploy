//go:build legacy
// +build legacy

package main

import (
	"context"

	"github.com/iw2rmb/ploy/internal/workflow/aster"
	"github.com/iw2rmb/ploy/internal/workflow/environments"
	"github.com/iw2rmb/ploy/internal/workflow/runner"
)

type manifestCompilerLoaderFunc func(dir string) (runner.ManifestCompiler, error)

type environmentService interface {
	Materialize(ctx context.Context, req environments.Request) (environments.Result, error)
}

type environmentFactoryFunc func() (environmentService, error)

type asterLocatorLoaderFunc func(dir string) (aster.Locator, error)

type workspacePreparerFactoryFunc func() (runner.WorkspacePreparer, error)

const (
	runtimeAdapterEnv = "PLOY_RUNTIME_ADAPTER"
)

var (
	workspacePreparerFactory workspacePreparerFactoryFunc = defaultWorkspacePreparerFactory
)
