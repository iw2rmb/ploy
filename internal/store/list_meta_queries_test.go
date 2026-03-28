package store

import (
	"strings"
	"testing"
)

// TestListMetaQueriesDoNotReturnBlobs verifies that list queries
// exclude large blob columns to reduce I/O.
func TestListMetaQueriesDoNotReturnBlobs(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name         string
		sql          string
		excludedCols []string
	}{
		// diffs.sql - exclude "patch" blob
		{
			name:         "ListDiffsByRun",
			sql:          listDiffsByRun,
			excludedCols: []string{", patch,", ", patch "},
		},
		{
			name:         "ListDiffsByRunRepo",
			sql:          listDiffsByRunRepo,
			excludedCols: []string{", patch,", ", patch ", "d.patch,", "d.patch "},
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			sqlLower := strings.ToLower(tc.sql)
			for _, col := range tc.excludedCols {
				if strings.Contains(sqlLower, strings.ToLower(col)) {
					t.Fatalf("%s must NOT include blob column; found %q in SQL:\n%s",
						tc.name, col, tc.sql)
				}
			}
			// Verify SELECT is explicit (not SELECT *)
			if strings.Contains(sqlLower, "select *") || strings.Contains(sqlLower, "select d.*") {
				t.Fatalf("%s must not use SELECT *; found wildcard in SQL:\n%s",
					tc.name, tc.sql)
			}
		})
	}
}

// TestListMetaQueriesUseExplicitSelect verifies that artifact/event list queries
// use explicit column selection (not SELECT *) so the selector surface is stable.
func TestListMetaQueriesUseExplicitSelect(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		sql  string
	}{
		// artifact_bundles.sql - explicit column list, no wildcard
		{"ListArtifactBundlesByRun", listArtifactBundlesByRun},
		{"ListArtifactBundlesByRunAndJob", listArtifactBundlesByRunAndJob},
		{"ListArtifactBundlesByCID", listArtifactBundlesByCID},
		// events.sql - explicit column list, no wildcard
		{"ListEventsByRun", listEventsByRun},
		{"ListEventsByRunSince", listEventsByRunSince},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			sqlLower := strings.ToLower(tc.sql)
			if strings.Contains(sqlLower, "select *") {
				t.Fatalf("%s must use explicit column selection, not SELECT *; found wildcard in SQL:\n%s",
					tc.name, tc.sql)
			}
		})
	}
}

// TestListMetaQueriesHaveDeterministicOrder verifies list queries
// have deterministic tie-breakers.
func TestListMetaQueriesHaveDeterministicOrder(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name      string
		sql       string
		wantOrder string
	}{
		// diffs.sql - created_at ASC, id ASC with deterministic tie-breakers
		{"ListDiffsByRun", listDiffsByRun, "ORDER BY created_at ASC, id ASC"},
		{"ListDiffsByRunRepo", listDiffsByRunRepo, "d.created_at ASC, d.id ASC"},
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
