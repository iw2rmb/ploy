package arf

import "testing"

func TestGenerateRecipeSuggestions(t *testing.T) {
	items := []catalogRecipe{
		{ID: "org.openrewrite.java.RemoveUnusedImports", DisplayName: "Remove Unused Imports"},
		{ID: "org.openrewrite.java.migrate.UpgradeToJava17", DisplayName: "Upgrade To Java 17"},
		{ID: "org.openrewrite.spring.UpgradeSpringBoot_3_2", DisplayName: "Upgrade Spring Boot 3.2"},
	}
	sugg := generateRecipeSuggestions("org.openrewrite.java.RemoveUnusedImport", items)
	if len(sugg) == 0 {
		t.Fatalf("expected suggestions")
	}
	if sugg[0] != "org.openrewrite.java.RemoveUnusedImports" {
		t.Fatalf("expected top suggestion RemoveUnusedImports, got %s", sugg[0])
	}
}
