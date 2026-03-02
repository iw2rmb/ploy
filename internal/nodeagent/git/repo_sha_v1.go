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
	repoSHAV1AuthorLine  = "author ploy-node <ploy-node@ploy.local> 0 +0000"
	repoSHAV1CommitLine  = "committer ploy-node <ploy-node@ploy.local> 0 +0000"
	repoSHAV1CommitTitle = "ploy repo_sha_v1"
)

// ComputeRepoSHAV1 calculates deterministic repo_sha_out for a workspace.
//
// Algorithm:
//  1. Stage workspace snapshot in a temporary git index (no ref mutations).
//  2. Compute snapshot tree hash.
//  3. If snapshot tree == repo_sha_in tree, return repo_sha_in.
//  4. Otherwise compute synthetic commit hash from fixed metadata.
func ComputeRepoSHAV1(ctx context.Context, repoDir, repoSHAIn string) (string, error) {
	repoSHAIn = strings.TrimSpace(repoSHAIn)
	if !repoSHA40Pattern.MatchString(repoSHAIn) {
		return "", fmt.Errorf("repo_sha_in must match ^[0-9a-f]{40}$")
	}

	indexFile, err := os.CreateTemp("", "ploy-repo-sha-index-*")
	if err != nil {
		return "", fmt.Errorf("create temp index: %w", err)
	}
	indexPath := indexFile.Name()
	if closeErr := indexFile.Close(); closeErr != nil {
		_ = os.Remove(indexPath)
		return "", fmt.Errorf("close temp index: %w", closeErr)
	}
	defer func() {
		_ = os.Remove(indexPath)
	}()

	env := []string{"GIT_INDEX_FILE=" + indexPath}

	if _, err := runGitOutput(ctx, repoDir, env, nil, "read-tree", repoSHAIn); err != nil {
		return "", fmt.Errorf("read input tree: %w", err)
	}
	if _, err := runGitOutput(
		ctx,
		repoDir,
		env,
		nil,
		"add", "-A", "--", ".",
		":(exclude)**/target/**", ":(exclude)target/",
	); err != nil {
		return "", fmt.Errorf("stage workspace snapshot: %w", err)
	}

	snapshotTree, err := runGitOutput(ctx, repoDir, env, nil, "write-tree")
	if err != nil {
		return "", fmt.Errorf("compute snapshot tree: %w", err)
	}
	inputTree, err := runGitOutput(ctx, repoDir, nil, nil, "rev-parse", repoSHAIn+"^{tree}")
	if err != nil {
		return "", fmt.Errorf("resolve input tree: %w", err)
	}
	if snapshotTree == inputTree {
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
