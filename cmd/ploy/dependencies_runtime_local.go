package main

import (
	"context"
	"fmt"

	"github.com/iw2rmb/ploy/internal/workflow/artifacts"
	"github.com/iw2rmb/ploy/internal/workflow/buildgate"
	"github.com/iw2rmb/ploy/internal/workflow/runner"
	"github.com/iw2rmb/ploy/internal/workflow/runtime"
	"github.com/iw2rmb/ploy/internal/workflow/runtime/step"
)

type stepExecutorFactoryFunc func() (runtime.StepExecutor, error)

var (
	stepExecutorFactory stepExecutorFactoryFunc = defaultStepExecutorFactory
)

type localRuntimeAdapter struct{}

func newLocalRuntimeAdapter() runtime.Adapter {
	return &localRuntimeAdapter{}
}

func (localRuntimeAdapter) Metadata() runtime.AdapterMetadata {
	return runtime.AdapterMetadata{
		Name:        "local-step",
		Aliases:     []string{"local"},
		Description: "Local step manifest runtime backed by Docker",
	}
}

func (localRuntimeAdapter) Connect(ctx context.Context) (runner.GridClient, error) {
	if stepExecutorFactory == nil {
		return nil, fmt.Errorf("configure local runtime: executor factory missing")
	}
	executor, err := stepExecutorFactory()
	// ctx unused but kept for symmetry
	_ = ctx
	if err != nil {
		return nil, err
	}
	client, err := runtime.NewLocalStepClient(runtime.LocalStepClientOptions{Runner: executor})
	if err != nil {
		return nil, err
	}
	return client, nil
}

func defaultStepExecutorFactory() (runtime.StepExecutor, error) {
	hydrator, err := step.NewFilesystemWorkspaceHydrator(step.FilesystemWorkspaceHydratorOptions{})
	if err != nil {
		return nil, err
	}
	diffGenerator := step.NewFilesystemDiffGenerator(step.FilesystemDiffGeneratorOptions{})
	client, err := artifactClientFactory()
	if err != nil {
		return nil, err
	}
	publisher, err := artifacts.NewClusterPublisher(artifacts.ClusterPublisherOptions{
		Client: client,
	})
	if err != nil {
		return nil, err
	}
	containerRuntime, err := step.NewDockerContainerRuntime(step.DockerContainerRuntimeOptions{PullImage: true})
	if err != nil {
		return nil, err
	}
	shiftClient, err := newBuildGateShiftClient()
	if err != nil {
		return nil, err
	}
	return step.Runner{
		Workspace:  hydrator,
		Containers: containerRuntime,
		Diffs:      diffGenerator,
		SHIFT:      shiftClient,
		Artifacts:  publisher,
	}, nil
}

func newBuildGateShiftClient() (step.ShiftClient, error) {
	sandbox := buildgate.NewSandboxRunner(noopSandboxExecutor{}, buildgate.SandboxRunnerOptions{})
	gateRunner := &buildgate.Runner{
		Sandbox: sandbox,
	}
	return step.NewBuildGateShiftClient(step.BuildGateShiftOptions{Runner: gateRunner})
}

type noopSandboxExecutor struct{}

func (noopSandboxExecutor) Execute(ctx context.Context, spec buildgate.SandboxSpec) (buildgate.SandboxBuildResult, error) {
	_ = ctx
	_ = spec
	return buildgate.SandboxBuildResult{
		Success:  true,
		CacheHit: false,
	}, nil
}
