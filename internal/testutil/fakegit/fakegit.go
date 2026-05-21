package fakegit

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

type Capture struct {
	Dir      string
	ArgsPath string
	EnvPath  string
}

func Install(t testing.TB, stdout string) Capture {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("shell script fake git is not portable to windows")
	}

	dir := t.TempDir()
	binDir := filepath.Join(dir, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatalf("create bin dir: %v", err)
	}

	capture := Capture{
		Dir:      dir,
		ArgsPath: filepath.Join(dir, "args.txt"),
		EnvPath:  filepath.Join(dir, "env.txt"),
	}

	script := "#!/bin/sh\nprintf '%s\\n' \"$@\" > \"$CAPTURE_ARGS\"\nenv > \"$CAPTURE_ENV\"\n"
	if stdout != "" {
		script += "printf '%s\\n' '" + stdout + "'\n"
	}
	if err := os.WriteFile(filepath.Join(binDir, "git"), []byte(script), 0o755); err != nil {
		t.Fatalf("write fake git: %v", err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("CAPTURE_ARGS", capture.ArgsPath)
	t.Setenv("CAPTURE_ENV", capture.EnvPath)
	return capture
}

func Read(t testing.TB, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(b)
}
