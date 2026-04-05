package stackdetect

import (
	"context"
	"path/filepath"
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
