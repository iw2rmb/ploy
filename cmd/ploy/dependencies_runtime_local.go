package main

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/iw2rmb/ploy/internal/workflow/artifacts"
	"github.com/iw2rmb/ploy/internal/workflow/buildgate"
    javaexec "github.com/iw2rmb/ploy/internal/workflow/buildgate/javaexec"
	"github.com/iw2rmb/ploy/internal/workflow/runner"
	"github.com/iw2rmb/ploy/internal/workflow/runtime"
	"github.com/iw2rmb/ploy/internal/workflow/runtime/step"
)

const (
	clusterURLEnv     = "PLOY_IPFS_CLUSTER_API"
	clusterTokenEnv   = "PLOY_IPFS_CLUSTER_TOKEN"
	clusterUserEnv    = "PLOY_IPFS_CLUSTER_USERNAME"
	clusterPassEnv    = "PLOY_IPFS_CLUSTER_PASSWORD"
	clusterReplMinEnv = "PLOY_IPFS_CLUSTER_REPL_MIN"
	clusterReplMaxEnv = "PLOY_IPFS_CLUSTER_REPL_MAX"
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

func (localRuntimeAdapter) Connect(ctx context.Context) (runner.RuntimeClient, error) {
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
	client, err := newClusterArtifactClient()
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
    gateClient, err := newBuildGateClient()
    if err != nil {
        return nil, err
    }
    return step.Runner{
        Workspace:  hydrator,
        Containers: containerRuntime,
        Diffs:      diffGenerator,
        Gate:       gateClient,
        Artifacts:  publisher,
    }, nil
}

func newClusterArtifactClient() (*artifacts.ClusterClient, error) {
	baseURL := strings.TrimSpace(os.Getenv(clusterURLEnv))
	if baseURL == "" {
		return nil, fmt.Errorf("configure cluster client: %s required", clusterURLEnv)
	}
	opts := artifacts.ClusterClientOptions{
		BaseURL:              baseURL,
		AuthToken:            strings.TrimSpace(os.Getenv(clusterTokenEnv)),
		BasicAuthUsername:    strings.TrimSpace(os.Getenv(clusterUserEnv)),
		BasicAuthPassword:    strings.TrimSpace(os.Getenv(clusterPassEnv)),
		ReplicationFactorMin: parseEnvInt(clusterReplMinEnv),
		ReplicationFactorMax: parseEnvInt(clusterReplMaxEnv),
	}
	return artifacts.NewClusterClient(opts)
}

func parseEnvInt(name string) int {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return 0
	}
	num, err := strconv.Atoi(value)
	if err != nil {
		return 0
	}
	return num
}

func newBuildGateClient() (step.GateClient, error) {
    executor, err := javaexec.NewExecutor(javaexec.Options{})
    if err != nil {
        return nil, err
    }
    sandbox := buildgate.NewSandboxRunner(executor, buildgate.SandboxRunnerOptions{})
    gateRunner := &buildgate.Runner{Sandbox: sandbox}
    return step.NewBuildGateClient(step.BuildGateClientOptions{Runner: gateRunner})
}
