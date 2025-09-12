package transflow

import (
    "os"
    "path/filepath"
)

// checkBuildFiles reports presence of common build files in a repo.
func checkBuildFiles(repoPath string) (hasPom, hasGradle, hasKts bool) {
    if _, err := os.Stat(filepath.Join(repoPath, "pom.xml")); err == nil { hasPom = true }
    if _, err := os.Stat(filepath.Join(repoPath, "build.gradle")); err == nil { hasGradle = true }
    if _, err := os.Stat(filepath.Join(repoPath, "build.gradle.kts")); err == nil { hasKts = true }
    return
}

// ensureBuildFile returns error when no supported build file is present.
func ensureBuildFile(repoPath string) error {
    hasPom, hasGradle, hasKts := checkBuildFiles(repoPath)
    if !hasPom && !hasGradle && !hasKts {
        return ErrNoBuildFile
    }
    return nil
}

