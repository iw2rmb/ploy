package store

import (
	"strings"
	"testing"
)

// TestListQueriesDeterministicOrder ensures all list queries ordering by non-unique columns
// have deterministic tie-breakers (id, created_at+id, etc.) to prevent nondeterministic
// ordering on ties.
// For chain/model-specific orderings, include stable tie-breakers.
func TestListQueriesDeterministicOrder(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name      string
		sql       string
		wantOrder string
	}{
		// jobs.sql - run/attempt scopes keep deterministic id tie-breakers
		{"ListJobsByRun", listJobsByRun, "ORDER BY attempt ASC, id ASC"},
		{"ListJobsByRunAttempt", listJobsByRunAttempt, "ORDER BY id ASC"},
		{"ListCreatedJobsByRunAttempt", listCreatedJobsByRunAttempt, "ORDER BY id ASC"},

		// runs.sql - created_at needs id tie-breaker
		{"ListRuns", listRuns, "ORDER BY created_at DESC, id DESC"},

		// nodes.sql - created_at needs id tie-breaker
		{"ListNodes", listNodes, "ORDER BY created_at DESC, id DESC"},

		// migs.sql - created_at needs id tie-breaker
		{"ListMigs", listMigs, "ORDER BY created_at DESC, id DESC"},

		// specs.sql - created_at needs id tie-breaker
		{"ListSpecs", listSpecs, "ORDER BY created_at DESC, id DESC"},

		// mig_repos.sql - created_at and repo_url need id tie-breakers
		{"ListMigReposByMig", listMigReposByMig, "ORDER BY created_at ASC, id ASC"},
		{"ListDistinctRepos", listDistinctRepos, "ORDER BY r.url ASC, mr.repo_id ASC"},
		{"ListDistinctRepos (lateral)", listDistinctRepos, "ORDER BY runs.started_at DESC NULLS LAST, runs.created_at DESC, runs.id DESC"},

		// tokens.sql - created_at needs token_id tie-breaker
		{"ListAPITokens", listAPITokens, "ORDER BY created_at DESC, token_id DESC"},

		// runs.sql - created_at needs id tie-breakers.
		{"ListRunsByWave", listRunsByWave, "ORDER BY created_at ASC, id ASC"},
		{"ListQueuedRunsByWave", listQueuedRunsByWave, "ORDER BY created_at ASC, id ASC"},
		{"ListRunsWithURLByWave", listRunsWithURLByWave, "ORDER BY runs.created_at ASC, runs.id ASC"},
		{"ListRunsForRepo", listRunsForRepo, "ORDER BY runs.created_at DESC, runs.id DESC"},
		{"ListFailedRepoIDsByMig", listFailedRepoIDsByMig, "ORDER BY repo_id, created_at DESC, id DESC"},

		// logs.sql - chunk order needs id tie-breaker.
		{"ListLogsByRun", listLogsByRun, "ORDER BY chunk_no ASC, id ASC"},
		{"ListLogsByRunSince", listLogsByRunSince, "ORDER BY chunk_no ASC, id ASC"},
		{"ListLogsByRunAndJob", listLogsByRunAndJob, "ORDER BY chunk_no ASC, id ASC"},
		{"ListLogsByRunAndJobSince", listLogsByRunAndJobSince, "ORDER BY chunk_no ASC, id ASC"},

		// diffs/artifact_bundles/events ordering is verified in list_meta_queries_test.go.
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if !containsOrderBy(tc.sql, tc.wantOrder) {
				t.Fatalf("%s must have deterministic ordering; want substring %q in SQL:\n%s",
					tc.name, tc.wantOrder, tc.sql)
			}
		})
	}
}

// containsOrderBy checks if the SQL contains the expected ORDER BY clause,
// handling whitespace variations from sqlc formatting.
func containsOrderBy(sql, orderBy string) bool {
	// Normalize whitespace for comparison
	normalized := normalizeWhitespace(sql)
	want := normalizeWhitespace(orderBy)
	return strings.Contains(normalized, want)
}

// normalizeWhitespace collapses multiple whitespace characters into single spaces.
func normalizeWhitespace(s string) string {
	var result strings.Builder
	inWhitespace := false
	for _, r := range s {
		if r == ' ' || r == '\t' || r == '\n' || r == '\r' {
			if !inWhitespace {
				result.WriteRune(' ')
				inWhitespace = true
			}
		} else {
			result.WriteRune(r)
			inWhitespace = false
		}
	}
	return strings.TrimSpace(result.String())
}
