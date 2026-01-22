package stackdetect

import (
	"context"
	"path/filepath"
	"testing"
)

func TestDetectPython_Python311VersionFile(t *testing.T) {
	ctx := context.Background()
	workspace := filepath.Join("testdata", "python", "python311-version-file")

	obs, err := detectPython(ctx, workspace)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	assertObservation(t, obs, "python", "pip", "3.11")
	assertEvidence(t, obs, "python", "3.11")
}

func TestDetectPython_Python310Pyproject(t *testing.T) {
	ctx := context.Background()
	workspace := filepath.Join("testdata", "python", "python310-pyproject")

	obs, err := detectPython(ctx, workspace)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	assertObservation(t, obs, "python", "pip", "3.10")
	assertEvidence(t, obs, "requires-python", "3.10")
}

func TestDetectPython_Python311Poetry(t *testing.T) {
	ctx := context.Background()
	workspace := filepath.Join("testdata", "python", "python311-poetry")

	_, err := detectPython(ctx, workspace)
	if err == nil {
		t.Fatal("expected error for Poetry ^3.11 specifier")
	}

	detErr, ok := err.(*DetectionError)
	if !ok {
		t.Fatalf("expected DetectionError, got %T", err)
	}

	if !detErr.IsUnknown() {
		t.Errorf("expected reason 'unknown', got %q", detErr.Reason)
	}
}

func TestDetectPython_Python39Runtime(t *testing.T) {
	ctx := context.Background()
	workspace := filepath.Join("testdata", "python", "python39-runtime")

	obs, err := detectPython(ctx, workspace)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	assertObservation(t, obs, "python", "pip", "3.9")
	assertEvidence(t, obs, "python", "3.9")
}

func TestDetectPython_PythonRangeUnknown(t *testing.T) {
	ctx := context.Background()
	workspace := filepath.Join("testdata", "python", "python-range-unknown")

	_, err := detectPython(ctx, workspace)
	if err == nil {
		t.Fatal("expected error for open-ended range")
	}

	detErr, ok := err.(*DetectionError)
	if !ok {
		t.Fatalf("expected DetectionError, got %T", err)
	}

	if !detErr.IsUnknown() {
		t.Errorf("expected reason 'unknown', got %q", detErr.Reason)
	}
}

func TestDetectPython_PythonDisagreement(t *testing.T) {
	ctx := context.Background()
	workspace := filepath.Join("testdata", "python", "python-disagreement")

	_, err := detectPython(ctx, workspace)
	if err == nil {
		t.Fatal("expected error for conflicting versions")
	}

	detErr, ok := err.(*DetectionError)
	if !ok {
		t.Fatalf("expected DetectionError, got %T", err)
	}

	if !detErr.IsUnknown() {
		t.Errorf("expected reason 'unknown', got %q", detErr.Reason)
	}

	// Should have evidence from multiple sources.
	if len(detErr.Evidence) < 2 {
		t.Errorf("expected at least 2 evidence items for disagreement, got %d", len(detErr.Evidence))
	}
}
