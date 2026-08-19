package nodeagent

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	types "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/workflow/step"
)

const (
	nodeCacheRootEnv     = "PLOY_NODE_CACHE_ROOT"
	nodeJobConfigRootEnv = "PLOY_NODE_JOB_CONFIG_ROOT"
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
	if runID.IsZero() {
		return ""
	}
	return filepath.Join(cacheRootDir(), runID.String())
}

func workspaceDir(runID types.RunID) string {
	root := runDir(runID)
	if root == "" {
		return ""
	}
	return filepath.Join(root, "workspace")
}

func jobsDir(runID types.RunID) string {
	root := runDir(runID)
	if root == "" {
		return ""
	}
	return filepath.Join(root, "jobs")
}

func runShareDir(runID types.RunID) string {
	root := runDir(runID)
	if root == "" {
		return ""
	}
	return filepath.Join(root, "share")
}

func runRuntimeShareDir(runID types.RunID) string {
	root := runDir(runID)
	if root == "" {
		return ""
	}
	return filepath.Join(root, "runtime-share")
}

// JobDirectories is the only authority for job-owned host storage.
type JobDirectories struct {
	Root             string
	Cache            string
	Home             string
	In               string
	Out              string
	Staging          string
	Tmp              string
	Stdout           string
	Stderr           string
	Diff             string
	ContainerInspect string
}

func jobDirectories(runID types.RunID, jobID types.JobID) JobDirectories {
	root := jobsDir(runID)
	if root == "" || jobID.IsZero() {
		return JobDirectories{}
	}
	root = filepath.Join(root, jobID.String())
	return JobDirectories{
		Root:             root,
		Cache:            filepath.Join(root, "cache"),
		Home:             filepath.Join(root, "home"),
		In:               filepath.Join(root, "in"),
		Out:              filepath.Join(root, "out"),
		Staging:          filepath.Join(root, "staging"),
		Tmp:              filepath.Join(root, "tmp"),
		Stdout:           filepath.Join(root, "stdout.log"),
		Stderr:           filepath.Join(root, "stderr.log"),
		Diff:             filepath.Join(root, "diff.patch"),
		ContainerInspect: filepath.Join(root, "container.inspect.json"),
	}
}

func jobMounts(dirs JobDirectories, runID types.RunID, jobType types.JobType) (step.JobMounts, error) {
	if err := validateJobDirectories(dirs); err != nil {
		return step.JobMounts{}, err
	}
	if err := jobType.Validate(); err != nil {
		return step.JobMounts{}, err
	}
	nodeCacheRoot := strings.TrimSpace(os.Getenv(nodeCacheRootEnv))
	if nodeCacheRoot == "" {
		return step.JobMounts{}, fmt.Errorf("%s is required", nodeCacheRootEnv)
	}
	nodeConfigRoot := strings.TrimSpace(os.Getenv(nodeJobConfigRootEnv))
	if nodeConfigRoot == "" {
		return step.JobMounts{}, fmt.Errorf("%s is required", nodeJobConfigRootEnv)
	}
	return step.JobMounts{
		Cache:        dirs.Cache,
		Home:         dirs.Home,
		In:           dirs.In,
		Out:          dirs.Out,
		Staging:      dirs.Staging,
		Tmp:          dirs.Tmp,
		Share:        runShareDir(runID),
		RuntimeShare: runRuntimeShareDir(runID),
		NodeCache:    nodeCacheRoot,
		CommonConfig: filepath.Join(nodeConfigRoot, "common"),
		JobConfig:    filepath.Join(nodeConfigRoot, jobType.String()),
		JobType:      jobType,
	}, nil
}

func ensureRunDirectories(runID types.RunID) error {
	for _, dir := range []string{runShareDir(runID), runRuntimeShareDir(runID)} {
		if strings.TrimSpace(dir) == "" {
			return fmt.Errorf("run directory path is empty")
		}
		if err := os.MkdirAll(dir, 0o750); err != nil {
			return fmt.Errorf("create run directory %s: %w", dir, err)
		}
	}
	return nil
}

func ensureJobDirectories(dirs JobDirectories) error {
	if err := validateJobDirectories(dirs); err != nil {
		return err
	}
	for _, dir := range []string{dirs.Cache, dirs.Home, dirs.In, dirs.Out, dirs.Staging, dirs.Tmp} {
		if err := os.MkdirAll(dir, 0o750); err != nil {
			return fmt.Errorf("create job directory %s: %w", dir, err)
		}
	}
	if err := os.Chmod(dirs.Tmp, 0o1777); err != nil {
		return fmt.Errorf("set job tmp permissions: %w", err)
	}
	return nil
}

func validateJobDirectories(dirs JobDirectories) error {
	root := filepath.Clean(strings.TrimSpace(dirs.Root))
	if root == "." || root == string(filepath.Separator) {
		return fmt.Errorf("job root path is empty or unsafe")
	}
	expected := map[string]string{
		"cache":                  dirs.Cache,
		"home":                   dirs.Home,
		"in":                     dirs.In,
		"out":                    dirs.Out,
		"staging":                dirs.Staging,
		"tmp":                    dirs.Tmp,
		"stdout.log":             dirs.Stdout,
		"stderr.log":             dirs.Stderr,
		"diff.patch":             dirs.Diff,
		"container.inspect.json": dirs.ContainerInspect,
	}
	for name, got := range expected {
		if filepath.Clean(strings.TrimSpace(got)) != filepath.Join(root, name) {
			return fmt.Errorf("job path %s is outside its declared root", name)
		}
	}
	return nil
}

func cleanupJobRuntime(dirs JobDirectories) error {
	if err := validateJobDirectories(dirs); err != nil {
		return err
	}
	var cleanupErr error
	for _, dir := range []string{dirs.Cache, dirs.Home, dirs.Staging, dirs.Tmp} {
		if err := os.RemoveAll(dir); err != nil {
			cleanupErr = errors.Join(cleanupErr, fmt.Errorf("remove job runtime directory %s: %w", dir, err))
		}
	}
	return cleanupErr
}
