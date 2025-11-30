package types

import "testing"

func TestLabelsForRun(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		var rid RunID
		if m := LabelsForRun(rid); len(m) != 0 {
			t.Fatalf("expected empty map, got %v", m)
		}
	})
	t.Run("value", func(t *testing.T) {
		rid := RunID("run-123")
		m := LabelsForRun(rid)
		if len(m) != 1 {
			t.Fatalf("expected single label, got %v", m)
		}
		if got := m[LabelRunID]; got != rid.String() {
			t.Fatalf("label %q=%q, want %q", LabelRunID, got, rid.String())
		}
	})
}

func TestLabelsForStep(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		var sid StepID
		if m := LabelsForStep(sid); len(m) != 0 {
			t.Fatalf("expected empty map, got %v", m)
		}
	})
	t.Run("value", func(t *testing.T) {
		sid := StepID("step-build")
		m := LabelsForStep(sid)
		if len(m) != 1 {
			t.Fatalf("expected single label, got %v", m)
		}
		if got := m[LabelJobID]; got != sid.String() {
			t.Fatalf("label %q=%q, want %q", LabelJobID, got, sid.String())
		}
	})
}
