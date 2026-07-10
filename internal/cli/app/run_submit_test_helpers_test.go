package app

import (
	"os"
	"path/filepath"
	"testing"
)

func writeRunSubmitSpec(t *testing.T, body string) string {
	t.Helper()
	specPath := filepath.Join(t.TempDir(), "spec.yaml")
	if err := os.WriteFile(specPath, []byte(body), 0o644); err != nil {
		t.Fatalf("write spec file: %v", err)
	}
	return specPath
}

func capturedSubmitSpec(t *testing.T, captured map[string]any) map[string]any {
	t.Helper()
	spec, ok := captured["spec"].(map[string]any)
	if !ok {
		t.Fatalf("expected spec object, got %#v", captured["spec"])
	}
	return spec
}

func capturedSubmitSteps(t *testing.T, captured map[string]any) []any {
	t.Helper()
	spec := capturedSubmitSpec(t, captured)
	steps, ok := spec["steps"].([]any)
	if !ok {
		t.Fatalf("expected spec steps array, got %#v", spec["steps"])
	}
	return steps
}

func capturedSubmitBuildGate(t *testing.T, captured map[string]any) map[string]any {
	t.Helper()
	spec := capturedSubmitSpec(t, captured)
	buildGate, ok := spec["build_gate"].(map[string]any)
	if !ok {
		t.Fatalf("expected build_gate object, got %#v", spec["build_gate"])
	}
	return buildGate
}
