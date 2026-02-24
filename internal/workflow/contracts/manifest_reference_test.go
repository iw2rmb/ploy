package contracts

import (
	"encoding/json"
	"testing"
)

func TestManifestReferenceValidate(t *testing.T) {
	t.Run("rejects empty name", func(t *testing.T) {
		m := ManifestReference{}
		if err := m.Validate(); err == nil {
			t.Fatal("expected error when name is empty")
		}
	})

	t.Run("rejects empty version", func(t *testing.T) {
		m := ManifestReference{Name: "repo"}
		if err := m.Validate(); err == nil {
			t.Fatal("expected error when version is empty")
		}
	})

	t.Run("accepts valid name and version", func(t *testing.T) {
		m := ManifestReference{Name: "repo", Version: "1.0.0"}
		if err := m.Validate(); err != nil {
			t.Fatalf("unexpected validate error: %v", err)
		}
	})
}

func TestManifestReferenceJSONStable(t *testing.T) {
	want := ManifestReference{Name: "smoke", Version: "2025-09-26"}
	payload, err := json.Marshal(want)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got ManifestReference
	if err := json.Unmarshal(payload, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got != want {
		t.Fatalf("roundtrip mismatch: got %+v want %+v", got, want)
	}
}

func TestStageNameJSONRoundtrip(t *testing.T) {
	var want StageName = "migs-plan"
	payload, err := json.Marshal(want)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got StageName
	if err := json.Unmarshal(payload, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got != want {
		t.Fatalf("roundtrip mismatch: got %q want %q", got, want)
	}
}
