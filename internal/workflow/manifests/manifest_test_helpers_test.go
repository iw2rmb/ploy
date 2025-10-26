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

// loadRegistry loads all manifests from dir using the production loader.
func loadRegistry(t *testing.T, dir string) *manifests.Registry {
	t.Helper()

	registry, err := manifests.LoadDirectory(dir)
	if err != nil {
		t.Fatalf("load manifests: %v", err)
	}

	return registry
}

// compileManifest compiles the registry manifest specified by opts and fails the test if compilation fails.
func compileManifest(t *testing.T, dir string, opts manifests.CompileOptions) manifests.Compilation {
	t.Helper()

	registry := loadRegistry(t, dir)
	compiled, err := registry.Compile(opts)
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
