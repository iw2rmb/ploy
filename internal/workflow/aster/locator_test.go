package aster

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestFilesystemLocatorReturnsBundleMetadata(t *testing.T) {
	root := t.TempDir()
	stageDir := filepath.Join(root, "mods")
	if err := os.MkdirAll(stageDir, 0o755); err != nil {
		t.Fatalf("failed to create stage directory: %v", err)
	}
	metadata := []byte(`{
		"bundle_id": "mods-plan-20250926",
		"digest": "sha256:deadbeef",
		"artifact_cid": "bafyplan",
		"source": "build/aster/mods-plan.tar.zst"
	}`)
	if err := os.WriteFile(filepath.Join(stageDir, "plan.json"), metadata, 0o644); err != nil {
		t.Fatalf("failed to write metadata file: %v", err)
	}

	locator, err := NewFilesystemLocator(root)
	if err != nil {
		t.Fatalf("unexpected error constructing locator: %v", err)
	}

	result, err := locator.Locate(context.Background(), Request{Stage: "mods", Toggle: "plan"})
	if err != nil {
		t.Fatalf("unexpected error locating bundle: %v", err)
	}
	if result.BundleID != "mods-plan-20250926" {
		t.Fatalf("unexpected bundle id: %s", result.BundleID)
	}
	if result.Digest != "sha256:deadbeef" {
		t.Fatalf("unexpected digest: %s", result.Digest)
	}
	if result.ArtifactCID != "bafyplan" {
		t.Fatalf("unexpected artifact cid: %s", result.ArtifactCID)
	}
	if result.Source != "build/aster/mods-plan.tar.zst" {
		t.Fatalf("unexpected source path: %s", result.Source)
	}
}

func TestFilesystemLocatorReportsMissingBundle(t *testing.T) {
	locator, err := NewFilesystemLocator(t.TempDir())
	if err != nil {
		t.Fatalf("unexpected error constructing locator: %v", err)
	}

	_, err = locator.Locate(context.Background(), Request{Stage: "mods", Toggle: "plan"})
	if err == nil {
		t.Fatal("expected error for missing bundle")
	}
	if err != ErrBundleNotFound {
		t.Fatalf("expected ErrBundleNotFound, got %v", err)
	}
}
