package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	bsmock "github.com/iw2rmb/ploy/internal/blobstore/mock"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

type fakeGateCatalogSeedStore struct {
	nextStackID        int64
	nextDefaultID      int64
	stackInsertCount   int
	defaultInsertCount int
	stacks             map[string]fakeSeedStackRow
	defaultProfiles    map[int64]fakeDefaultProfileRow
}

type fakeSeedStackRow struct {
	ID      int64
	Lang    string
	Release string
	Tool    string
	Image   string
}

type fakeDefaultProfileRow struct {
	ID        int64
	StackID   int64
	ObjectKey string
}

func newFakeGateCatalogSeedStore() *fakeGateCatalogSeedStore {
	return &fakeGateCatalogSeedStore{
		stacks:          map[string]fakeSeedStackRow{},
		defaultProfiles: map[int64]fakeDefaultProfileRow{},
	}
}

func (f *fakeGateCatalogSeedStore) UpsertStackBySelector(_ context.Context, lang, release, tool, image string) (int64, error) {
	key := fmt.Sprintf("%s|%s|%s", lang, release, tool)
	if row, exists := f.stacks[key]; exists {
		row.Image = image
		f.stacks[key] = row
		return row.ID, nil
	}
	f.nextStackID++
	f.stackInsertCount++
	row := fakeSeedStackRow{
		ID:      f.nextStackID,
		Lang:    lang,
		Release: release,
		Tool:    tool,
		Image:   image,
	}
	f.stacks[key] = row
	return row.ID, nil
}

func (f *fakeGateCatalogSeedStore) UpsertDefaultGateProfile(_ context.Context, stackID int64, objectKey string) (int64, error) {
	if row, exists := f.defaultProfiles[stackID]; exists {
		row.ObjectKey = objectKey
		f.defaultProfiles[stackID] = row
		return row.ID, nil
	}
	f.nextDefaultID++
	f.defaultInsertCount++
	row := fakeDefaultProfileRow{
		ID:        f.nextDefaultID,
		StackID:   stackID,
		ObjectKey: objectKey,
	}
	f.defaultProfiles[stackID] = row
	return row.ID, nil
}

func TestSeedGateCatalogDefaults_IdempotentReseed(t *testing.T) {
	t.Setenv("PLOY_CONTAINER_REGISTRY", "registry.test.local/ploy")

	root := t.TempDir()
	catalogPath := filepath.Join(root, "gates", "stacks.yaml")
	profileJava := filepath.Join(root, "gates", "profiles", "java-17-maven.yaml")
	profileGo := filepath.Join(root, "gates", "profiles", "go-1.25.8.yaml")
	writeGateProfileYAML(t, profileJava, "default", "java", "maven", "17", "mvn -q test")
	writeGateProfileYAML(t, profileGo, "default", "go", "go", "1.25", "go test ./...")

	catalog := `stacks:
  - lang: java
    release: "17"
    tool: maven
    image: $PLOY_CONTAINER_REGISTRY/mig-${stack.language}-${stack.release}-${stack.tool}:latest
    profile: gates/profiles/java-17-maven.yaml
  - lang: go
    release: "1.25.8"
    image: $PLOY_CONTAINER_REGISTRY/${stack.language}:${stack.release}
    profile: profiles/go-1.25.8.yaml
`
	if err := os.MkdirAll(filepath.Dir(catalogPath), 0o755); err != nil {
		t.Fatalf("mkdir catalog dir: %v", err)
	}
	if err := os.WriteFile(catalogPath, []byte(catalog), 0o644); err != nil {
		t.Fatalf("write catalog: %v", err)
	}

	fakeStore := newFakeGateCatalogSeedStore()
	blobStore := bsmock.New()

	for i := 0; i < 2; i++ {
		if err := seedGateCatalogDefaults(context.Background(), fakeStore, blobStore, catalogPath); err != nil {
			t.Fatalf("seedGateCatalogDefaults() run=%d: %v", i+1, err)
		}
	}

	if got, want := fakeStore.stackInsertCount, 2; got != want {
		t.Fatalf("stack inserts=%d, want %d", got, want)
	}
	if got, want := fakeStore.defaultInsertCount, 2; got != want {
		t.Fatalf("default gate profile inserts=%d, want %d", got, want)
	}
	if got, want := blobStore.Count(), 2; got != want {
		t.Fatalf("blob objects=%d, want %d", got, want)
	}

	wantKeys := map[string]bool{
		"gate-profiles/defaults/java/17/maven/profile.json":     false,
		"gate-profiles/defaults/go/1.25.8/default/profile.json": false,
	}
	for _, row := range fakeStore.defaultProfiles {
		if _, ok := wantKeys[row.ObjectKey]; ok {
			wantKeys[row.ObjectKey] = true
		}
		data, exists := blobStore.GetData(row.ObjectKey)
		if !exists {
			t.Fatalf("blob for %q not found", row.ObjectKey)
		}
		if _, err := contracts.ParseGateProfileJSON(data); err != nil {
			t.Fatalf("blob %q is not a valid gate_profile JSON: %v", row.ObjectKey, err)
		}
	}
	for key, seen := range wantKeys {
		if !seen {
			t.Fatalf("expected object key %q to be seeded", key)
		}
	}

	javaRow, ok := fakeStore.stacks["java|17|maven"]
	if !ok {
		t.Fatal("java stack row missing")
	}
	if got, want := javaRow.Image, "registry.test.local/ploy/mig-java-17-maven:latest"; got != want {
		t.Fatalf("java image=%q, want %q", got, want)
	}
	goRow, ok := fakeStore.stacks["go|1.25.8|"]
	if !ok {
		t.Fatal("go stack row missing")
	}
	if got, want := goRow.Image, "registry.test.local/ploy/go:1.25.8"; got != want {
		t.Fatalf("go image=%q, want %q", got, want)
	}
}

