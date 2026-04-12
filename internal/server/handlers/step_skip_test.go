package handlers

import (
	"testing"

	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

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

func TestMigStepIndexForCacheKey(t *testing.T) {
	metaWithIndex := func(idx int) []byte {
		meta := contracts.NewMigJobMeta()
		meta.MigStepIndex = &idx
		raw, _ := contracts.MarshalJobMeta(meta)
		return raw
	}

	tests := []struct {
		name      string
		meta      []byte
		stepsLen  int
		want      int
		shouldErr bool
	}{
		{name: "single step defaults to zero", meta: nil, stepsLen: 1, want: 0},
		{name: "multi step parses index from meta", meta: metaWithIndex(2), stepsLen: 3, want: 2},
		{name: "missing meta for multi step", meta: nil, stepsLen: 3, shouldErr: true},
		{name: "out of range", meta: metaWithIndex(3), stepsLen: 3, shouldErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := migStepIndexForCacheKey(tc.meta, tc.stepsLen)
			if tc.shouldErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("migStepIndexForCacheKey() error = %v", err)
			}
			if got != tc.want {
				t.Fatalf("index = %d, want %d", got, tc.want)
			}
		})
	}
}
