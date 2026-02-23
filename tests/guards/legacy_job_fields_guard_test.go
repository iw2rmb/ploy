package guards

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestNoNewLegacyJobFieldTokensInProtectedSurfaces blocks introducing new
// legacy job-field names in protected interface surfaces while migration is in
// progress. The baseline counts match the repository state when this guard was
// added; counts may go down as migration progresses, but must not go up.
func TestNoNewLegacyJobFieldTokensInProtectedSurfaces(t *testing.T) {
	type tokenBudget map[string]int

	budgets := map[string]tokenBudget{
		"internal/nodeagent/job.go": {
			"mod_type":   1,
			"mod_image":  0,
			"step_index": 0,
			"ModType":    7,
			"ModImage":   0,
		},
		"internal/workflow/contracts/job_meta.go": {
			"mod_type":   0,
			"mod_image":  0,
			"step_index": 0,
			"ModType":    0,
			"ModImage":   0,
		},
		"docs/api/components/schemas/controlplane.yaml": {
			"mod_type":   6,
			"mod_image":  3,
			"step_index": 14,
			"ModType":    0,
			"ModImage":   0,
		},
	}

	for path, budget := range budgets {
		content, err := os.ReadFile(resolveRepoPath(path))
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		text := string(content)

		for token, maxCount := range budget {
			got := strings.Count(text, token)
			if got > maxCount {
				t.Fatalf("%s: token %q count increased: got=%d max=%d", path, token, got, maxCount)
			}
		}
	}
}

func resolveRepoPath(path string) string {
	if _, err := os.Stat(path); err == nil {
		return path
	}
	return filepath.Join("..", "..", path)
}
