package knowledgebase

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestMergeCatalogAppendsIncidents(t *testing.T) {
	base := Catalog{
		SchemaVersion: "2025-09-27.1",
		Incidents:     []Incident{{ID: "base", Summary: "base", Errors: []string{"base"}}},
	}
	incoming := Catalog{
		SchemaVersion: "2025-09-27.1",
		Incidents:     []Incident{{ID: "incoming", Summary: "incoming", Errors: []string{"incoming"}}},
	}
	merged, err := MergeCatalog(base, incoming)
	if err != nil {
		t.Fatalf("merge catalog: %v", err)
	}
	if merged.SchemaVersion != "2025-09-27.1" {
		t.Fatalf("expected schema version preserved, got %s", merged.SchemaVersion)
	}
	if len(merged.Incidents) != 2 {
		t.Fatalf("expected merged incidents length 2, got %d", len(merged.Incidents))
	}
	if merged.Incidents[0].ID != "base" || merged.Incidents[1].ID != "incoming" {
		t.Fatalf("expected incidents sorted by id, got %#v", merged.Incidents)
	}
}

func TestMergeCatalogRejectsDuplicate(t *testing.T) {
	base := Catalog{Incidents: []Incident{{ID: "dup"}}}
	incoming := Catalog{Incidents: []Incident{{ID: "dup"}}}
	_, err := MergeCatalog(base, incoming)
	if err == nil {
		t.Fatalf("expected duplicate to error")
	}
	if !errors.Is(err, ErrDuplicateIncident) {
		t.Fatalf("expected duplicate error, got %v", err)
	}
}

func TestSaveCatalogFileWritesAtomically(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "catalog.json")
	catalog := Catalog{
		SchemaVersion: "2025-09-27.1",
		Incidents:     []Incident{{ID: "one", Summary: "summary"}},
	}
	if err := SaveCatalogFile(path, catalog); err != nil {
		t.Fatalf("save catalog: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat catalog: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("expected catalog perm 0600, got %v", info.Mode().Perm())
	}
	loaded, err := LoadCatalogFile(path)
	if err != nil {
		t.Fatalf("reload catalog: %v", err)
	}
	if len(loaded.Incidents) != 1 || loaded.Incidents[0].ID != "one" {
		t.Fatalf("expected incident persisted, got %#v", loaded.Incidents)
	}
}

func TestSaveCatalogFileRejectsEmptyPath(t *testing.T) {
	if err := SaveCatalogFile("", Catalog{}); err == nil {
		t.Fatalf("expected error for empty path")
	}
}

func TestSaveCatalogFileFailsWhenDirectoryNotWritable(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("chmod semantics differ on Windows")
	}
	if os.Geteuid() == 0 {
		t.Skip("permission checks require non-root user")
	}
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Skipf("unable to adjust permissions: %v", err)
	}
	defer func() {
		_ = os.Chmod(dir, 0o700)
	}()
	path := filepath.Join(dir, "catalog.json")
	err := SaveCatalogFile(path, Catalog{})
	if err == nil {
		t.Fatalf("expected permission error when directory not writable")
	}
}

func TestSaveCatalogFileHandlesCreateTempError(t *testing.T) {
	defer func(prev func(string, string) (*os.File, error)) { createTemp = prev }(createTemp)
	createTemp = func(string, string) (*os.File, error) {
		return nil, errors.New("create temp failure")
	}
	err := SaveCatalogFile(filepath.Join(t.TempDir(), "catalog.json"), Catalog{})
	if err == nil || !strings.Contains(err.Error(), "create temp catalog") {
		t.Fatalf("expected create temp error, got %v", err)
	}
}

func TestSaveCatalogFileHandlesRenameError(t *testing.T) {
	defer func(prev func(string, string) error) { renameFile = prev }(renameFile)
	var called bool
	renameFile = func(oldpath, newpath string) error {
		called = true
		return errors.New("rename failure")
	}
	err := SaveCatalogFile(filepath.Join(t.TempDir(), "catalog.json"), Catalog{})
	if err == nil || !strings.Contains(err.Error(), "replace catalog") {
		t.Fatalf("expected rename error, got %v", err)
	}
	if !called {
		t.Fatalf("expected rename to be attempted")
	}
}

func TestSaveCatalogFileHandlesChmodError(t *testing.T) {
	defer func(prev func(string, os.FileMode) error) { chmodFile = prev }(chmodFile)
	chmodFile = func(string, os.FileMode) error {
		return errors.New("chmod failure")
	}
	err := SaveCatalogFile(filepath.Join(t.TempDir(), "catalog.json"), Catalog{})
	if err == nil || !strings.Contains(err.Error(), "chmod temp catalog") {
		t.Fatalf("expected chmod error, got %v", err)
	}
}
