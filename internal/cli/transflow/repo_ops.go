package transflow

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// hasRepoChanges returns true if the working tree has any changes
func hasRepoChanges(repoPath string) (bool, error) {
	cmd := exec.Command("git", "status", "--porcelain")
	cmd.Dir = repoPath
	out, err := cmd.CombinedOutput()
	if err != nil {
		return false, fmt.Errorf("git status failed: %v: %s", err, string(out))
	}
	return strings.TrimSpace(string(out)) != "", nil
}

// getHeadHash returns the current HEAD commit hash
func getHeadHash(repoPath string) (string, error) {
	cmd := exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = repoPath
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git rev-parse failed: %v: %s", err, string(out))
	}
	return strings.TrimSpace(string(out)), nil
}

// createTarFromDir creates a tar archive of a directory using system tar
func createTarFromDir(srcDir, dstTar string) error {
	// Remove existing tar if any
	_ = os.Remove(dstTar)
	cmd := exec.Command("tar", "-cf", dstTar, ".")
	cmd.Dir = srcDir
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("tar failed: %v: %s", err, string(out))
	}
	return nil
}

// test indirection for getHeadHash
var getHeadHashFn = getHeadHash
