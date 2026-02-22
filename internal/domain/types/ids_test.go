package types

import (
	"encoding"
	"encoding/json"
	"errors"
	"testing"
)

// testStringID runs common tests for a StringID type: zero value, construct/compare,
// text/JSON round-trip, and empty rejection.
func testStringID[T any](t *testing.T, name string, validValue string) {
	t.Helper()

	t.Run("zero", func(t *testing.T) {
		var v StringID[T]
		if !v.IsZero() {
			t.Fatalf("%s zero IsZero() = false", name)
		}
		if v.String() != "" {
			t.Fatalf("%s zero String() = %q", name, v.String())
		}
	})

	t.Run("construct_compare", func(t *testing.T) {
		a, b := StringID[T](validValue), StringID[T](validValue)
		if a != b || a.String() != validValue {
			t.Fatalf("%s compare/string failed", name)
		}
	})

	t.Run("text_roundtrip", func(t *testing.T) {
		v := StringID[T](validValue)
		b, err := v.MarshalText()
		if err != nil {
			t.Fatalf("%s MarshalText: %v", name, err)
		}
		var v2 StringID[T]
		if err := v2.UnmarshalText(b); err != nil {
			t.Fatalf("%s UnmarshalText: %v", name, err)
		}
		if v2 != v {
			t.Fatalf("%s text roundtrip: %v != %v", name, v2, v)
		}
	})

	t.Run("json_roundtrip", func(t *testing.T) {
		v := StringID[T](validValue)
		b, err := json.Marshal(v)
		if err != nil {
			t.Fatalf("%s json.Marshal: %v", name, err)
		}
		if len(b) == 0 || b[0] != '"' {
			t.Fatalf("%s json must be string: %s", name, string(b))
		}
		var v2 StringID[T]
		if err := json.Unmarshal(b, &v2); err != nil {
			t.Fatalf("%s json.Unmarshal: %v", name, err)
		}
		if v2 != v {
			t.Fatalf("%s json roundtrip: %v != %v", name, v2, v)
		}
	})

	t.Run("reject_empty", func(t *testing.T) {
		var v StringID[T]
		if err := v.UnmarshalText([]byte("   ")); !errors.Is(err, ErrEmpty) {
			t.Fatalf("%s expected ErrEmpty, got %v", name, err)
		}
	})

	// Verify interface compliance.
	var v StringID[T]
	_ = encoding.TextMarshaler(v)
	_ = encoding.TextUnmarshaler(&v)
}

func TestStringIDs(t *testing.T) {
	t.Run("RunID", func(t *testing.T) { testStringID[runIDTag](t, "RunID", NewRunID().String()) })
	t.Run("StepID", func(t *testing.T) { testStringID[stepIDTag](t, "StepID", "step-1") })
	t.Run("JobID", func(t *testing.T) { testStringID[jobIDTag](t, "JobID", NewJobID().String()) })
	t.Run("ClusterID", func(t *testing.T) { testStringID[clusterIDTag](t, "ClusterID", "c-1") })
	t.Run("NodeID", func(t *testing.T) { testStringID[nodeIDTag](t, "NodeID", NewNodeKey()) })
	t.Run("ModID", func(t *testing.T) { testStringID[modIDTag](t, "ModID", NewModID().String()) })
	t.Run("SpecID", func(t *testing.T) { testStringID[specIDTag](t, "SpecID", NewSpecID().String()) })
	t.Run("ModRepoID", func(t *testing.T) { testStringID[modRepoIDTag](t, "ModRepoID", NewModRepoID().String()) })
}

func TestIDs_RejectInvalidFormats(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		fn    func([]byte) error
		input string
	}{
		{"RunID", func(b []byte) error { var v RunID; return v.UnmarshalText(b) }, "abc123"},
		{"JobID", func(b []byte) error { var v JobID; return v.UnmarshalText(b) }, "job123"},
		{"NodeID_long", func(b []byte) error { var v NodeID; return v.UnmarshalText(b) }, "too-long"},
		{"NodeID_chars", func(b []byte) error { var v NodeID; return v.UnmarshalText(b) }, "ab cd1"},
		{"ModID", func(b []byte) error { var v ModID; return v.UnmarshalText(b) }, "abcdefg"},
		{"SpecID", func(b []byte) error { var v SpecID; return v.UnmarshalText(b) }, "short"},
		{"ModRepoID", func(b []byte) error { var v ModRepoID; return v.UnmarshalText(b) }, "short"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if err := tt.fn([]byte(tt.input)); err == nil {
				t.Fatalf("expected error for invalid input %q, got nil", tt.input)
			}
		})
	}
}

func TestIDGenerators(t *testing.T) {
	tests := []struct {
		name    string
		genStr  func() string
		wantLen int
	}{
		{"NewRunID", func() string { return NewRunID().String() }, 27},
		{"NewJobID", func() string { return NewJobID().String() }, 27},
		{"NewNodeKey", func() string { return NewNodeKey() }, 6},
		{"NewModID", func() string { return NewModID().String() }, 6},
		{"NewSpecID", func() string { return NewSpecID().String() }, 8},
		{"NewModRepoID", func() string { return NewModRepoID().String() }, 8},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			id := tt.genStr()
			if id == "" {
				t.Fatalf("%s returned empty", tt.name)
			}
			if len(id) != tt.wantLen {
				t.Fatalf("%s length = %d, want %d", tt.name, len(id), tt.wantLen)
			}

			// Verify uniqueness over 100 calls.
			seen := make(map[string]struct{})
			for i := 0; i < 100; i++ {
				newID := tt.genStr()
				if _, exists := seen[newID]; exists {
					t.Fatalf("%s produced duplicate after %d calls", tt.name, i)
				}
				seen[newID] = struct{}{}
			}
		})
	}
}

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

func TestDiffID_UnmarshalText(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		var id DiffID
		if err := id.UnmarshalText([]byte("550e8400-e29b-41d4-a716-446655440000")); err != nil {
			t.Fatalf("UnmarshalText() error = %v", err)
		}
		if id.String() != "550e8400-e29b-41d4-a716-446655440000" {
			t.Fatalf("got %q", id.String())
		}
	})

	t.Run("empty", func(t *testing.T) {
		var id DiffID
		if err := id.UnmarshalText([]byte("   ")); !errors.Is(err, ErrEmpty) {
			t.Fatalf("expected ErrEmpty, got %v", err)
		}
	})

	t.Run("invalid", func(t *testing.T) {
		var id DiffID
		if err := id.UnmarshalText([]byte("not-a-uuid")); !errors.Is(err, ErrInvalidDiffID) {
			t.Fatalf("expected ErrInvalidDiffID, got %v", err)
		}
	})
}

func TestDiffID_JSONRoundtrip(t *testing.T) {
	type payload struct {
		ID DiffID `json:"id"`
	}

	in := payload{ID: DiffID("550e8400-e29b-41d4-a716-446655440000")}
	data, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}

	var out payload
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if out.ID != in.ID {
		t.Fatalf("roundtrip mismatch: got %q want %q", out.ID, in.ID)
	}
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
