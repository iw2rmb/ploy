package transflow

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"

	doublestar "github.com/bmatcuk/doublestar/v4"
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

// ValidateDiffPaths checks that all changed file paths in the unified diff match at least one allowed glob.
// allowedGlobs example: []string{"src/**", "pom.xml"}
func ValidateDiffPaths(diffPath string, allowedGlobs []string) error {
	f, err := os.Open(diffPath)
	if err != nil {
		return err
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		// Look for lines starting with "+++ " and extract path portion
		if len(line) > 4 && line[:4] == "+++ " {
			// Expected format: +++ b/path or +++ /dev/null
			fields := line[4:]
			if fields == "/dev/null" {
				continue
			}
			// Strip leading prefixes like a/ or b/
			p := fields
			if len(p) > 2 && (p[:2] == "a/" || p[:2] == "b/") {
				p = p[2:]
			}
			// Match against allowed globs using doublestar (** support)
			ok := false
			for _, pat := range allowedGlobs {
				if m, _ := doublestar.PathMatch(pat, p); m {
					ok = true
					break
				}
			}
			if !ok {
				return fmt.Errorf("diff touches disallowed path: %s", p)
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	return nil
}
