package store

import (
	"strings"
	"testing"
)

// Ensure log listing queries have deterministic ordering.
// Note: ListLogsByRunJobAndBuild and ListLogsByRunJobAndBuildSince removed as part of
// builds table removal; logs now use job-level grouping only.
func TestLogsQueries_OrderByChunkThenID(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		sql  string
	}{
		{"ListLogsByRun", listLogsByRun},
		{"ListLogsByRunSince", listLogsByRunSince},
		{"ListLogsByRunAndJob", listLogsByRunAndJob},
		{"ListLogsByRunAndJobSince", listLogsByRunAndJobSince},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			want := "ORDER BY chunk_no ASC, id ASC"
			if !containsIgnoreSpace(tc.sql, want) {
				t.Fatalf("%s must order deterministically; want substring %q, got: %q", tc.name, want, tc.sql)
			}
		})
	}
}

// containsIgnoreSpace reports whether b contains a, ignoring redundant whitespace
// differences introduced by sqlc formatting.
func containsIgnoreSpace(b, a string) bool {
	// Fast path.
	if containsNormalized(b, a) {
		return true
	}
	// Allow trailing newline or semicolon variations.
	if containsNormalized(b, a+"\n") || containsNormalized(b, a+"\n\n") || containsNormalized(b, a+"\n\n\n") {
		return true
	}
	return false
}

func containsNormalized(haystack, needle string) bool {
	// We only need a minimal normalization; sqlc preserves spaces/newlines
	// around ORDER BY consistently, so a direct substring check suffices.
	return strings.Contains(haystack, needle)
}
