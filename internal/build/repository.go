package build

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/iw2rmb/ploy/internal/git"
)

// extractSourceRepository attempts to extract source repository information using enhanced Git integration
func extractSourceRepository(srcDir string) string {
	// Try using enhanced Git utilities first
	gitUtils := git.NewGitUtils(srcDir)
	if gitUtils.IsGitRepository() {
		if url, err := gitUtils.GetRepositoryURL(); err == nil && url != "" {
			return url
		}
	}

	// Fallback to original implementation for non-Git projects
	// Try to read from package.json for Node.js projects
	packageJSONPath := filepath.Join(srcDir, "package.json")
	if data, err := os.ReadFile(packageJSONPath); err == nil {
		var pkg map[string]interface{}
		if json.Unmarshal(data, &pkg) == nil {
			if repo, ok := pkg["repository"]; ok {
				if repoMap, ok := repo.(map[string]interface{}); ok {
					if url, ok := repoMap["url"].(string); ok {
						return url
					}
				} else if repoStr, ok := repo.(string); ok {
					return repoStr
				}
			}
		}
	}

	return ""
}
