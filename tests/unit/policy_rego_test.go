package unit

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/open-policy-agent/opa/ast"
)

// TestRegoPolicies_SyntaxValid ensures all .rego files under policies/ parse successfully.
func TestRegoPolicies_SyntaxValid(t *testing.T) {
	root := "policies"
	info, err := os.Stat(root)
	if err != nil {
		if os.IsNotExist(err) {
			t.Skip("no policies directory present")
			return
		}
		t.Fatalf("stat policies dir: %v", err)
	}
	if !info.IsDir() {
		t.Skip("policies exists but is not a directory")
	}

	var filesChecked int
	walkErr := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if filepath.Ext(path) != ".rego" {
			return nil
		}
		src, readErr := os.ReadFile(path)
		if readErr != nil {
			t.Fatalf("read %s: %v", path, readErr)
		}
		if _, parseErr := ast.ParseModule(path, string(src)); parseErr != nil {
			t.Fatalf("rego parse error in %s: %v", path, parseErr)
		}
		filesChecked++
		return nil
	})
	if walkErr != nil {
		t.Fatalf("walk policies: %v", walkErr)
	}
	if filesChecked == 0 {
		t.Skip("no rego files found under policies/")
	}
}

// TestWasmPolicy_HasKeyRules checks presence of key rules for WASM policy if the file exists.
func TestWasmPolicy_HasKeyRules(t *testing.T) {
	path := filepath.Join("policies", "wasm.rego")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			t.Skip("policies/wasm.rego not present")
			return
		}
		t.Fatalf("read wasm.rego: %v", err)
	}
	src := string(data)
	// Minimal presence checks mirroring shell script expectations
	if !strings.Contains(src, "allow_wasm_deployment") {
		t.Errorf("expected rule 'allow_wasm_deployment' in %s", path)
	}
	if !strings.Contains(src, "max_wasm_size_mb") {
		t.Errorf("expected variable or rule 'max_wasm_size_mb' in %s", path)
	}
}
