package store

import (
	"strings"
	"testing"
)

func TestSBOMInsertQuery_UsesConflictNoop(t *testing.T) {
	t.Parallel()

	want := "ON CONFLICT (job_id, repo_id, lib, ver) DO NOTHING"
	if !strings.Contains(normalizeWhitespace(upsertSBOMRow), normalizeWhitespace(want)) {
		t.Fatalf("UpsertSBOMRow must keep idempotent insert semantics; want %q in SQL:\n%s", want, upsertSBOMRow)
	}
}

func TestSBOMQuery_OrderDeterministic(t *testing.T) {
	t.Parallel()

	want := "ORDER BY lib ASC, ver ASC"
	if !containsOrderBy(listSBOMRowsByJob, want) {
		t.Fatalf("ListSBOMRowsByJob must have deterministic ordering; want %q in SQL:\n%s", want, listSBOMRowsByJob)
	}
}

func TestSBOMConstraint_PrimaryKeyDefinedInSchema(t *testing.T) {
	t.Parallel()

	schema := normalizeWhitespace(getSchemaSQL())
	wantTable := normalizeWhitespace("CREATE TABLE IF NOT EXISTS sboms")
	wantPK := normalizeWhitespace("PRIMARY KEY (job_id, repo_id, lib, ver)")
	if !strings.Contains(schema, wantTable) {
		t.Fatalf("schema missing sboms table definition")
	}
	if !strings.Contains(schema, wantPK) {
		t.Fatalf("schema missing sboms primary key definition")
	}
}
