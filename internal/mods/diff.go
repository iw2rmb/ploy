package mods

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

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
	defer func() { _ = f.Close() }()
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

// stagePathsFromDiff stages only the files referenced by diffPath.
// - Added/Modified files: git add -- path
// - Deleted files: git rm -- path (if present), otherwise ensure removal is staged
// It also avoids staging common build artifacts by limiting to explicit diff paths.
func stagePathsFromDiff(ctx context.Context, repoPath, diffPath string) error {
	addedOrModified := make(map[string]struct{})
	deleted := make(map[string]struct{})

	f, err := os.Open(diffPath)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	s := bufio.NewScanner(f)
	var lastMinus, lastPlus string
	for s.Scan() {
		line := s.Text()
		if strings.HasPrefix(line, "--- ") {
			lastMinus = strings.TrimPrefix(line, "--- ")
		} else if strings.HasPrefix(line, "+++ ") {
			lastPlus = strings.TrimPrefix(line, "+++ ")
			// Determine added/modified/deleted based on pairing
			// Normalize prefixes a/ or b/
			norm := func(p string) string {
				if p == "/dev/null" {
					return p
				}
				if strings.HasPrefix(p, "a/") || strings.HasPrefix(p, "b/") {
					return p[2:]
				}
				return p
			}
			minus := norm(lastMinus)
			plus := norm(lastPlus)
			if plus == "/dev/null" && minus != "/dev/null" {
				deleted[minus] = struct{}{}
			} else if plus != "/dev/null" { // added or modified
				addedOrModified[plus] = struct{}{}
			}
		}
	}
	if err := s.Err(); err != nil {
		return err
	}

	// Stage deletions first
	for p := range deleted {
		cmd := exec.CommandContext(ctx, "git", "rm", "--", p)
		cmd.Dir = repoPath
		if out, err := cmd.CombinedOutput(); err != nil {
			// Fallback: try to remove cached if file already absent
			_ = exec.CommandContext(ctx, "git", "rm", "--cached", "--", p).Run()
			_ = out
		}
	}
	// Stage added/modified
	for p := range addedOrModified {
		// Skip common build artifact locations defensively (target/, build/, .sbom.json handled by diff itself)
		if strings.HasPrefix(p, "target/") || strings.HasPrefix(p, "build/") {
			continue
		}
		cmd := exec.CommandContext(ctx, "git", "add", "--", p)
		cmd.Dir = repoPath
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("git add failed for %s: %v: %s", p, err, string(out))
		}
	}
	return nil
}
