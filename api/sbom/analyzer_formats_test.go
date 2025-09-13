package sbom

import (
	"encoding/json"
	"os"
	"testing"
)

func TestExtractDependencies_Syft(t *testing.T) {
	a := NewSyftSBOMAnalyzer()
	data := map[string]interface{}{
		"artifacts": []interface{}{
			map[string]interface{}{"name": "express", "version": "4.18.0"},
			map[string]interface{}{"name": "lodash", "version": "4.17.20"},
		},
	}
	deps, err := a.ExtractDependencies(data)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(deps) != 2 {
		t.Fatalf("want 2 deps, got %d", len(deps))
	}
}

func TestExtractDependencies_CycloneDX(t *testing.T) {
	a := NewSyftSBOMAnalyzer()
	data := map[string]interface{}{
		"components": []interface{}{
			map[string]interface{}{"name": "react", "version": "17.0.2"},
			map[string]interface{}{"name": "vite", "version": "5.0.0"},
		},
	}
	deps, err := a.ExtractDependencies(data)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(deps) != 2 {
		t.Fatalf("want 2 deps, got %d", len(deps))
	}
}

func TestExtractDependencies_SPDX(t *testing.T) {
	a := NewSyftSBOMAnalyzer()
	data := map[string]interface{}{
		"packages": []interface{}{
			map[string]interface{}{"name": "flask", "versionInfo": "2.0.1"},
			map[string]interface{}{"name": "jinja2", "versionInfo": "3.0.0"},
		},
	}
	deps, err := a.ExtractDependencies(data)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(deps) != 2 {
		t.Fatalf("want 2 deps, got %d", len(deps))
	}
}

func TestAnalyzeSBOM_IntegrationLike(t *testing.T) {
	a := NewSyftSBOMAnalyzer()
	src := map[string]interface{}{
		"artifacts": []interface{}{
			map[string]interface{}{"name": "a", "version": "1"},
			map[string]interface{}{"name": "b", "version": "2"},
		},
	}
	b, _ := json.Marshal(src)
	t.Setenv("TMPDIR", t.TempDir())
	path := t.TempDir() + "/sbom.json"
	if err := os.WriteFile(path, b, 0644); err != nil {
		t.Fatalf("write: %v", err)
	}
	res, err := a.AnalyzeSBOM(path)
	if err != nil {
		t.Fatalf("AnalyzeSBOM: %v", err)
	}
	if len(res.Dependencies) != 2 {
		t.Fatalf("want 2 deps, got %d", len(res.Dependencies))
	}
}
