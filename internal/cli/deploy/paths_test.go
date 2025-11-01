package deploycli

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestDefaultIdentityPath(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	got := DefaultIdentityPath()
	want := filepath.Join(home, ".ssh", "id_rsa")
	if got != want {
		t.Fatalf("DefaultIdentityPath got %q want %q", got, want)
	}
}

func TestDefaultPloydBinaryPathFindsAdjacent(t *testing.T) {
	execPath, err := os.Executable()
	if err != nil {
		t.Skip("no executable path")
	}
	dir := filepath.Dir(execPath)
	// Create a candidate ployd binary next to the test executable.
	name := "ployd"
	if runtime.GOOS == "windows" {
		name = "ployd.exe"
	}
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte("bin"), 0o755); err != nil {
		t.Skipf("cannot write adjacent binary: %v", err)
	}
	defer func() { _ = os.Remove(path) }()
	got, err := DefaultPloydBinaryPath("")
	if err != nil || got == "" {
		t.Fatalf("DefaultPloydBinaryPath error=%v path=%q", err, got)
	}
}
