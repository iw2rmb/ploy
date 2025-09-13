package mods

import "testing"

func TestValidateArtifactKey_Valid(t *testing.T) {
	cases := []string{
		"mods/exec-123/input.tar",
		"mods/abc/branches/x/steps/y/diff.patch",
		"/mods/leading/slash/is/ok",
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
		"mods/../escape",
		"otherns/key",
		"mods\\backslash",
	}
	for _, c := range cases {
		if err := validateArtifactKey(c); err == nil {
			t.Fatalf("expected error for invalid key %q, got nil", c)
		}
	}
}
