package mods

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// submitORWJobAndFetchDiff validates HCL, reports job, submits the job and fetches diff.patch from SeaweedFS.
// The diff is fetched from artifacts/mods/<modID>/branches/<branchID>/steps/<stepID>/diff.patch into diffPath.
func submitORWJobAndFetchDiff(
	ctx context.Context,
	validate func(string) error,
	submit func(string, time.Duration) error,
	reportLastJob func(context.Context, string, string, string),
	seaweed, modID, branchID, stepID, jobName, hclPath, diffPath string,
	timeout time.Duration,
) error {
	if err := validate(hclPath); err != nil {
		return fmt.Errorf("ORW HCL validation failed: %w", err)
	}
	if reportLastJob != nil && jobName != "" {
		reportLastJob(ctx, jobName, "apply", string(StepTypeORWApply))
	}
	if err := submit(hclPath, timeout); err != nil {
		return fmt.Errorf("orw-apply job failed: %w", err)
	}
	if modID == "" {
		return fmt.Errorf("missing mod id for diff fetch")
	}
	// Fetch diff from SeaweedFS
	branchDiffKey := computeBranchDiffKey(modID, branchID, stepID)
	url := strings.TrimRight(seaweed, "/") + "/artifacts/" + branchDiffKey
	if err := os.MkdirAll(filepath.Dir(diffPath), 0755); err != nil {
		return fmt.Errorf("prepare diff dir: %w", err)
	}
	if err := downloadToFileFn(url, diffPath); err != nil {
		return fmt.Errorf("no diff produced by orw-apply: %w", err)
	}
	return nil
}
