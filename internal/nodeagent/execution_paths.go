package nodeagent

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	types "github.com/iw2rmb/ploy/internal/domain/types"
)

// runCacheRootDir returns the durable run cache root under PLOYD_CACHE_HOME.
func runCacheRootDir() string {
	baseRoot := os.Getenv("PLOYD_CACHE_HOME")
	if baseRoot == "" {
		baseRoot = os.TempDir()
	}
	return filepath.Join(baseRoot, "runs")
}

func runCacheDir(runID types.RunID) string {
	return filepath.Join(runCacheRootDir(), runID.String())
}

func runRepoRootDir(runID types.RunID, repoID types.MigRepoID) string {
	if repoID.IsZero() {
		return ""
	}
	return filepath.Join(runCacheDir(runID), "repos", repoID.String())
}

func runRepoWorkspaceDir(runID types.RunID, repoID types.MigRepoID) string {
	root := runRepoRootDir(runID, repoID)
	if root == "" {
		return ""
	}
	return filepath.Join(root, "workspace")
}

func runRepoArtifactsDir(runID types.RunID, repoID types.MigRepoID) string {
	root := runRepoRootDir(runID, repoID)
	if root == "" {
		return ""
	}
	return filepath.Join(root, "artifacts")
}

func runRepoSharedArtifactsDir(runID types.RunID, repoID types.MigRepoID) string {
	artifactsDir := runRepoArtifactsDir(runID, repoID)
	if artifactsDir == "" {
		return ""
	}
	return filepath.Join(artifactsDir, "shared")
}

func runRepoJobArtifactsDir(runID types.RunID, repoID types.MigRepoID, jobID types.JobID) string {
	artifactsDir := runRepoArtifactsDir(runID, repoID)
	if artifactsDir == "" || jobID.IsZero() {
		return ""
	}
	return filepath.Join(artifactsDir, jobID.String())
}

type jobArtifactPaths struct {
	Root   string
	In     string
	Out    string
	Stdout string
	Stderr string
	Diff   string
}

func runRepoJobArtifactPaths(runID types.RunID, repoID types.MigRepoID, jobID types.JobID) jobArtifactPaths {
	root := runRepoJobArtifactsDir(runID, repoID, jobID)
	if root == "" {
		return jobArtifactPaths{}
	}
	return jobArtifactPaths{
		Root:   root,
		In:     filepath.Join(root, "in"),
		Out:    filepath.Join(root, "out"),
		Stdout: filepath.Join(root, "stdout.log"),
		Stderr: filepath.Join(root, "stderr.log"),
		Diff:   filepath.Join(root, "diff.patch"),
	}
}

func ensureRunRepoShareDir(runID types.RunID, repoID types.MigRepoID) (string, error) {
	shareDir := runRepoSharedArtifactsDir(runID, repoID)
	if strings.TrimSpace(shareDir) == "" {
		return "", nil
	}
	if err := os.MkdirAll(shareDir, 0o750); err != nil {
		return "", fmt.Errorf("create run/repo shared artifacts dir: %w", err)
	}
	return shareDir, nil
}

func ensureJobArtifactDirs(paths jobArtifactPaths) error {
	if strings.TrimSpace(paths.Root) == "" {
		return fmt.Errorf("job artifacts path is empty")
	}
	for _, dir := range []string{paths.In, paths.Out} {
		if err := os.MkdirAll(dir, 0o750); err != nil {
			return fmt.Errorf("create job artifact dir %s: %w", dir, err)
		}
	}
	return nil
}

func runRepoJobOutFile(runID types.RunID, repoID types.MigRepoID, jobID types.JobID, outPath string) (string, error) {
	normalizedOutPath := path.Clean(strings.TrimSpace(outPath))
	if !strings.HasPrefix(normalizedOutPath, "/out/") || normalizedOutPath == "/out" {
		return "", fmt.Errorf("source path must stay under /out")
	}
	paths := runRepoJobArtifactPaths(runID, repoID, jobID)
	if strings.TrimSpace(paths.Out) == "" {
		return "", fmt.Errorf("source job artifacts path is empty")
	}
	rel := strings.TrimPrefix(normalizedOutPath, "/out/")
	sourcePath := filepath.Clean(filepath.Join(paths.Out, filepath.FromSlash(rel)))
	cleanOutDir := filepath.Clean(paths.Out)
	if sourcePath != cleanOutDir && !strings.HasPrefix(sourcePath, cleanOutDir+string(filepath.Separator)) {
		return "", fmt.Errorf("source path escapes source /out")
	}
	return sourcePath, nil
}
