package transflow

import (
    "context"
    "fmt"
    "os/exec"
)

// ValidateUnifiedDiff runs a dry-run git apply --check on the diff file in the repo path
func ValidateUnifiedDiff(ctx context.Context, repoPath, diffPath string) error {
    cmd := exec.CommandContext(ctx, "git", "apply", "--check", diffPath)
    cmd.Dir = repoPath
    out, err := cmd.CombinedOutput()
    if err != nil {
        return fmt.Errorf("git apply --check failed: %v: %s", err, string(out))
    }
    return nil
}

// ApplyUnifiedDiff applies a unified diff to the repo path
func ApplyUnifiedDiff(ctx context.Context, repoPath, diffPath string) error {
    cmd := exec.CommandContext(ctx, "git", "apply", diffPath)
    cmd.Dir = repoPath
    out, err := cmd.CombinedOutput()
    if err != nil {
        return fmt.Errorf("git apply failed: %v: %s", err, string(out))
    }
    return nil
}

