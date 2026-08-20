package stackdetect

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDetectRust_Success(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		workspace   string
		wantRelease string
		evidenceKey string
		evidenceVal string
	}{
		{
			name:        "cargo rust-version",
			workspace:   filepath.Join("testdata", "rust", "rust176-cargo"),
			wantRelease: "1.76",
			evidenceKey: "rust-version",
			evidenceVal: "1.76",
		},
		{
			name:        "toolchain channel",
			workspace:   filepath.Join("testdata", "rust", "rust175-toolchain"),
			wantRelease: "1.75",
			evidenceKey: "channel",
			evidenceVal: "1.75",
		},
	}

	ctx := context.Background()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			obs, err := detectRust(ctx, tt.workspace)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			assertObservation(t, obs, "rust", "cargo", tt.wantRelease)
			assertEvidence(t, obs, tt.evidenceKey, tt.evidenceVal)
		})
	}
}

func TestDetectRustToolchainFormats(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		file        string
		content     string
		wantRelease string
		wantErr     string
	}{
		{name: "TOML numeric", file: "rust-toolchain.toml", content: "[toolchain]\nchannel = \"1.78.1\"\n", wantRelease: "1.78"},
		{name: "plain numeric", file: "rust-toolchain", content: "1.79.2\n", wantRelease: "1.79"},
		{name: "TOML non-numeric", file: "rust-toolchain.toml", content: "[toolchain]\nchannel = \"custom\"\n", wantErr: "rust-toolchain.toml specifies non-numeric channel"},
		{name: "plain non-deterministic", file: "rust-toolchain", content: "stable\n", wantErr: "rust-toolchain specifies non-deterministic channel"},
		{name: "plain non-numeric", file: "rust-toolchain", content: "custom\n", wantErr: "no rust-version in Cargo.toml"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			workspace := t.TempDir()
			if err := os.WriteFile(filepath.Join(workspace, tt.file), []byte(tt.content), 0o600); err != nil {
				t.Fatalf("write %s: %v", tt.file, err)
			}
			obs, err := detectRust(context.Background(), workspace)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("detectRust() error = %v, want containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("detectRust() error: %v", err)
			}
			assertObservation(t, obs, "rust", "cargo", tt.wantRelease)
			assertEvidence(t, obs, "channel", tt.wantRelease)
		})
	}
}

func TestDetectRust_Unknown(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		workspace string
	}{
		{"stable toolchain", filepath.Join("testdata", "rust", "stable-toolchain")},
		{"nightly toolchain", filepath.Join("testdata", "rust", "nightly-toolchain")},
	}

	ctx := context.Background()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := detectRust(ctx, tt.workspace)
			assertDetectionError(t, err, "unknown")
		})
	}
}
