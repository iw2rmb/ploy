package mods

import (
	"context"
	"os"
	"path/filepath"
)

// reconstructBranchState replays previous step diffs in a branch from root to HEAD.
// Best-effort: skips failures silently to match previous behavior.
func (r *ModRunner) reconstructBranchState(ctx context.Context, seaweed, modID, branchID, baseDir, repoPath string) error {
	rep := BranchChainReplayer{
		GetJSON:             getJSONFn,
		DownloadToFile:      downloadToFileFn,
		ValidateDiffPaths:   validateDiffPathsFn,
		ValidateUnifiedDiff: validateUnifiedDiffFn,
		ApplyUnifiedDiff:    applyUnifiedDiffFn,
		Allowlist:           ResolveDefaultsFromEnv().Allowlist,
		Reporter:            NewControllerEventReporter(ResolveInfraFromEnv().Controller, os.Getenv("MOD_ID")),
	}
	return rep.Replay(ctx, seaweed, modID, branchID, filepath.Join(baseDir, "out"), repoPath)
}
