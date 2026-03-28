package store

import (
	"strings"
	"testing"
)

// TestListMetaQueriesDoNotReturnBlobs verifies that List*Meta queries
// exclude large blob columns (bundle, meta) to reduce I/O.
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

		// artifact_bundles.sql - exclude "bundle" blob
		{
			name:         "ListArtifactBundlesMetaByRun",
			sql:          listArtifactBundlesMetaByRun,
			excludedCols: []string{", bundle,", ", bundle "},
		},
		{
			name:         "ListArtifactBundlesMetaByRunAndJob",
			sql:          listArtifactBundlesMetaByRunAndJob,
			excludedCols: []string{", bundle,", ", bundle "},
		},
		{
			name:         "ListArtifactBundlesMetaByCID",
			sql:          listArtifactBundlesMetaByCID,
			excludedCols: []string{", bundle,", ", bundle "},
		},

		// events.sql - exclude "meta" JSONB blob
		{
			name:         "ListEventsMetaByRun",
			sql:          listEventsMetaByRun,
			excludedCols: []string{", meta\n", ", meta "},
		},
		{
			name:         "ListEventsMetaByRunSince",
			sql:          listEventsMetaByRunSince,
			excludedCols: []string{", meta\n", ", meta "},
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

// TestListMetaQueriesHaveDeterministicOrder verifies List*Meta queries
// have deterministic tie-breakers matching their corresponding List* queries.
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

		// artifact_bundles.sql - created_at DESC, id DESC
		{"ListArtifactBundlesMetaByRun", listArtifactBundlesMetaByRun, "ORDER BY created_at DESC, id DESC"},
		{"ListArtifactBundlesMetaByRunAndJob", listArtifactBundlesMetaByRunAndJob, "ORDER BY created_at DESC, id DESC"},
		{"ListArtifactBundlesMetaByCID", listArtifactBundlesMetaByCID, "ORDER BY created_at DESC, id DESC"},

		// events.sql - time ASC, id ASC
		{"ListEventsMetaByRun", listEventsMetaByRun, "ORDER BY time ASC, id ASC"},
		{"ListEventsMetaByRunSince", listEventsMetaByRunSince, "ORDER BY time ASC, id ASC"},
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
