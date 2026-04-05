package stackdetect

import (
	"context"
	"path/filepath"
	"testing"
)

func TestDetectPython_Success(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		workspace    string
		wantTool     string
		wantRelease  string
		evidenceKey  string
		evidenceVal  string
	}{
		{
			name:        "version file",
			workspace:   filepath.Join("testdata", "python", "python311-version-file"),
			wantTool:    "pip",
			wantRelease: "3.11",
			evidenceKey: "python",
			evidenceVal: "3.11",
		},
		{
			name:        "pyproject requires-python",
			workspace:   filepath.Join("testdata", "python", "python310-pyproject"),
			wantTool:    "pip",
			wantRelease: "3.10",
			evidenceKey: "requires-python",
			evidenceVal: "3.10",
		},
		{
			name:        "runtime version",
			workspace:   filepath.Join("testdata", "python", "python39-runtime"),
			wantTool:    "pip",
			wantRelease: "3.9",
			evidenceKey: "python",
			evidenceVal: "3.9",
		},
	}

	ctx := context.Background()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			obs, err := detectPython(ctx, scanWorkspace(tt.workspace))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			assertObservation(t, obs, "python", tt.wantTool, tt.wantRelease)
			assertEvidence(t, obs, tt.evidenceKey, tt.evidenceVal)
		})
	}
}

func TestDetectPython_Unknown(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		workspace   string
		minEvidence int
	}{
		{
			name:      "poetry caret specifier",
			workspace: filepath.Join("testdata", "python", "python311-poetry"),
		},
		{
			name:      "open-ended range",
			workspace: filepath.Join("testdata", "python", "python-range-unknown"),
		},
		{
			name:        "conflicting versions",
			workspace:   filepath.Join("testdata", "python", "python-disagreement"),
			minEvidence: 2,
		},
	}

	ctx := context.Background()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := detectPython(ctx, scanWorkspace(tt.workspace))
			detErr := assertDetectionError(t, err, "unknown")
			if tt.minEvidence > 0 && len(detErr.Evidence) < tt.minEvidence {
				t.Errorf("expected at least %d evidence items, got %d", tt.minEvidence, len(detErr.Evidence))
			}
		})
	}
}
