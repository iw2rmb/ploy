package handlers

import (
	"context"
	"fmt"
	"strings"
)

const testSourceCommitSHA = "0123456789abcdef0123456789abcdef01234567"

func init() {
	sourceCommitSHAResolver = func(_ context.Context, repoURL, ref string) (string, error) {
		if strings.TrimSpace(repoURL) == "" {
			return "", fmt.Errorf("repo_url is empty")
		}
		if strings.TrimSpace(ref) == "" {
			return "", fmt.Errorf("base_ref is empty")
		}
		return testSourceCommitSHA, nil
	}
}
