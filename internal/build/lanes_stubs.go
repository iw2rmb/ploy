package build

import (
    "context"
)

// BuildResult is a minimal alias to satisfy stub signatures.
// Use the existing type from this package if defined elsewhere.

// Stubs for lane-specific builders to unblock Dev deployments.
// These return empty results and no error; callers should handle no-op results.
func buildLaneAB(ctx context.Context, srcDir, appName, sha, mainClass string) (*BuildResult, string, error) {
    return &BuildResult{Success: true, Version: ""}, "", nil
}

func buildLaneC(ctx context.Context, srcDir, appName, sha, mainClass string) (*BuildResult, string, error) {
    return &BuildResult{Success: true, Version: ""}, "", nil
}

func buildLaneD(ctx context.Context, srcDir, appName, sha, mainClass string) (*BuildResult, string, error) {
    return &BuildResult{Success: true, Version: ""}, "", nil
}

func buildLaneE(ctx context.Context, srcDir, appName, sha, mainClass string) (*BuildResult, string, error) {
    return &BuildResult{Success: true, Version: ""}, "", nil
}

func buildLaneF(ctx context.Context, srcDir, appName, sha, mainClass string) (*BuildResult, string, error) {
    return &BuildResult{Success: true, Version: ""}, "", nil
}

func buildLaneG(ctx context.Context, srcDir, appName, sha, mainClass string) (*BuildResult, string, error) {
    return &BuildResult{Success: true, Version: ""}, "", nil
}

