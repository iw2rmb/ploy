package arf

import (
	"encoding/json"
	"testing"
)

func TestParseCatalogList(t *testing.T) {
	payload := []map[string]interface{}{
		{"id": "org.openrewrite.java.RemoveUnusedImports", "display_name": "Remove Unused Imports", "description": "Removes unused imports", "pack": "rewrite-java", "version": "2.20.0"},
		{"id": "org.openrewrite.java.migrate.UpgradeToJava17", "display_name": "Upgrade To Java 17", "description": "Upgrade Java version", "pack": "rewrite-migrate-java", "version": "2.20.0"},
	}
	b, _ := json.Marshal(payload)
	items, err := parseCatalogList(b)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}
	if items[0].ID != "org.openrewrite.java.RemoveUnusedImports" {
		t.Fatalf("unexpected first id: %s", items[0].ID)
	}
}
