package main

import (
	"testing"
)

// FuzzBuildArtifactFilename ensures filename sanitization and suffix rules hold
// across a wide range of inputs.
func FuzzBuildArtifactFilename(f *testing.F) {
	// Seed with a few interesting cases.
	f.Add("plan", "dir/file.txt", "cid:abc/123", "sha256:deadbeef")
	f.Add("exec", "win\\path\\n", "cid-xyz", "")
	f.Add("", "..\\..//evil:name", "cid", "sha256:abcdef0123456789abcdef0123456789")

	f.Fuzz(func(t *testing.T, stage, name, cid, digest string) {
		got := buildArtifactFilename(stage, name, cid, digest)
		if got == "" {
			t.Fatalf("empty filename for inputs: %q %q %q %q", stage, name, cid, digest)
		}
		if !hasSuffix(got, ".bin") {
			t.Fatalf("filename %q must end with .bin", got)
		}
		// Disallow path separators and Windows-forbidden colon.
		for _, bad := range []rune{'/', '\\', ':'} {
			if containsRune(got, bad) {
				t.Fatalf("filename %q contains forbidden rune %q", got, string(bad))
			}
		}
	})
}

func hasSuffix(s, suf string) bool { return len(s) >= len(suf) && s[len(s)-len(suf):] == suf }
func containsRune(s string, r rune) bool {
	for _, c := range s {
		if c == r {
			return true
		}
	}
	return false
}
