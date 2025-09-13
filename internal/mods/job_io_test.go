package mods

import "testing"

func TestValidateArtifactKey_Valid(t *testing.T) {
	cases := []string{
		"transflow/exec-123/input.tar",
		"transflow/abc/branches/x/steps/y/diff.patch",
		"/transflow/leading/slash/is/ok",
	}
	for _, c := range cases {
		if err := validateArtifactKey(c); err != nil {
			t.Fatalf("expected valid key %q, got error: %v", c, err)
		}
	}
}

func TestValidateArtifactKey_Invalid(t *testing.T) {
	cases := []string{
		"",
		"../escape",
		"transflow/../escape",
		"otherns/key",
		"transflow\\backslash",
	}
	for _, c := range cases {
		if err := validateArtifactKey(c); err == nil {
			t.Fatalf("expected error for invalid key %q, got nil", c)
		}
	}
}
