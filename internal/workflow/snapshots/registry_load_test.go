package snapshots

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
)

func TestLoadDirectoryResolvesRelativeFixture(t *testing.T) {
	dir := t.TempDir()
	fixtureName := "fixture.json"
	fixturePath := filepath.Join(dir, fixtureName)
	writeJSON(t, fixturePath, map[string][]map[string]string{"users": {
		{"id": "1"},
	}})
	specPath := filepath.Join(dir, "snapshot.toml")
	writeFile(t, specPath, `name = "relative"
[source]
engine = "postgres"
dsn = "postgres://dev"
fixture = "`+fixtureName+`"
`)

	registry, err := LoadDirectory(dir, LoadOptions{})
	if err != nil {
		t.Fatalf("load directory: %v", err)
	}

	report, err := registry.Plan(context.Background(), "relative")
	if err != nil {
		t.Fatalf("plan: %v", err)
	}

	if !filepath.IsAbs(report.FixturePath) {
		t.Fatalf("expected absolute fixture path, got %s", report.FixturePath)
	}
}

func TestLoadDirectoryRejectsDuplicateNames(t *testing.T) {
	dir := t.TempDir()
	fixturePath := filepath.Join(dir, "fixture.json")
	writeJSON(t, fixturePath, map[string][]map[string]string{"users": {
		{"id": "1"},
	}})
	content := `name = "dup"
[source]
engine = "postgres"
dsn = "postgres://dev"
fixture = "` + fixturePath + `"
`
	writeFile(t, filepath.Join(dir, "a.toml"), content)
	writeFile(t, filepath.Join(dir, "b.toml"), content)

	if _, err := LoadDirectory(dir, LoadOptions{}); err == nil {
		t.Fatal("expected error for duplicate snapshot names")
	}
}

func TestValidateSpecRequiresFields(t *testing.T) {
	if err := validateSpec(Spec{}); err == nil {
		t.Fatal("expected error for missing name")
	}
	if err := validateSpec(Spec{Name: "demo"}); err == nil {
		t.Fatal("expected error for missing engine")
	}
	if err := validateSpec(Spec{Name: "demo", Source: Source{Engine: "postgres"}}); err == nil {
		t.Fatal("expected error for missing fixture")
	}
}

func TestGetSpecHandlesEmptyAndNilRegistry(t *testing.T) {
	registry := &Registry{specs: map[string]Spec{}}
	if _, err := registry.getSpec(""); !errors.Is(err, ErrSnapshotNotFound) {
		t.Fatalf("expected ErrSnapshotNotFound, got %v", err)
	}
	var nilRegistry *Registry
	if _, err := nilRegistry.getSpec("anything"); !errors.Is(err, ErrSnapshotNotFound) {
		t.Fatalf("expected ErrSnapshotNotFound for nil registry, got %v", err)
	}
}
