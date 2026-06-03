package step

import (
	"bufio"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// readGradleBuildCacheHits parses the workspace-local cache-hits file written
// by the Gradle gate init script, returning the sorted unique task list. The
// file is removed after a successful read so subsequent runs start clean.
func readGradleBuildCacheHits(workspace string) []string {
	path := filepath.Join(workspace, gradleCacheHitsHostFile)
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer func() { _ = f.Close() }()
	defer func() { _ = os.Remove(path) }()

	seen := make(map[string]struct{})
	var hits []string

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		s := strings.TrimSpace(scanner.Text())
		if s == "" {
			continue
		}
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		hits = append(hits, s)
	}
	if len(hits) == 0 {
		return nil
	}
	sort.Strings(hits)
	return hits
}
