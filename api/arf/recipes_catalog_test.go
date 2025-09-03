package arf

import (
    "testing"
)

// Sample minimal recipe YAMLs as they would appear in META-INF/rewrite/*.yml
var sampleRecipeYAML1 = []byte(`
type: specs.openrewrite.org/v1beta/recipe
name: org.openrewrite.java.RemoveUnusedImports
displayName: Remove Unused Imports
description: Remove imports that are not used in Java source files.
tags:
  - java
`)

var sampleRecipeYAML2 = []byte(`
type: specs.openrewrite.org/v1beta/recipe
name: org.openrewrite.java.migrate.Java8toJava11
displayName: Java 8 to Java 11 Migration
description: Migrate Java code and build configuration from 8 to 11.
tags:
  - java
  - migrate
`)

func TestRecipesCatalog_BuildListSearchGet(t *testing.T) {
    cat := NewRecipesCatalog()
    if err := cat.BuildFromYAMLs([][]byte{sampleRecipeYAML1, sampleRecipeYAML2}, "rewrite-java", "2.20.0"); err != nil {
        t.Fatalf("build catalog failed: %v", err)
    }

    // List
    all := cat.List("", "", 10)
    if len(all) != 2 {
        t.Fatalf("expected 2 recipes, got %d", len(all))
    }

    // Search
    got := cat.Search("migration", 10)
    if len(got) != 1 || got[0].ID != "org.openrewrite.java.migrate.Java8toJava11" {
        t.Fatalf("expected search to return Java8toJava11, got %#v", got)
    }

    // Get by ID
    r := cat.GetByID("org.openrewrite.java.RemoveUnusedImports")
    if r == nil || r.DisplayName != "Remove Unused Imports" {
        t.Fatalf("expected RemoveUnusedImports metadata, got %#v", r)
    }
}

