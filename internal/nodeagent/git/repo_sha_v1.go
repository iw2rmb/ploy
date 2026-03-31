package git

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
)

var repoSHA40Pattern = regexp.MustCompile(`^[0-9a-f]{40}$`)

const (
	repoSHAV1AuthorLine  = "author node <node@ploy.local> 0 +0000"
	repoSHAV1CommitLine  = "committer node <node@ploy.local> 0 +0000"
	repoSHAV1CommitTitle = "ploy repo_sha_v1"
)

var workspaceTreeAddArgs = []string{
	"add", "-A", "--", ".",
	":(exclude)**/target/**", ":(exclude)target/",
}

// ComputeRepoSHAV1 calculates deterministic repo_sha_out for a workspace.
//
// Algorithm:
//  1. Stage workspace snapshot in a temporary git index (no ref mutations).
//  2. Compute snapshot tree hash.
//  3. If snapshot tree == repo_sha_in tree, return repo_sha_in.
//  4. Otherwise compute synthetic commit hash from fixed metadata.
//
// Optional inputTree may be supplied by caller when repo_sha_in is synthetic and
// not resolvable as a local git object.
func ComputeRepoSHAV1(ctx context.Context, repoDir, repoSHAIn string, inputTree ...string) (string, error) {
	repoSHAIn = strings.TrimSpace(repoSHAIn)
	if !repoSHA40Pattern.MatchString(repoSHAIn) {
		return "", fmt.Errorf("repo_sha_in must match ^[0-9a-f]{40}$")
	}

	snapshotTree, err := ComputeWorkspaceTreeSHA(ctx, repoDir)
	if err != nil {
		return "", err
	}

	inTree, err := resolveInputTree(ctx, repoDir, repoSHAIn, inputTree...)
	if err != nil {
		return "", err
	}
	if snapshotTree == inTree {
		return repoSHAIn, nil
	}

	commitData := fmt.Sprintf(
		"tree %s\nparent %s\n%s\n%s\n\n%s\n",
		snapshotTree,
		repoSHAIn,
		repoSHAV1AuthorLine,
		repoSHAV1CommitLine,
		repoSHAV1CommitTitle,
	)
	repoSHAOut, err := runGitOutput(ctx, repoDir, nil, []byte(commitData), "hash-object", "-t", "commit", "--stdin")
	if err != nil {
		return "", fmt.Errorf("hash synthetic commit: %w", err)
	}
	if !repoSHA40Pattern.MatchString(repoSHAOut) {
		return "", fmt.Errorf("computed repo_sha_out has invalid format")
	}
	return repoSHAOut, nil
}

// ComputeWorkspaceTreeSHA computes a stable tree hash for the current workspace
// using a temporary index and excluding build output folders.
func ComputeWorkspaceTreeSHA(ctx context.Context, repoDir string) (string, error) {
	indexFile, err := os.CreateTemp("", "ploy-repo-sha-index-*")
	if err != nil {
		return "", fmt.Errorf("create temp index: %w", err)
	}
	indexPath := indexFile.Name()
	if closeErr := indexFile.Close(); closeErr != nil {
		_ = os.Remove(indexPath)
		return "", fmt.Errorf("close temp index: %w", closeErr)
	}
	if err := os.Remove(indexPath); err != nil {
		return "", fmt.Errorf("remove temp index placeholder: %w", err)
	}
	defer func() {
		_ = os.Remove(indexPath)
	}()

	env := []string{"GIT_INDEX_FILE=" + indexPath}
	if _, err := runGitOutput(ctx, repoDir, env, nil, workspaceTreeAddArgs...); err != nil {
		return "", fmt.Errorf("stage workspace snapshot: %w", err)
	}

	treeSHA, err := runGitOutput(ctx, repoDir, env, nil, "write-tree")
	if err != nil {
		return "", fmt.Errorf("compute snapshot tree: %w", err)
	}
	if !repoSHA40Pattern.MatchString(treeSHA) {
		return "", fmt.Errorf("computed snapshot tree has invalid format")
	}
	return treeSHA, nil
}

func resolveInputTree(ctx context.Context, repoDir, repoSHAIn string, inputTree ...string) (string, error) {
	if len(inputTree) > 0 {
		tree := strings.TrimSpace(inputTree[0])
		if tree != "" {
			if !repoSHA40Pattern.MatchString(tree) {
				return "", fmt.Errorf("input tree must match ^[0-9a-f]{40}$")
			}
			return tree, nil
		}
	}

	tree, err := runGitOutput(ctx, repoDir, nil, nil, "rev-parse", repoSHAIn+"^{tree}")
	if err == nil {
		return tree, nil
	}

	// repo_sha_in can be synthetic and absent from local object DB; fallback to
	// HEAD tree to preserve local unchanged/changed detection.
	headTree, headErr := runGitOutput(ctx, repoDir, nil, nil, "rev-parse", "HEAD^{tree}")
	if headErr != nil {
		return "", fmt.Errorf("resolve input tree: %w (fallback HEAD^{tree} failed: %v)", err, headErr)
	}
	return headTree, nil
}

func runGitOutput(ctx context.Context, repoDir string, env []string, stdin []byte, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = repoDir
	cmd.Env = append(os.Environ(), env...)
	if len(stdin) > 0 {
		cmd.Stdin = bytes.NewReader(stdin)
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf(
			"git %s failed: %w (stderr=%s)",
			strings.Join(args, " "),
			err,
			strings.TrimSpace(stderr.String()),
		)
	}
	return strings.TrimSpace(stdout.String()), nil
}
