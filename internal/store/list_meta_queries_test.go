package store

import (
	"strings"
	"testing"
)

// TestDiffSelectorBehavior validates the canonical diff list selector path:
// - uses explicit column selection (not SELECT *)
// - excludes the patch blob column to avoid unnecessary I/O
// - includes all metadata columns needed by callers (object_key for retrieval)
// - has deterministic ordering with id tie-breaker
func TestDiffSelectorBehavior(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name            string
		sql             string
		requiredColumns []string
		excludedCols    []string
		wantOrder       string
	}{
		{
			name:            "ListDiffsByRun",
			sql:             listDiffsByRun,
			requiredColumns: []string{"object_key", "patch_size", "summary", "created_at"},
			excludedCols:    []string{", patch,", ", patch "},
			wantOrder:       "ORDER BY created_at ASC, id ASC",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			sqlLower := strings.ToLower(tc.sql)
			if strings.Contains(sqlLower, "select *") || strings.Contains(sqlLower, "select d.*") {
				t.Fatalf("%s must use explicit column selection, not SELECT *; found wildcard in SQL:\n%s",
					tc.name, tc.sql)
			}
			for _, col := range tc.excludedCols {
				if strings.Contains(sqlLower, strings.ToLower(col)) {
					t.Fatalf("%s must NOT include blob column; found %q in SQL:\n%s",
						tc.name, col, tc.sql)
				}
			}
			for _, col := range tc.requiredColumns {
				if !strings.Contains(sqlLower, col) {
					t.Fatalf("%s selector must include column %q in SQL:\n%s",
						tc.name, col, tc.sql)
				}
			}
			if !containsOrderBy(tc.sql, tc.wantOrder) {
				t.Fatalf("%s must have deterministic ordering; want substring %q in SQL:\n%s",
					tc.name, tc.wantOrder, tc.sql)
			}
		})
	}
}

// TestArtifactSelectorBehavior validates the canonical artifact list selector path:
// - uses explicit column selection (not SELECT *)
// - includes object_key for object-storage retrieval (full-selection path)
// - includes all metadata columns needed by callers
// - has deterministic ordering with id tie-breaker
func TestArtifactSelectorBehavior(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name            string
		sql             string
		requiredColumns []string
		wantOrder       string
	}{
		{
			name:            "ListArtifactBundlesByRun",
			sql:             listArtifactBundlesByRun,
			requiredColumns: []string{"object_key", "bundle_size", "cid", "digest", "created_at"},
			wantOrder:       "ORDER BY created_at DESC, id DESC",
		},
		{
			name:            "ListArtifactBundlesByRunAndJob",
			sql:             listArtifactBundlesByRunAndJob,
			requiredColumns: []string{"object_key", "bundle_size", "cid", "digest", "created_at"},
			wantOrder:       "ORDER BY created_at DESC, id DESC",
		},
		{
			name:            "ListArtifactBundlesByCID",
			sql:             listArtifactBundlesByCID,
			requiredColumns: []string{"object_key", "bundle_size", "cid", "digest", "created_at"},
			wantOrder:       "ORDER BY created_at DESC, id DESC",
		},
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
			if strings.Contains(sqlLower, "::boolean or not $") {
				t.Fatalf("%s must not include tautological selector-flag boolean guard in SQL:\n%s",
					tc.name, tc.sql)
			}
			for _, col := range tc.requiredColumns {
				if !strings.Contains(sqlLower, col) {
					t.Fatalf("%s selector must include column %q in SQL:\n%s",
						tc.name, col, tc.sql)
				}
			}
			if !containsOrderBy(tc.sql, tc.wantOrder) {
				t.Fatalf("%s must have deterministic ordering; want substring %q in SQL:\n%s",
					tc.name, tc.wantOrder, tc.sql)
			}
		})
	}
}

// TestEventSelectorBehavior validates the canonical event list selector path:
// - uses explicit column selection (not SELECT *)
// - includes all event payload columns (level, message, meta, time)
// - has deterministic ordering with id tie-breaker
func TestEventSelectorBehavior(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name            string
		sql             string
		requiredColumns []string
		wantOrder       string
	}{
		{
			name:            "ListEventsByRun",
			sql:             listEventsByRun,
			requiredColumns: []string{"level", "message", "meta", "time"},
			wantOrder:       "ORDER BY time ASC, id ASC",
		},
		{
			name:            "ListEventsByRunSince",
			sql:             listEventsByRunSince,
			requiredColumns: []string{"level", "message", "meta", "time"},
			wantOrder:       "ORDER BY time ASC, id ASC",
		},
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
			if strings.Contains(sqlLower, "::boolean or not $") {
				t.Fatalf("%s must not include tautological selector-flag boolean guard in SQL:\n%s",
					tc.name, tc.sql)
			}
			for _, col := range tc.requiredColumns {
				if !strings.Contains(sqlLower, col) {
					t.Fatalf("%s selector must include column %q in SQL:\n%s",
						tc.name, col, tc.sql)
				}
			}
			if !containsOrderBy(tc.sql, tc.wantOrder) {
				t.Fatalf("%s must have deterministic ordering; want substring %q in SQL:\n%s",
					tc.name, tc.wantOrder, tc.sql)
			}
		})
	}
}

func TestCacheReplaySelectorExcludesMirroredCandidates(t *testing.T) {
	t.Parallel()

	sqlLower := strings.ToLower(resolveReusableJobByCacheKey)
	if !strings.Contains(sqlLower, "not (meta ? 'cache_mirror')") {
		t.Fatalf("ResolveReusableJobByCacheKey must exclude mirrored cache candidates; SQL:\n%s", resolveReusableJobByCacheKey)
	}
	if !strings.Contains(sqlLower, "status = 'fail'") ||
		!strings.Contains(sqlLower, "exit_code = 1") ||
		!strings.Contains(sqlLower, "exists") ||
		!strings.Contains(sqlLower, "from logs") {
		t.Fatalf("ResolveReusableJobByCacheKey must allow replaying failed candidates only for exit_code=1 with log rows; SQL:\n%s", resolveReusableJobByCacheKey)
	}
}
