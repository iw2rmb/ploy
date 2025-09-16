package mods

import (
	"testing"

	build "github.com/iw2rmb/ploy/internal/build"
)

func TestSandboxParser_ParseJavaMaven(t *testing.T) {
	raw := `
[ERROR] COMPILATION ERROR :
[ERROR] /workspace/src/healing/java/e2e/FailHealing.java:[4,9] cannot find symbol
[ERROR] /workspace/src/healing/java/e2e/FailHealing.java:[4,32] cannot find symbol
`
	got := build.ParseBuildErrors("java", "maven", raw)
	if len(got) == 0 {
		t.Fatalf("expected parsed errors, got none")
	}
	if got[0].File != "/workspace/src/healing/java/e2e/FailHealing.java" {
		t.Fatalf("unexpected file: %s", got[0].File)
	}
	if got[0].Line != 4 {
		t.Fatalf("unexpected line: %d", got[0].Line)
	}
	if got[0].Message == "" {
		t.Fatalf("expected message, got empty")
	}
}
