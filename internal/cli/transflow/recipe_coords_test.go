package transflow

import "testing"

func TestValidateRecipeCoords(t *testing.T) {
	// Valid
	if err := validateRecipeCoords("g", "a", "v", "s1"); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	// Missing fields
	if err := validateRecipeCoords("", "a", "v", "s2"); err == nil {
		t.Fatalf("expected error for missing group")
	}
	if err := validateRecipeCoords("g", "", "v", "s3"); err == nil {
		t.Fatalf("expected error for missing artifact")
	}
	if err := validateRecipeCoords("g", "a", "", "s4"); err == nil {
		t.Fatalf("expected error for missing version")
	}
}
