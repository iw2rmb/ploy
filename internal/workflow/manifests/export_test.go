package manifests

// Test-only exports for external test package (manifests_test).

// ExportCompileOptions is a test-only alias for compileOptions.
type ExportCompileOptions = compileOptions

// ExportLoadDirectory exposes loadDirectory for tests that need to test error paths.
var ExportLoadDirectory = loadDirectory

// ExportCompileFromDir loads a directory and compiles with the given options.
func ExportCompileFromDir(dir string, opts compileOptions) (Compilation, error) {
	r, err := loadDirectory(dir)
	if err != nil {
		return Compilation{}, err
	}
	return r.compileManifest(opts)
}
