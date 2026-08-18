package step

import (
	"fmt"
	"path"
	"strings"

	types "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

const (
	jobCacheContainerDir    = "/ploy/cache/job"
	nodeCacheContainerDir   = "/ploy/cache/node"
	nodeConfigContainerDir  = "/ploy/config/common"
	jobConfigContainerDir   = "/ploy/config/job"
	jobInContainerDir       = "/in"
	jobOutContainerDir      = "/out"
	jobTmpContainerDir      = "/tmp"
	jobShareContainerDir    = "/share"
	jobRuntimeContainerDir  = "/run-share"
	jobDefaultHomeContainer = "/root"
	nodeConfigContainerRoot = "/ploy/config"
)

const (
	ployJobCacheDirEnv        = "PLOY_JOB_CACHE_DIR"
	ployNodeCacheDirEnv       = "PLOY_NODE_CACHE_DIR"
	ployJobHomeDirEnv         = "PLOY_JOB_HOME_DIR"
	ployRunShareDirEnv        = "PLOY_RUN_SHARE_DIR"
	ployRunRuntimeShareDirEnv = "PLOY_RUN_RUNTIME_SHARE_DIR"
	ployNodeConfigDirEnv      = "PLOY_NODE_CONFIG_DIR"
	ployJobConfigDirEnv       = "PLOY_JOB_CONFIG_DIR"
	ployJobTypeEnv            = "PLOY_JOB_TYPE"
)

// JobMounts is the workflow boundary for job, run, and node host storage.
type JobMounts struct {
	Cache        string
	Home         string
	In           string
	Out          string
	Staging      string
	Tmp          string
	Share        string
	RuntimeShare string
	NodeCache    string
	CommonConfig string
	JobConfig    string
	JobType      types.JobType
}

type nestedMountContract struct {
	parent string
	child  string
}

func buildCommonJobMounts(jobMounts JobMounts, homeTarget string) ([]ContainerMount, error) {
	if err := validateJobMountSources(jobMounts); err != nil {
		return nil, err
	}
	if err := validateJobHomeTarget(homeTarget); err != nil {
		return nil, err
	}
	mounts := []ContainerMount{
		{Source: jobMounts.In, Target: jobInContainerDir, ReadOnly: true},
		{Source: jobMounts.Out, Target: jobOutContainerDir, ReadOnly: false},
		{Source: jobMounts.Tmp, Target: jobTmpContainerDir, ReadOnly: false},
		{Source: jobMounts.Cache, Target: jobCacheContainerDir, ReadOnly: false},
		{Source: jobMounts.Home, Target: homeTarget, ReadOnly: false},
		{Source: jobMounts.Share, Target: jobShareContainerDir, ReadOnly: false},
		{Source: jobMounts.RuntimeShare, Target: jobRuntimeContainerDir, ReadOnly: false},
		{Source: jobMounts.NodeCache, Target: nodeCacheContainerDir, ReadOnly: true},
		{Source: jobMounts.CommonConfig, Target: nodeConfigContainerDir, ReadOnly: true},
		{Source: jobMounts.JobConfig, Target: jobConfigContainerDir, ReadOnly: true},
	}
	if err := validateContainerMounts(mounts, nil); err != nil {
		return nil, err
	}
	return mounts, nil
}

func validateJobMountSources(jobMounts JobMounts) error {
	sources := []struct {
		name string
		path string
	}{
		{name: "cache", path: jobMounts.Cache},
		{name: "home", path: jobMounts.Home},
		{name: "in", path: jobMounts.In},
		{name: "out", path: jobMounts.Out},
		{name: "staging", path: jobMounts.Staging},
		{name: "tmp", path: jobMounts.Tmp},
		{name: "share", path: jobMounts.Share},
		{name: "runtime-share", path: jobMounts.RuntimeShare},
		{name: "node cache", path: jobMounts.NodeCache},
		{name: "common config", path: jobMounts.CommonConfig},
		{name: "job config", path: jobMounts.JobConfig},
	}
	for _, source := range sources {
		if strings.TrimSpace(source.path) == "" {
			return fmt.Errorf("job mount source %s is empty", source.name)
		}
	}
	if err := jobMounts.JobType.Validate(); err != nil {
		return fmt.Errorf("job mounts: %w", err)
	}
	return nil
}

func applyReservedJobEnv(base map[string]string, jobMounts JobMounts) (map[string]string, error) {
	if err := validateJobMountSources(jobMounts); err != nil {
		return nil, err
	}
	homeTarget, err := resolveJobHome(base)
	if err != nil {
		return nil, err
	}
	env := contracts.MergeEnv(base, map[string]string{
		ployJobCacheDirEnv:        jobCacheContainerDir,
		ployNodeCacheDirEnv:       nodeCacheContainerDir,
		ployJobHomeDirEnv:         homeTarget,
		ployRunShareDirEnv:        jobShareContainerDir,
		ployRunRuntimeShareDirEnv: jobRuntimeContainerDir,
		ployNodeConfigDirEnv:      nodeConfigContainerDir,
		ployJobConfigDirEnv:       jobConfigContainerDir,
		ployJobTypeEnv:            jobMounts.JobType.String(),
	})
	return env, nil
}

func resolveJobHome(env map[string]string) (string, error) {
	homeTarget := jobDefaultHomeContainer
	if configured := env["HOME"]; configured != "" {
		homeTarget = configured
	}
	if err := validateJobHomeTarget(homeTarget); err != nil {
		return "", err
	}
	return homeTarget, nil
}

func validateJobHomeTarget(homeTarget string) error {
	if !path.IsAbs(homeTarget) {
		return fmt.Errorf("job HOME target %q is not absolute", homeTarget)
	}
	if clean := path.Clean(homeTarget); clean != homeTarget {
		return fmt.Errorf("job HOME target %q is not canonical", homeTarget)
	}
	return nil
}

func validateContainerMounts(mounts []ContainerMount, nested []nestedMountContract) error {
	seen := make(map[string]struct{}, len(mounts))
	for _, mount := range mounts {
		if strings.TrimSpace(mount.Source) == "" {
			return fmt.Errorf("container mount source for target %q is empty", mount.Target)
		}
		target := strings.TrimSpace(mount.Target)
		if !path.IsAbs(target) {
			return fmt.Errorf("container mount target %q is not absolute", mount.Target)
		}
		if clean := path.Clean(target); clean != target {
			return fmt.Errorf("container mount target %q is not canonical", mount.Target)
		}
		if _, ok := seen[target]; ok {
			return fmt.Errorf("duplicate container mount target %q", target)
		}
		if isNodeOwnedTarget(target) && !mount.ReadOnly {
			return fmt.Errorf("node-owned container mount target %q must be read-only", target)
		}
		seen[target] = struct{}{}
	}

	for i := range mounts {
		for j := i + 1; j < len(mounts); j++ {
			parent, child, overlaps := mountOverlap(mounts[i].Target, mounts[j].Target)
			if !overlaps || permitsNestedMount(nested, parent, child) {
				continue
			}
			return fmt.Errorf("container mount targets %q and %q overlap", mounts[i].Target, mounts[j].Target)
		}
	}
	return nil
}

func isNodeOwnedTarget(target string) bool {
	return target == nodeCacheContainerDir || strings.HasPrefix(target, nodeCacheContainerDir+"/") ||
		target == nodeConfigContainerRoot || strings.HasPrefix(target, nodeConfigContainerRoot+"/")
}

func mountOverlap(first, second string) (string, string, bool) {
	if strings.HasPrefix(second, first+"/") {
		return first, second, true
	}
	if strings.HasPrefix(first, second+"/") {
		return second, first, true
	}
	return "", "", false
}

func permitsNestedMount(contracts []nestedMountContract, parent, child string) bool {
	for _, contract := range contracts {
		if contract.parent == parent && contract.child == child {
			return true
		}
	}
	return false
}
