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
		// jobs.sql - repo/attempt scopes keep deterministic id tie-breakers
		{"ListJobsByRun", listJobsByRun, "ORDER BY repo_id ASC, attempt ASC, id ASC"},
		{"ListJobsByRunRepoAttempt", listJobsByRunRepoAttempt, "ORDER BY id ASC"},
		{"ListCreatedJobsByRunRepoAttempt", listCreatedJobsByRunRepoAttempt, "ORDER BY id ASC"},

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
		{"ListReposByPrepStatus", listReposByPrepStatus, "ORDER BY prep_updated_at ASC, created_at ASC, id ASC"},
		{"ListDistinctRepos", listDistinctRepos, "ORDER BY mr.repo_url ASC, mr.id ASC"},
		{"ListDistinctRepos (lateral)", listDistinctRepos, "ORDER BY rrr.started_at DESC NULLS LAST, rrr.created_at DESC, rrr.run_id DESC"},

		// diffs.sql - created_at needs id tie-breaker
		{"ListDiffsByRunRepo", listDiffsByRunRepo, "d.created_at ASC, d.id ASC"},

		// artifact_bundles.sql - created_at needs id tie-breaker
		{"ListArtifactBundlesByRun", listArtifactBundlesByRun, "ORDER BY created_at DESC, id DESC"},
		{"ListArtifactBundlesByRunAndJob", listArtifactBundlesByRunAndJob, "ORDER BY created_at DESC, id DESC"},
		{"ListArtifactBundlesByCID", listArtifactBundlesByCID, "ORDER BY created_at DESC, id DESC"},

		// tokens.sql - created_at needs token_id tie-breaker
		{"ListAPITokens", listAPITokens, "ORDER BY created_at DESC, token_id DESC"},

		// run_repos.sql - created_at needs repo_id/run_id tie-breakers (composite PK)
		{"ListRunReposByRun", listRunReposByRun, "ORDER BY created_at ASC, repo_id ASC"},
		{"ListQueuedRunReposByRun", listQueuedRunReposByRun, "ORDER BY created_at ASC, repo_id ASC"},
		{"ListRunReposWithURLByRun", listRunReposWithURLByRun, "ORDER BY rr.created_at ASC, rr.repo_id ASC"},
		{"ListRunsForRepo", listRunsForRepo, "ORDER BY rr.created_at DESC, rr.run_id DESC"},
		{"ListFailedRepoIDsByMig", listFailedRepoIDsByMig, "ORDER BY rr.repo_id, rr.created_at DESC, rr.run_id DESC"},

		// logs.sql and events.sql already have id tie-breakers (verified in logs_sql_ordering_test.go)
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
