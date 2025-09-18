package build

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDetectBuildContextForcesDockerLane(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/test"), 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}

	lane, language, _, _, _ := detectBuildContext(dir, "G", "")
	if lane != "D" {
		t.Fatalf("lane = %s, want D", lane)
	}
	if language != "go" {
		t.Fatalf("language = %s, want go", language)
	}

	lane, _, _, _, _ = detectBuildContext(dir, "", "")
	if lane != "D" {
		t.Fatalf("lane without override = %s, want D", lane)
	}
}
