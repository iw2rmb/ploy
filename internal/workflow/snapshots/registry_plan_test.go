package snapshots

import (
	"context"
	"path/filepath"
	"testing"
)

func TestPlanSummarisesRules(t *testing.T) {
	dir := t.TempDir()
	fixturePath := filepath.Join(dir, "dev-db.json")
	fixture := map[string][]map[string]string{
		"users": {
			{"id": "1", "email": "alice@example.com", "ssn": "123-45-6789"},
			{"id": "2", "email": "bob@example.com", "ssn": "987-65-4321"},
		},
	}
	writeJSON(t, fixturePath, fixture)

	specPath := filepath.Join(dir, "dev-db.toml")
	writeFile(t, specPath, `name = "dev-db"
description = "Development database"
[source]
engine = "postgres"
dsn = "postgres://dev"
fixture = "`+fixturePath+`"

[[strip]]
table = "users"
columns = ["ssn"]

[[mask]]
table = "users"
column = "email"
strategy = "hash"

[[synthetic]]
table = "users"
column = "token"
strategy = "uuid"
`)

	registry, err := LoadDirectory(dir, LoadOptions{})
	if err != nil {
		t.Fatalf("load directory: %v", err)
	}

	report, err := registry.Plan(context.Background(), "dev-db")
	if err != nil {
		t.Fatalf("plan dev-db: %v", err)
	}

	if report.SnapshotName != "dev-db" {
		t.Fatalf("expected snapshot name dev-db, got %s", report.SnapshotName)
	}
	if report.Engine != "postgres" {
		t.Fatalf("expected engine postgres, got %s", report.Engine)
	}
	if report.Stripping.Total != 1 || report.Masking.Total != 1 || report.Synthetic.Total != 1 {
		t.Fatalf("unexpected rule totals: %+v", report)
	}
	if got := report.Masking.Tables["users"]; got != 1 {
		t.Fatalf("expected masking count 1 for users, got %d", got)
	}
	if len(report.Highlights) == 0 {
		t.Fatalf("expected highlights, got none")
	}
}

func TestPlanErrorsForUnknownSnapshot(t *testing.T) {
	registry := &Registry{specs: map[string]Spec{}}
	if _, err := registry.Plan(context.Background(), "missing"); err == nil {
		t.Fatal("expected error for missing snapshot")
	}
}
