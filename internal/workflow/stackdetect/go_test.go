package stackdetect

import (
	"context"
	"path/filepath"
	"testing"
)

func TestDetectGoMod_Go122(t *testing.T) {
	ctx := context.Background()
	workspace := filepath.Join("testdata", "go", "go122")
	goModPath := filepath.Join(workspace, "go.mod")

	obs, err := detectGo(ctx, workspace, goModPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	assertObservation(t, obs, "go", "go", "1.22")
	assertEvidence(t, obs, "go", "1.22")
}

func TestDetectGoMod_Go122Toolchain(t *testing.T) {
	ctx := context.Background()
	workspace := filepath.Join("testdata", "go", "go122-toolchain")
	goModPath := filepath.Join(workspace, "go.mod")

	obs, err := detectGo(ctx, workspace, goModPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	assertObservation(t, obs, "go", "go", "1.22")
	assertEvidence(t, obs, "go", "1.22")
	assertEvidence(t, obs, "toolchain", "1.22.5")
}

func TestDetectGoMod_NoGoDirective(t *testing.T) {
	ctx := context.Background()
	workspace := filepath.Join("testdata", "go", "no-go-directive")
	goModPath := filepath.Join(workspace, "go.mod")

	_, err := detectGo(ctx, workspace, goModPath)
	if err == nil {
		t.Fatal("expected error for missing go directive")
	}

	detErr, ok := err.(*DetectionError)
	if !ok {
		t.Fatalf("expected DetectionError, got %T", err)
	}

	if !detErr.IsUnknown() {
		t.Errorf("expected reason 'unknown', got %q", detErr.Reason)
	}
}
