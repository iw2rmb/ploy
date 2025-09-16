package mods

import "testing"

func TestBuildSignatureLockKey(t *testing.T) {
	got := BuildSignatureLockKey("java", "abcd1234")
	// Expect posix-style path join semantics
	if got != "java/abcd1234" {
		t.Fatalf("BuildSignatureLockKey() = %q, want %q", got, "java/abcd1234")
	}
}
