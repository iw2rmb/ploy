//go:build nvdmods
// +build nvdmods

package mods

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// Test that runVulnerabilityGate is non-fatal when SBOM is missing
func TestRunVulnerabilityGate_NoSBOM_OK(t *testing.T) {
	dir := t.TempDir()
	r := &ModRunner{}
	if err := r.runVulnerabilityGate(context.Background(), dir); err != nil {
		t.Fatalf("expected nil error when no SBOM present, got %v", err)
	}
}

// Test that runVulnerabilityGate is non-fatal when NVD is unreachable
func TestRunVulnerabilityGate_UnreachableNVD_OK(t *testing.T) {
	dir := t.TempDir()
	// Create minimal syft-like SBOM
	sbom := map[string]interface{}{
		"artifacts": []map[string]interface{}{{
			"name":    "examplepkg",
			"version": "1.0.0",
		}},
	}
	b, _ := json.Marshal(sbom)
	if err := os.WriteFile(filepath.Join(dir, ".sbom.json"), b, 0644); err != nil {
		t.Fatalf("failed to write sbom: %v", err)
	}
	// Point NVD to an invalid endpoint to force request errors
	t.Setenv("NVD_BASE_URL", "http://127.0.0.1:0")
	r := &ModRunner{}
	if err := r.runVulnerabilityGate(context.Background(), dir); err != nil {
		t.Fatalf("expected nil error when NVD unreachable, got %v", err)
	}
}
