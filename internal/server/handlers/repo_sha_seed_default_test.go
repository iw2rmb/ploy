package handlers

import (
	"context"
	"fmt"
	"strings"
)

const testSourceCommitSHA = "0123456789abcdef0123456789abcdef01234567"

func init() {
	sourceCommitSHAResolver = func(_ context.Context, repoURL, ref string) (resolvedSourceCommit, error) {
		if strings.TrimSpace(repoURL) == "" {
			return resolvedSourceCommit{}, fmt.Errorf("repo_url is empty")
		}
		if strings.TrimSpace(ref) == "" {
			return resolvedSourceCommit{}, fmt.Errorf("base_ref is empty")
		}
		return resolvedSourceCommit{BaseRef: ref, SHA: testSourceCommitSHA}, nil
	}
}
