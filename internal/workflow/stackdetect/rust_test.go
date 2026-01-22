package stackdetect

import (
	"context"
	"path/filepath"
	"testing"
)

func TestDetectRust_Rust176Cargo(t *testing.T) {
	ctx := context.Background()
	workspace := filepath.Join("testdata", "rust", "rust176-cargo")

	obs, err := detectRust(ctx, workspace)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	assertObservation(t, obs, "rust", "cargo", "1.76")
	assertEvidence(t, obs, "rust-version", "1.76")
}

func TestDetectRust_Rust175Toolchain(t *testing.T) {
	ctx := context.Background()
	workspace := filepath.Join("testdata", "rust", "rust175-toolchain")

	obs, err := detectRust(ctx, workspace)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	assertObservation(t, obs, "rust", "cargo", "1.75")
	assertEvidence(t, obs, "channel", "1.75")
}

func TestDetectRust_StableToolchain(t *testing.T) {
	ctx := context.Background()
	workspace := filepath.Join("testdata", "rust", "stable-toolchain")

	_, err := detectRust(ctx, workspace)
	if err == nil {
		t.Fatal("expected error for stable toolchain")
	}

	detErr, ok := err.(*DetectionError)
	if !ok {
		t.Fatalf("expected DetectionError, got %T", err)
	}

	if !detErr.IsUnknown() {
		t.Errorf("expected reason 'unknown', got %q", detErr.Reason)
	}
}

func TestDetectRust_NightlyToolchain(t *testing.T) {
	ctx := context.Background()
	workspace := filepath.Join("testdata", "rust", "nightly-toolchain")

	_, err := detectRust(ctx, workspace)
	if err == nil {
		t.Fatal("expected error for nightly toolchain")
	}

	detErr, ok := err.(*DetectionError)
	if !ok {
		t.Fatalf("expected DetectionError, got %T", err)
	}

	if !detErr.IsUnknown() {
		t.Errorf("expected reason 'unknown', got %q", detErr.Reason)
	}
}
