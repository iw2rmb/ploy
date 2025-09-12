package transflow

import (
    "context"
    "fmt"
    "strings"
    "time"
)

// submitORWJobAndFetchDiff validates HCL, reports job, submits the job and fetches diff.patch from SeaweedFS.
// The diff is fetched from artifacts/transflow/<execID>/branches/<branchID>/steps/<stepID>/diff.patch into diffPath.
func submitORWJobAndFetchDiff(
    ctx context.Context,
    validate func(string) error,
    submit func(string, time.Duration) error,
    reportLastJob func(context.Context, string, string, string),
    seaweed, execID, branchID, stepID, hclPath, diffPath string,
    timeout time.Duration,
) error {
    runID := fmt.Sprintf("orw-apply-%s", stepID)
    if err := validate(hclPath); err != nil {
        return fmt.Errorf("ORW HCL validation failed: %w", err)
    }
    if reportLastJob != nil {
        reportLastJob(ctx, runID, "apply", "orw-apply")
    }
    if err := submit(hclPath, timeout); err != nil {
        return fmt.Errorf("orw-apply job failed: %w", err)
    }
    if execID == "" {
        return fmt.Errorf("missing execution id for diff fetch")
    }
    // Fetch diff from SeaweedFS
    branchDiffKey := computeBranchDiffKey(execID, branchID, stepID)
    url := strings.TrimRight(seaweed, "/") + "/artifacts/" + branchDiffKey
    if err := downloadToFileFn(url, diffPath); err != nil {
        return fmt.Errorf("no diff produced by orw-apply: %w", err)
    }
    return nil
}

