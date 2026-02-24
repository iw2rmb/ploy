package manifests_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/iw2rmb/ploy/internal/workflow/manifests"
)

// writeManifest writes the provided manifest body into dir/name and fails the test if it cannot.
func writeManifest(t *testing.T, dir, name, body string) string {
	t.Helper()

	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	return path
}

// compileManifest loads and compiles the manifest from dir with the given options.
func compileManifest(t *testing.T, dir string, opts manifests.ExportCompileOptions) manifests.Compilation {
	t.Helper()

	compiled, err := manifests.ExportCompileFromDir(dir, opts)
	if err != nil {
		t.Fatalf("compile manifest: %v", err)
	}

	return compiled
}

// copyManifestFromTestdata copies a manifest fixture from testdata into dir so it can be loaded.
func copyManifestFromTestdata(t *testing.T, dir, filename string) string {
	t.Helper()

	contents, err := os.ReadFile(filepath.Join("testdata", filename))
	if err != nil {
		t.Fatalf("read testdata manifest: %v", err)
	}

	return writeManifest(t, dir, filename, string(contents))
}
