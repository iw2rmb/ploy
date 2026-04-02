package handlers

import "testing"

func TestCanonicalizeAndHashJSON_Deterministic(t *testing.T) {
	left := map[string]any{
		"b": map[string]any{"y": 2, "x": 1},
		"a": []any{map[string]any{"k2": "v2", "k1": "v1"}},
	}
	right := map[string]any{
		"a": []any{map[string]any{"k1": "v1", "k2": "v2"}},
		"b": map[string]any{"x": 1, "y": 2},
	}

	leftJSON, leftHash, err := canonicalizeAndHashJSON(left)
	if err != nil {
		t.Fatalf("canonicalizeAndHashJSON(left) error = %v", err)
	}
	rightJSON, rightHash, err := canonicalizeAndHashJSON(right)
	if err != nil {
		t.Fatalf("canonicalizeAndHashJSON(right) error = %v", err)
	}

	if string(leftJSON) != string(rightJSON) {
		t.Fatalf("canonical JSON mismatch:\nleft:  %s\nright: %s", leftJSON, rightJSON)
	}
	if leftHash != rightHash {
		t.Fatalf("hash mismatch: left=%s right=%s", leftHash, rightHash)
	}
}

func TestMigStepIndexFromJobNameForClaim(t *testing.T) {
	tests := []struct {
		name      string
		jobName   string
		stepsLen  int
		want      int
		shouldErr bool
	}{
		{name: "single step defaults to zero", jobName: "mig-0", stepsLen: 1, want: 0},
		{name: "multi step parses index", jobName: "mig-2", stepsLen: 3, want: 2},
		{name: "invalid prefix", jobName: "step-1", stepsLen: 3, shouldErr: true},
		{name: "out of range", jobName: "mig-3", stepsLen: 3, shouldErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := migStepIndexFromJobNameForClaim(tc.jobName, tc.stepsLen)
			if tc.shouldErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("migStepIndexFromJobNameForClaim() error = %v", err)
			}
			if got != tc.want {
				t.Fatalf("index = %d, want %d", got, tc.want)
			}
		})
	}
}
