package store

import (
	"reflect"
	"strings"
	"testing"
)

func TestSplitSQLStatements_Simple(t *testing.T) {
	in := "CREATE SCHEMA IF NOT EXISTS ploy; CREATE TABLE t(a int);  -- trailing comment\n"
	got := splitSQLStatements(in)
	want := []string{
		"CREATE SCHEMA IF NOT EXISTS ploy",
		"CREATE TABLE t(a int)",
		"-- trailing comment",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("split mismatch\nwant: %#v\n got: %#v", want, got)
	}
}

func TestSplitSQLStatements_QuotesAndComments(t *testing.T) {
	in := `
        -- preface; with semicolon in comment;
        INSERT INTO t(a, b) VALUES ('x; y', "z; w");
        /* block; comment; with; semicolons; */
        UPDATE t SET a = 'abc'';def' WHERE b = 1; -- inline comment; here;
    `
	got := splitSQLStatements(in)
	// Expect two executable statements (INSERT + UPDATE); comments may be attached.
	var hasInsert, hasUpdate bool
	for _, s := range got {
		if strings.Contains(s, "INSERT INTO t") {
			hasInsert = true
		}
		if strings.Contains(s, "UPDATE t SET") {
			hasUpdate = true
		}
	}
	if !hasInsert || !hasUpdate {
		t.Fatalf("missing split around quotes/comments: %#v", got)
	}
}

func TestSplitSQLStatements_DollarQuotes(t *testing.T) {
	in := `
        CREATE OR REPLACE FUNCTION test_fn() RETURNS void AS $$
        BEGIN
            PERFORM 1; -- semicolon inside function
        END;
        $$ LANGUAGE plpgsql;

        CREATE OR REPLACE FUNCTION test_fn2() RETURNS text AS $tag$
        BEGIN
            RETURN 'ok; still ok';
        END;
        $tag$ LANGUAGE plpgsql;
    `
	got := splitSQLStatements(in)
	if len(got) != 2 {
		t.Fatalf("expected 2 statements, got %d: %#v", len(got), got)
	}
}
