package transflow

import (
	"context"
	"path/filepath"
)

// reconstructBranchState replays previous step diffs in a branch from root to HEAD.
// Best-effort: skips failures silently to match previous behavior.
func (r *TransflowRunner) reconstructBranchState(ctx context.Context, seaweed, execID, branchID, baseDir, repoPath string) error {
	rep := BranchChainReplayer{
		GetJSON:             getJSONFn,
		DownloadToFile:      downloadToFileFn,
		ValidateDiffPaths:   validateDiffPathsFn,
		ValidateUnifiedDiff: validateUnifiedDiffFn,
		ApplyUnifiedDiff:    applyUnifiedDiffFn,
		Allowlist:           ResolveDefaultsFromEnv().Allowlist,
	}
	return rep.Replay(ctx, seaweed, execID, branchID, filepath.Join(baseDir, "out"), repoPath)
}
