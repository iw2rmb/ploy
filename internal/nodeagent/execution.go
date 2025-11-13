package nodeagent

import (
	"github.com/iw2rmb/ploy/internal/worker/hydration"
	"github.com/iw2rmb/ploy/internal/workflow/runtime/step"
)

// Runtime component factory methods.
// These methods isolate component initialization logic from the orchestration flow.

// createGitFetcher initializes a git fetcher for repository operations.
func (r *runController) createGitFetcher() (step.GitFetcher, error) {
	return hydration.NewGitFetcher(hydration.GitFetcherOptions{PublishSnapshot: false})
}

// createWorkspaceHydrator initializes a workspace hydrator with the provided repo fetcher.
func (r *runController) createWorkspaceHydrator(fetcher step.GitFetcher) (step.WorkspaceHydrator, error) {
	return step.NewFilesystemWorkspaceHydrator(step.FilesystemWorkspaceHydratorOptions{
		RepoFetcher: fetcher,
	})
}

// createContainerRuntime initializes a Docker container runtime with image pull enabled.
func (r *runController) createContainerRuntime() (step.ContainerRuntime, error) {
	return step.NewDockerContainerRuntime(step.DockerContainerRuntimeOptions{
		PullImage: true,
	})
}

// createDiffGenerator initializes a filesystem diff generator.
func (r *runController) createDiffGenerator() step.DiffGenerator {
	return step.NewFilesystemDiffGenerator(step.FilesystemDiffGeneratorOptions{})
}
