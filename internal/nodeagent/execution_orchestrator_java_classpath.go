package nodeagent

import (
	"fmt"
	"path/filepath"
	"strings"

	types "github.com/iw2rmb/ploy/internal/domain/types"
)

func runRepoJavaClasspathPath(runID types.RunID, repoID types.MigRepoID) string {
	shareDir := runRepoShareDir(runID, repoID)
	if strings.TrimSpace(shareDir) == "" {
		return ""
	}
	return filepath.Join(shareDir, sbomJavaClasspathFileName)
}

func requiresJavaClasspath(req StartRunRequest) bool {
	if req.JobType == types.JobTypeSBOM || req.JobType == types.JobTypeHeal || req.JavaClasspathContext == nil {
		return false
	}
	return req.JavaClasspathContext.Required
}

func (r *runController) ensureRequiredJavaClasspathShare(req StartRunRequest) error {
	if !requiresJavaClasspath(req) {
		return nil
	}

	sourcePath := runRepoJavaClasspathPath(req.RunID, req.RepoID)
	if strings.TrimSpace(sourcePath) == "" {
		return fmt.Errorf("run/repo share java classpath path is not configured")
	}
	if err := validateJavaClasspathPath(sourcePath); err != nil {
		return fmt.Errorf("validate /share/%s: %w", sbomJavaClasspathFileName, err)
	}
	return nil
}
