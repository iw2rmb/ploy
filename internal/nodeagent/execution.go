package nodeagent

import (
	"os"

	"github.com/iw2rmb/ploy/internal/worker/hydration"
	"github.com/iw2rmb/ploy/internal/workflow/runtime/step"
)

// Runtime component factory methods.
// These methods isolate component initialization logic from the orchestration flow.

// createGitFetcher initializes a git fetcher for repository operations.
// When PLOYD_CACHE_HOME is set, the fetcher uses it as the base directory for
// caching git clones, avoiding repeated network fetches for the same repo/ref/commit.
func (r *runController) createGitFetcher() (step.GitFetcher, error) {
	cacheDir := os.Getenv("PLOYD_CACHE_HOME")
	return hydration.NewGitFetcher(hydration.GitFetcherOptions{
		PublishSnapshot: false,
		CacheDir:        cacheDir,
	})
}

// createWorkspaceHydrator initializes a workspace hydrator with the provided repo fetcher.
func (r *runController) createWorkspaceHydrator(fetcher step.GitFetcher) (step.WorkspaceHydrator, error) {
	return step.NewFilesystemWorkspaceHydrator(step.FilesystemWorkspaceHydratorOptions{
		RepoFetcher: fetcher,
	})
}

// createContainerRuntime initializes a Docker container runtime with image pull enabled.
func (r *runController) createContainerRuntime() (step.ContainerRuntime, error) {
	network := os.Getenv("PLOY_DOCKER_NETWORK")
	return step.NewDockerContainerRuntime(step.DockerContainerRuntimeOptions{
		PullImage: true,
		Network:   network,
	})
}

// createDiffGenerator initializes a filesystem diff generator.
func (r *runController) createDiffGenerator() step.DiffGenerator {
	return step.NewFilesystemDiffGenerator(step.FilesystemDiffGeneratorOptions{})
}
