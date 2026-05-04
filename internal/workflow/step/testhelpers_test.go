package step

import (
	"context"
	"testing"

	types "github.com/iw2rmb/ploy/internal/domain/types"
	workspaceutil "github.com/iw2rmb/ploy/internal/testutil/workspace"
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
	return ContainerHandle("mock"), nil
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

// newGateTestManifest returns a StepManifest with a single read-only input and
// the given gate-enabled flag. Tests that need different fields can override
// after calling this helper.
func newGateTestManifest(gateEnabled bool) contracts.StepManifest {
	return contracts.StepManifest{
		ID:    types.StepID("test-step"),
		Name:  "Test Step",
		Image: "maven:jdk17",
		Inputs: []contracts.StepInput{{
			Name:        "source",
			MountPath:   "/workspace",
			Mode:        contracts.StepInputModeReadOnly,
			SnapshotCID: types.CID("bafytest123"),
		}},
		Gate: &contracts.StepGateSpec{
			Enabled: gateEnabled,
		},
	}
}

// newGateTestRequest wraps a manifest into a Request with a fixed workspace path.
func newGateTestRequest(m contracts.StepManifest) Request {
	return Request{
		Manifest:  m,
		Workspace: "/tmp/test-workspace",
	}
}

// newDockerGateTestHarness creates a DockerGateExecutor backed by a
// testContainerRuntime and a temporary Maven workspace. Returns the executor,
// the runtime (for assertions), and the workspace path.
func newDockerGateTestHarness(t *testing.T) (GateExecutor, *testContainerRuntime, string) {
	t.Helper()
	rt := &testContainerRuntime{}
	executor := NewDockerGateExecutor(rt)
	workspace := createMavenWorkspace(t, "17")
	return executor, rt, workspace
}

func createMavenWorkspace(t *testing.T, javaVersion string) string {
	t.Helper()
	return workspaceutil.Maven(t, javaVersion)
}

func createMavenWorkspaceNoJavaVersion(t *testing.T) string {
	t.Helper()
	return workspaceutil.MavenNoJavaVersion(t)
}

func createGradleWorkspace(t *testing.T, javaVersion string) string {
	t.Helper()
	return workspaceutil.Gradle(t, javaVersion)
}

func createGradleWorkspaceWithWrapper(t *testing.T, javaVersion string) string {
	t.Helper()
	return workspaceutil.GradleWithWrapper(t, javaVersion)
}

func createGoWorkspace(t *testing.T, goVersion string) string {
	t.Helper()
	return workspaceutil.Go(t, goVersion)
}

func createCargoWorkspace(t *testing.T, rustVersion string) string {
	t.Helper()
	return workspaceutil.Cargo(t, rustVersion)
}

func createPythonWorkspace(t *testing.T, pythonVersion string) string {
	t.Helper()
	return workspaceutil.Python(t, pythonVersion)
}
