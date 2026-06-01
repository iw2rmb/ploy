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

func runRootDir(runID types.RunID) string {
	return runCacheDir(runID)
}

func runWorkspaceDir(runID types.RunID) string {
	root := runRootDir(runID)
	if root == "" {
		return ""
	}
	return filepath.Join(root, "workspace")
}

func runArtifactsDir(runID types.RunID) string {
	root := runRootDir(runID)
	if root == "" {
		return ""
	}
	return filepath.Join(root, "artifacts")
}

func runSharedArtifactsDir(runID types.RunID) string {
	artifactsDir := runArtifactsDir(runID)
	if artifactsDir == "" {
		return ""
	}
	return filepath.Join(artifactsDir, "shared")
}

func runJobArtifactsDir(runID types.RunID, jobID types.JobID) string {
	artifactsDir := runArtifactsDir(runID)
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

func runJobArtifactPaths(runID types.RunID, jobID types.JobID) jobArtifactPaths {
	root := runJobArtifactsDir(runID, jobID)
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

func ensureRunShareDir(runID types.RunID) (string, error) {
	shareDir := runSharedArtifactsDir(runID)
	if strings.TrimSpace(shareDir) == "" {
		return "", nil
	}
	if err := os.MkdirAll(shareDir, 0o750); err != nil {
		return "", fmt.Errorf("create run shared artifacts dir: %w", err)
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

func runJobOutFile(runID types.RunID, jobID types.JobID, outPath string) (string, error) {
	normalizedOutPath := path.Clean(strings.TrimSpace(outPath))
	if !strings.HasPrefix(normalizedOutPath, "/out/") || normalizedOutPath == "/out" {
		return "", fmt.Errorf("source path must stay under /out")
	}
	paths := runJobArtifactPaths(runID, jobID)
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
