package sbom

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSyftSBOMAnalyzer_AnalyzeSBOM_InvalidPath(t *testing.T) {
	a := NewSyftSBOMAnalyzer()
	if _, err := a.AnalyzeSBOM(""); err == nil {
		t.Fatalf("expected error for empty path")
	}
}

func TestSyftSBOMAnalyzer_AnalyzeSBOM_ValidJSON(t *testing.T) {
	a := NewSyftSBOMAnalyzer()
	dir := t.TempDir()
	p := filepath.Join(dir, "sbom.json")
	content := `{"artifacts":[{"name":"a","version":"1.0.0"},{"name":"b","version":"2.0.0"}]}`
	if err := os.WriteFile(p, []byte(content), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}
	res, err := a.AnalyzeSBOM(p)
	if err != nil {
		t.Fatalf("AnalyzeSBOM: %v", err)
	}
	if len(res.Dependencies) != 2 {
		t.Fatalf("expected 2 deps, got %d", len(res.Dependencies))
	}
}
