package packs_test

import (
	"errors"
	"path/filepath"
	"testing"

	"os"

	"github.com/iw2rmb/ploy/internal/recipes/packs"
)

func writePackSpec(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
	return path
}

func TestLoadDirectoryAndFindByLanguage(t *testing.T) {
	dir := t.TempDir()

	writePackSpec(t, dir, "java-default.toml", `name = "java-default"
description = "Default Java recipe packs"
default = true
languages = ["java"]

[[packs]]
id = "rewrite-java"
version = "7.40.0"

[[packs]]
id = "rewrite-migrate-java"
version = "7.40.0"
`)

	writePackSpec(t, dir, "kotlin-gradle.toml", `name = "kotlin-gradle"
description = "Kotlin/Gradle recipes"
languages = ["kotlin", "gradle"]

[[packs]]
id = "rewrite-kotlin"
version = "1.2.3"
`)

	registry, err := packs.LoadDirectory(dir)
	if err != nil {
		t.Fatalf("LoadDirectory: %v", err)
	}

	lists := registry.List()
	if len(lists) != 2 {
		t.Fatalf("expected 2 pack lists, got %d", len(lists))
	}
	if lists[0].Name != "java-default" || lists[1].Name != "kotlin-gradle" {
		t.Fatalf("expected sorted pack list names, got %q", []string{lists[0].Name, lists[1].Name})
	}

	java, ok := registry.Get("java-default")
	if !ok {
		t.Fatalf("expected java-default in registry")
	}
	if !java.Default {
		t.Fatalf("java-default should be default pack list")
	}
	if len(java.Packs) != 2 {
		t.Fatalf("expected java pack list to contain 2 packs, got %d", len(java.Packs))
	}

	kotlinLists := registry.FindByLanguage("kotlin")
	if len(kotlinLists) != 1 || kotlinLists[0].Name != "kotlin-gradle" {
		t.Fatalf("expected kotlin-gradle for kotlin language, got %+v", kotlinLists)
	}

	none := registry.FindByLanguage("ruby")
	if len(none) != 0 {
		t.Fatalf("expected no pack lists for ruby, got %d", len(none))
	}

	defaultList, err := registry.Default()
	if err != nil {
		t.Fatalf("Default: %v", err)
	}
	if defaultList.Name != "java-default" {
		t.Fatalf("expected java-default as default, got %s", defaultList.Name)
	}
}

func TestLoadDirectoryRejectsInvalidSpecs(t *testing.T) {
	dir := t.TempDir()

	writePackSpec(t, dir, "invalid.toml", `name = "invalid"
`)

	_, err := packs.LoadDirectory(dir)
	if !errors.Is(err, packs.ErrInvalidSpec) {
		t.Fatalf("expected ErrInvalidSpec, got %v", err)
	}
}

func TestLoadDirectoryRejectsDuplicateNames(t *testing.T) {
	dir := t.TempDir()

	writePackSpec(t, dir, "a.toml", `name = "dup"
languages = ["java"]

[[packs]]
id = "rewrite-java"
version = "7.40.0"
`)

	writePackSpec(t, dir, "b.toml", `name = "dup"
languages = ["java"]

[[packs]]
id = "rewrite-java"
version = "7.40.0"
`)

	_, err := packs.LoadDirectory(dir)
	if err == nil {
		t.Fatalf("expected error for duplicate names")
	}
}

func TestLoadDirectoryRejectsMultipleDefaults(t *testing.T) {
	dir := t.TempDir()

	writePackSpec(t, dir, "a.toml", `name = "one"
default = true
languages = ["java"]

[[packs]]
id = "rewrite-java"
version = "7.40.0"
`)

	writePackSpec(t, dir, "b.toml", `name = "two"
default = true
languages = ["kotlin"]

[[packs]]
id = "rewrite-kotlin"
version = "1.2.3"
`)

	_, err := packs.LoadDirectory(dir)
	if !errors.Is(err, packs.ErrInvalidSpec) {
		t.Fatalf("expected ErrInvalidSpec for multiple defaults, got %v", err)
	}
}
