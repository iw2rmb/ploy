package mods

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteORWPreHCL_SubstitutesAll(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "orw_apply.rendered.hcl")
	tpl := strings.Join([]string{
		"class=\"${RECIPE_CLASS}\"",
		"group=\"${RECIPE_GROUP}\"",
		"artifact=\"${RECIPE_ARTIFACT}\"",
		"version=\"${RECIPE_VERSION}\"",
		"plugin=\"${MAVEN_PLUGIN_VERSION}\"",
		"input=\"${INPUT_TAR_HOST_PATH}\"",
		"run=\"${RUN_ID}\"",
		"",
	}, "\n")
	if err := os.WriteFile(in, []byte(tpl), 0644); err != nil {
		t.Fatal(err)
	}

	params := ORWRecipeParams{
		Class:         "org.openrewrite.java.migrate.UpgradeToJava17",
		Group:         "org.openrewrite.recipe",
		Artifact:      "rewrite-migrate-java",
		Version:       "3.17.0",
		PluginVersion: "6.18.0",
	}
	pre, err := writeORWPreHCL(in, params, "/tmp/input.tar", "run-123")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	b, _ := os.ReadFile(pre)
	s := string(b)
	mustContain(t, s, params.Class)
	mustContain(t, s, params.Group)
	mustContain(t, s, params.Artifact)
	mustContain(t, s, params.Version)
	mustContain(t, s, params.PluginVersion)
	mustContain(t, s, "/tmp/input.tar")
	mustContain(t, s, "run-123")
}
