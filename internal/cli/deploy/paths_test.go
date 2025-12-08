package deploycli

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
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

func TestDefaultPloydBinaryPathErrorMessageMentionsClusterDeploy(t *testing.T) {
	// Use a synthetic workstation OS to avoid platform-specific candidate paths.
	// If a ployd binary happens to exist adjacent to the test binary, skip.
	got, err := DefaultPloydBinaryPath("test-os")
	if err == nil {
		t.Skipf("DefaultPloydBinaryPath unexpectedly succeeded with path %q; skipping error message test", got)
	}
	if !strings.Contains(err.Error(), "ploy cluster deploy") {
		t.Fatalf("error message %q does not mention 'ploy cluster deploy'", err.Error())
	}
}
