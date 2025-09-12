package transflow

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestPreviewTarEntries_Truncates(t *testing.T) {
	dir := t.TempDir()
	// Create files
	files := []string{"a.txt", "b.txt", "c.txt"}
	for _, f := range files {
		if err := os.WriteFile(filepath.Join(dir, f), []byte("x"), 0644); err != nil {
			t.Fatalf("write file: %v", err)
		}
	}
	// Create tar
	tarPath := filepath.Join(dir, "input.tar")
	args := append([]string{"-cf", tarPath}, files...)
	cmd := exec.Command("tar", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("tar create: %v: %s", err, string(out))
	}
	// Preview first 2
	entries, err := previewTarEntries(tarPath, 2)
	if err != nil {
		t.Fatalf("preview err: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d (%v)", len(entries), entries)
	}
}
