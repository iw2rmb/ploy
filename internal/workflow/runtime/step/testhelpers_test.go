package step

import (
	"context"

	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

// testContainerRuntime is a configurable mock for ContainerRuntime.
// Fields can be set to customize behavior; nil fields use sensible defaults.
// Boolean tracking fields record whether each method was called.
type testContainerRuntime struct {
	createFn func(ctx context.Context, spec ContainerSpec) (ContainerHandle, error)
	startFn  func(ctx context.Context, handle ContainerHandle) error
	waitFn   func(ctx context.Context, handle ContainerHandle) (ContainerResult, error)
	logsFn   func(ctx context.Context, handle ContainerHandle) ([]byte, error)
	removeFn func(ctx context.Context, handle ContainerHandle) error

	// captured holds the last ContainerSpec passed to Create.
	captured     ContainerSpec
	createCalled bool
	startCalled  bool
	waitCalled   bool
	logsCalled   bool
	removeCalled bool
}

func (m *testContainerRuntime) Create(ctx context.Context, spec ContainerSpec) (ContainerHandle, error) {
	m.createCalled = true
	m.captured = spec
	if m.createFn != nil {
		return m.createFn(ctx, spec)
	}
	return ContainerHandle{ID: "mock"}, nil
}

func (m *testContainerRuntime) Start(ctx context.Context, handle ContainerHandle) error {
	m.startCalled = true
	if m.startFn != nil {
		return m.startFn(ctx, handle)
	}
	return nil
}

func (m *testContainerRuntime) Wait(ctx context.Context, handle ContainerHandle) (ContainerResult, error) {
	m.waitCalled = true
	if m.waitFn != nil {
		return m.waitFn(ctx, handle)
	}
	return ContainerResult{ExitCode: 0}, nil
}

func (m *testContainerRuntime) Logs(ctx context.Context, handle ContainerHandle) ([]byte, error) {
	m.logsCalled = true
	if m.logsFn != nil {
		return m.logsFn(ctx, handle)
	}
	return nil, nil
}

func (m *testContainerRuntime) Remove(ctx context.Context, handle ContainerHandle) error {
	m.removeCalled = true
	if m.removeFn != nil {
		return m.removeFn(ctx, handle)
	}
	return nil
}

// testGateExecutor is a configurable mock for GateExecutor.
type testGateExecutor struct {
	executeFn func(ctx context.Context, spec *contracts.StepGateSpec, workspace string) (*contracts.BuildGateStageMetadata, error)
}

func (m *testGateExecutor) Execute(ctx context.Context, spec *contracts.StepGateSpec, workspace string) (*contracts.BuildGateStageMetadata, error) {
	if m.executeFn != nil {
		return m.executeFn(ctx, spec, workspace)
	}
	return &contracts.BuildGateStageMetadata{
		StaticChecks: []contracts.BuildGateStaticCheckReport{{
			Tool:   "default",
			Passed: true,
		}},
	}, nil
}

// testWorkspaceHydrator is a configurable mock for WorkspaceHydrator.
type testWorkspaceHydrator struct {
	hydrateFn func(ctx context.Context, manifest contracts.StepManifest, workspace string) error
}

func (m *testWorkspaceHydrator) Hydrate(ctx context.Context, manifest contracts.StepManifest, workspace string) error {
	if m.hydrateFn != nil {
		return m.hydrateFn(ctx, manifest, workspace)
	}
	return nil
}

// testGitFetcher is a configurable mock for GitFetcher.
type testGitFetcher struct {
	fetchFn func(ctx context.Context, repo *contracts.RepoMaterialization, dest string) error
}

func (m *testGitFetcher) Fetch(ctx context.Context, repo *contracts.RepoMaterialization, dest string) error {
	if m.fetchFn != nil {
		return m.fetchFn(ctx, repo, dest)
	}
	return nil
}
