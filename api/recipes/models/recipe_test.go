package models

import (
	"testing"
)

func validOpenRewriteStep(name string) RecipeStep {
	return RecipeStep{
		Name: name,
		Type: StepTypeOpenRewrite,
		Config: map[string]interface{}{
			"recipe": "org.openrewrite.java.SpringBoot3Upgrade",
		},
	}
}

func TestRecipeGenerateIDAndSetSystemFields(t *testing.T) {
	recipe := &Recipe{
		Metadata: RecipeMetadata{
			Name:        "spring-upgrade",
			Version:     "1.2.3",
			Description: "Upgrade Spring dependencies",
		},
		Steps: []RecipeStep{validOpenRewriteStep("openrewrite")},
	}

	if id := recipe.GenerateID(); id != "spring-upgrade-1.2.3" {
		t.Fatalf("GenerateID() = %s, want spring-upgrade-1.2.3", id)
	}

	hash, err := recipe.CalculateHash()
	if err != nil {
		t.Fatalf("CalculateHash() error = %v", err)
	}

	recipe.SetSystemFields("tester@example.com")

	if recipe.UploadedBy != "tester@example.com" {
		t.Errorf("UploadedBy = %s, want tester@example.com", recipe.UploadedBy)
	}
	if recipe.ID != "spring-upgrade-1.2.3" {
		t.Errorf("ID = %s, want spring-upgrade-1.2.3", recipe.ID)
	}
	if recipe.Hash != hash {
		t.Errorf("Hash = %s, want %s", recipe.Hash, hash)
	}
	if recipe.Metadata.Version != "1.2.3" {
		t.Errorf("metadata version modified unexpectedly: %s", recipe.Metadata.Version)
	}
	if recipe.Version != "1.2.3" {
		t.Errorf("Version = %s, want 1.2.3", recipe.Version)
	}
	if recipe.CreatedAt.IsZero() || recipe.UpdatedAt.IsZero() {
		t.Errorf("system timestamps not populated: created=%v updated=%v", recipe.CreatedAt, recipe.UpdatedAt)
	}
}

func TestRecipeValidate(t *testing.T) {
	baseRecipe := Recipe{
		Metadata: RecipeMetadata{
			Name:        "spring-upgrade",
			Description: "Upgrade Spring dependencies",
		},
		Steps: []RecipeStep{validOpenRewriteStep("openrewrite")},
	}

	if err := baseRecipe.Validate(); err != nil {
		t.Fatalf("expected valid recipe, got error: %v", err)
	}

	invalidDesc := baseRecipe
	invalidDesc.Metadata.Description = ""
	if err := invalidDesc.Validate(); err == nil {
		t.Fatalf("expected error for missing description")
	}

	invalidStep := baseRecipe
	invalidStep.Steps = []RecipeStep{{Name: "shell", Type: StepTypeShellScript, Config: map[string]interface{}{}}}
	if err := invalidStep.Validate(); err == nil {
		t.Fatalf("expected step validation error for missing config")
	}
}

func TestRecipeGetRequiredTools(t *testing.T) {
	recipe := Recipe{
		Metadata: RecipeMetadata{Name: "tool-check", Description: "collect tools"},
		Steps: []RecipeStep{
			validOpenRewriteStep("openrewrite"),
			{
				Name:   "run-script",
				Type:   StepTypeShellScript,
				Config: map[string]interface{}{"script": "echo hello"},
			},
			{
				Name:   "ast-go",
				Type:   StepTypeASTTransform,
				Config: map[string]interface{}{"language": "go", "transform": "rename"},
			},
		},
	}

	tools := recipe.GetRequiredTools()
	expected := map[string]struct{}{"maven": {}, "java": {}, "bash": {}, "go": {}}

	if len(tools) != len(expected) {
		t.Fatalf("GetRequiredTools() returned %d tools, want %d", len(tools), len(expected))
	}

	for _, tool := range tools {
		if _, ok := expected[tool]; !ok {
			t.Fatalf("unexpected tool %q in result", tool)
		}
		delete(expected, tool)
	}

	if len(expected) != 0 {
		t.Fatalf("missing expected tools: %v", expected)
	}
}

func TestRecipeCloneProducesDeepCopy(t *testing.T) {
	recipe := &Recipe{
		Metadata: RecipeMetadata{Name: "clone-me", Description: "clone"},
		Steps:    []RecipeStep{validOpenRewriteStep("openrewrite")},
	}
	recipe.SetSystemFields("tester")

	cloned := recipe.Clone()
	if cloned == recipe {
		t.Fatalf("Clone() returned same pointer")
	}

	cloned.Metadata.Description = "modified"
	cloned.Steps[0].Config["recipe"] = "modified"

	if recipe.Metadata.Description == "modified" {
		t.Errorf("original recipe metadata mutated after clone")
	}
	if recipe.Steps[0].Config["recipe"] == "modified" {
		t.Errorf("original recipe step config mutated after clone")
	}
}
