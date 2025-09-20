package mods

import "testing"

func TestValidateRecipeCoords(t *testing.T) {
	// Valid
	if err := validateRecipeCoords("g", "a", "v", "s1", "test"); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	// Missing fields
	if err := validateRecipeCoords("", "a", "v", "s2", "test"); err == nil {
		t.Fatalf("expected error for missing group")
	}
	if err := validateRecipeCoords("g", "", "v", "s3", "test"); err == nil {
		t.Fatalf("expected error for missing artifact")
	}
	if err := validateRecipeCoords("g", "a", "", "s4", "test"); err == nil {
		t.Fatalf("expected error for missing version")
	}
}