func TestSeedGateCatalogDefaults_UnresolvedEnvInImageFails(t *testing.T) {
	root := t.TempDir()
	catalogPath := filepath.Join(root, "gates", "stacks.yaml")
	profileJava := filepath.Join(root, "gates", "profiles", "java-17-maven.yaml")
	writeGateProfileYAML(t, profileJava, "default", "java", "maven", "17", "mvn -q test")
	catalog := `stacks:
  - lang: java
    release: "17"
    tool: maven
    image: $PLOY_TEST_UNSET_REGISTRY/mig:${stack.release}
    profile: gates/profiles/java-17-maven.yaml
`
	if err := os.MkdirAll(filepath.Dir(catalogPath), 0o755); err != nil {
		t.Fatalf("mkdir catalog dir: %v", err)
	}
	if err := os.WriteFile(catalogPath, []byte(catalog), 0o644); err != nil {
		t.Fatalf("write catalog: %v", err)
	}

	err := seedGateCatalogDefaults(context.Background(), newFakeGateCatalogSeedStore(), bsmock.New(), catalogPath)
	if err == nil {
		t.Fatal("expected unresolved env error")
	}
	if !strings.Contains(err.Error(), "unresolved environment variables: PLOY_TEST_UNSET_REGISTRY") {
		t.Fatalf("error=%q, want unresolved env message", err.Error())
	}
}

func TestSeedGateCatalogDefaults_MissingProfileFileFails(t *testing.T) {
	root := t.TempDir()
	catalogPath := filepath.Join(root, "gates", "stacks.yaml")
	catalog := `stacks:
  - lang: java
    release: "17"
    tool: maven
    image: $PLOY_CONTAINER_REGISTRY/maven:3-eclipse-temurin-17
    profile: gates/profiles/missing.yaml
`
	if err := os.MkdirAll(filepath.Dir(catalogPath), 0o755); err != nil {
		t.Fatalf("mkdir catalog dir: %v", err)
	}
	if err := os.WriteFile(catalogPath, []byte(catalog), 0o644); err != nil {
		t.Fatalf("write catalog: %v", err)
	}

	err := seedGateCatalogDefaults(context.Background(), newFakeGateCatalogSeedStore(), bsmock.New(), catalogPath)
	if err == nil {
		t.Fatal("expected error for missing profile file")
	}
	if !strings.Contains(err.Error(), "referenced file does not exist") {
		t.Fatalf("error=%q, want missing profile message", err.Error())
	}
}

func writeGateProfileYAML(
	t *testing.T,
	path string,
	repoID string,
	lang string,
	tool string,
	release string,
	command string,
) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir profile dir: %v", err)
	}
	content := fmt.Sprintf(`schema_version: 1
repo_id: %s
runner_mode: simple
stack:
  language: %s
  tool: %s
  release: "%s"
targets:
  active: all_tests
  build:
    status: not_attempted
    env: {}
  unit:
    status: not_attempted
    env: {}
  all_tests:
    status: passed
    command: %s
    env: {}
orchestration:
  pre: []
  post: []
`, repoID, lang, tool, release, command)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write profile %q: %v", path, err)
	}
}
