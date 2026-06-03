package nodeagent

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	types "github.com/iw2rmb/ploy/internal/domain/types"
)

// cacheRootDir returns the durable run cache root under PLOYD_CACHE_HOME.
func cacheRootDir() string {
	baseRoot := os.Getenv("PLOYD_CACHE_HOME")
	if baseRoot == "" {
		baseRoot = os.TempDir()
	}
	return filepath.Join(baseRoot, "runs")
}

func runDir(runID types.RunID) string {
	return filepath.Join(cacheRootDir(), runID.String())
}

func workspaceDir(runID types.RunID) string {
	root := runDir(runID)
	if root == "" {
		return ""
	}
	return filepath.Join(root, "workspace")
}

func artifactsDir(runID types.RunID) string {
	root := runDir(runID)
	if root == "" {
		return ""
	}
	return filepath.Join(root, "artifacts")
}

func sharedArtifactsDir(runID types.RunID) string {
	artifactsDir := artifactsDir(runID)
	if artifactsDir == "" {
		return ""
	}
	return filepath.Join(artifactsDir, "shared")
}

func jobArtifactsDir(runID types.RunID, jobID types.JobID) string {
	artifactsDir := artifactsDir(runID)
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

func artifactPaths(runID types.RunID, jobID types.JobID) jobArtifactPaths {
	root := jobArtifactsDir(runID, jobID)
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
	shareDir := sharedArtifactsDir(runID)
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

func jobOutFile(runID types.RunID, jobID types.JobID, outPath string) (string, error) {
	normalizedOutPath := path.Clean(strings.TrimSpace(outPath))
	if !strings.HasPrefix(normalizedOutPath, "/out/") || normalizedOutPath == "/out" {
		return "", fmt.Errorf("source path must stay under /out")
	}
	paths := artifactPaths(runID, jobID)
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
