package types

import (
	"encoding/json"
	"errors"
	"testing"
)

func TestIDs_Basics(t *testing.T) {
	t.Run("zero", func(t *testing.T) {
		var (
			a RunID
			d StepID
			e ClusterID
			g ModID
			h SpecID
			i ModRepoID
		)
		if !a.IsZero() || a.String() != "" {
			t.Fatalf("RunID zero failed")
		}
		if !d.IsZero() || d.String() != "" {
			t.Fatalf("StepID zero failed")
		}
		if !e.IsZero() || e.String() != "" {
			t.Fatalf("ClusterID zero failed")
		}
		if !g.IsZero() || g.String() != "" {
			t.Fatalf("ModID zero failed")
		}
		if !h.IsZero() || h.String() != "" {
			t.Fatalf("SpecID zero failed")
		}
		if !i.IsZero() || i.String() != "" {
			t.Fatalf("ModRepoID zero failed")
		}
	})

	t.Run("construct_compare", func(t *testing.T) {
		r1, r2 := RunID("r1"), RunID("r1")
		if r1 != r2 || r1.String() != "r1" {
			t.Fatalf("RunID compare/string failed")
		}
		st1, st2 := StepID("st1"), StepID("st1")
		if st1 != st2 || st1.String() != "st1" {
			t.Fatalf("StepID compare/string failed")
		}
		c1, c2 := ClusterID("c1"), ClusterID("c1")
		if c1 != c2 || c1.String() != "c1" {
			t.Fatalf("ClusterID compare/string failed")
		}
		m1, m2 := ModID("m1"), ModID("m1")
		if m1 != m2 || m1.String() != "m1" {
			t.Fatalf("ModID compare/string failed")
		}
		sp1, sp2 := SpecID("sp1"), SpecID("sp1")
		if sp1 != sp2 || sp1.String() != "sp1" {
			t.Fatalf("SpecID compare/string failed")
		}
		mr1, mr2 := ModRepoID("mr1"), ModRepoID("mr1")
		if mr1 != mr2 || mr1.String() != "mr1" {
			t.Fatalf("ModRepoID compare/string failed")
		}
	})

}

func TestIDs_TextAndJSONRoundTrip(t *testing.T) {
	// Use one representative value for each type.
	var (
		rid  RunID
		step StepID
		cid  ClusterID
		mid  ModID
		sid  SpecID
		mrid ModRepoID
	)

	// Test RunID text/JSON round-trip (covers run identifier serialization).
	runIDStr := NewRunID().String()
	if err := rid.UnmarshalText([]byte("  " + runIDStr + "  ")); err != nil {
		t.Fatalf("run UnmarshalText: %v", err)
	}
	if rid.String() != runIDStr {
		t.Fatalf("run normalize: %q", rid.String())
	}
	b, err := json.Marshal(rid)
	if err != nil {
		t.Fatalf("run marshal: %v", err)
	}
	if string(b) != "\""+runIDStr+"\"" {
		t.Fatalf("run json string expected, got %s", string(b))
	}
	var rid2 RunID
	if err := json.Unmarshal(b, &rid2); err != nil {
		t.Fatalf("run unmarshal json: %v", err)
	}
	if rid2 != rid {
		t.Fatalf("run roundtrip mismatch: %v != %v", rid2, rid)
	}

	// Test other ID types.
	if err := step.UnmarshalText([]byte(" step-1 ")); err != nil {
		t.Fatalf("step UnmarshalText: %v", err)
	}
	if err := cid.UnmarshalText([]byte(" c-1 ")); err != nil {
		t.Fatalf("cluster UnmarshalText: %v", err)
	}
	// Test v1 ID types text round-trip.
	modIDStr := NewModID().String()
	if err := mid.UnmarshalText([]byte(" " + modIDStr + " ")); err != nil {
		t.Fatalf("mod UnmarshalText: %v", err)
	}
	specIDStr := NewSpecID().String()
	if err := sid.UnmarshalText([]byte(" " + specIDStr + " ")); err != nil {
		t.Fatalf("spec UnmarshalText: %v", err)
	}
	modRepoIDStr := NewModRepoID().String()
	if err := mrid.UnmarshalText([]byte(" " + modRepoIDStr + " ")); err != nil {
		t.Fatalf("modrepo UnmarshalText: %v", err)
	}

	for name, v := range map[string]any{
		"run":     rid,
		"step":    step,
		"cluster": cid,
		"mod":     mid,
		"spec":    sid,
		"modrepo": mrid,
	} {
		bb, err := json.Marshal(v)
		if err != nil {
			t.Fatalf("%s marshal: %v", name, err)
		}
		if len(bb) == 0 || bb[0] != '"' {
			t.Fatalf("%s json must be string: %s", name, string(bb))
		}
	}
}

func TestIDs_RejectEmpty(t *testing.T) {
	// Verify all ID types reject empty/whitespace-only values.
	tests := []struct {
		name string
		fn   func([]byte) error
	}{
		{"run", func(b []byte) error { var v RunID; return v.UnmarshalText(b) }},
		{"step", func(b []byte) error { var v StepID; return v.UnmarshalText(b) }},
		{"cluster", func(b []byte) error { var v ClusterID; return v.UnmarshalText(b) }},
		{"mod", func(b []byte) error { var v ModID; return v.UnmarshalText(b) }},
		{"spec", func(b []byte) error { var v SpecID; return v.UnmarshalText(b) }},
		{"modrepo", func(b []byte) error { var v ModRepoID; return v.UnmarshalText(b) }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.fn([]byte("   ")); !errors.Is(err, ErrEmpty) {
				t.Fatalf("expected ErrEmpty, got %v", err)
			}
		})
	}
}

func TestIDs_RejectInvalidFormats(t *testing.T) {
	t.Parallel()

	t.Run("RunID", func(t *testing.T) {
		t.Parallel()
		var v RunID
		if err := v.UnmarshalText([]byte("abc123")); err == nil {
			t.Fatalf("expected error for invalid RunID, got nil")
		}
	})

	t.Run("JobID", func(t *testing.T) {
		t.Parallel()
		var v JobID
		if err := v.UnmarshalText([]byte("job123")); err == nil {
			t.Fatalf("expected error for invalid JobID, got nil")
		}
	})

	t.Run("NodeID", func(t *testing.T) {
		t.Parallel()
		var v NodeID
		if err := v.UnmarshalText([]byte("too-long")); err == nil {
			t.Fatalf("expected error for invalid NodeID, got nil")
		}
		if err := v.UnmarshalText([]byte("ab cd1")); err == nil {
			t.Fatalf("expected error for invalid NodeID chars, got nil")
		}
	})

	t.Run("ModID", func(t *testing.T) {
		t.Parallel()
		var v ModID
		if err := v.UnmarshalText([]byte("abcdefg")); err == nil {
			t.Fatalf("expected error for invalid ModID length, got nil")
		}
	})

	t.Run("SpecID", func(t *testing.T) {
		t.Parallel()
		var v SpecID
		if err := v.UnmarshalText([]byte("short")); err == nil {
			t.Fatalf("expected error for invalid SpecID length, got nil")
		}
	})

	t.Run("ModRepoID", func(t *testing.T) {
		t.Parallel()
		var v ModRepoID
		if err := v.UnmarshalText([]byte("short")); err == nil {
			t.Fatalf("expected error for invalid ModRepoID length, got nil")
		}
	})
}

// TestIDGenerators verifies the KSUID and NanoID-based ID generation helpers.
func TestIDGenerators(t *testing.T) {
	t.Run("NewRunID", func(t *testing.T) {
		// Verify non-empty output with expected KSUID length (27 characters).
		id := NewRunID()
		if id.IsZero() {
			t.Fatal("NewRunID returned zero value")
		}
		// KSUID string representation is always 27 characters.
		if len(id.String()) != 27 {
			t.Fatalf("NewRunID length = %d, want 27", len(id.String()))
		}

		// Verify multiple calls produce different values (probabilistic uniqueness).
		seen := make(map[RunID]struct{})
		for i := 0; i < 100; i++ {
			newID := NewRunID()
			if _, exists := seen[newID]; exists {
				t.Fatalf("NewRunID produced duplicate ID after %d calls", i)
			}
			seen[newID] = struct{}{}
		}
	})

	t.Run("NewJobID", func(t *testing.T) {
		// Verify non-empty output with expected KSUID length (27 characters).
		id := NewJobID()
		if id.IsZero() {
			t.Fatal("NewJobID returned zero value")
		}
		if len(id.String()) != 27 {
			t.Fatalf("NewJobID length = %d, want 27", len(id.String()))
		}

		// Verify multiple calls produce different values.
		seen := make(map[JobID]struct{})
		for i := 0; i < 100; i++ {
			newID := NewJobID()
			if _, exists := seen[newID]; exists {
				t.Fatalf("NewJobID produced duplicate ID after %d calls", i)
			}
			seen[newID] = struct{}{}
		}
	})

	t.Run("NewNodeKey", func(t *testing.T) {
		// Verify non-empty output with expected NanoID length (6 characters).
		id := NewNodeKey()
		if id == "" {
			t.Fatal("NewNodeKey returned empty string")
		}
		if len(id) != 6 {
			t.Fatalf("NewNodeKey length = %d, want 6", len(id))
		}

		// Verify characters are from the expected URL-safe alphabet.
		for _, c := range id {
			if !isURLSafeChar(c) {
				t.Fatalf("NewNodeKey contains invalid character: %c", c)
			}
		}

		// Verify multiple calls produce different values.
		seen := make(map[string]struct{})
		for i := 0; i < 100; i++ {
			newID := NewNodeKey()
			if _, exists := seen[newID]; exists {
				t.Fatalf("NewNodeKey produced duplicate ID after %d calls", i)
			}
			seen[newID] = struct{}{}
		}
	})

	// v1 ID generators: ModID, SpecID, ModRepoID use NanoID for compact, URL-safe identifiers.

	t.Run("NewModID", func(t *testing.T) {
		// Verify non-empty output with expected NanoID length (6 characters).
		// ModID uses NanoID(6) for mod project identifiers.
		id := NewModID()
		if id.IsZero() {
			t.Fatal("NewModID returned zero value")
		}
		if len(id.String()) != 6 {
			t.Fatalf("NewModID length = %d, want 6", len(id.String()))
		}

		// Verify characters are from the expected URL-safe alphabet.
		for _, c := range id.String() {
			if !isURLSafeChar(c) {
				t.Fatalf("NewModID contains invalid character: %c", c)
			}
		}

		// Verify multiple calls produce different values.
		seen := make(map[ModID]struct{})
		for i := 0; i < 100; i++ {
			newID := NewModID()
			if _, exists := seen[newID]; exists {
				t.Fatalf("NewModID produced duplicate ID after %d calls", i)
			}
			seen[newID] = struct{}{}
		}
	})

	t.Run("NewSpecID", func(t *testing.T) {
		// Verify non-empty output with expected NanoID length (8 characters).
		// SpecID uses NanoID(8) for spec identifiers in the append-only specs table.
		id := NewSpecID()
		if id.IsZero() {
			t.Fatal("NewSpecID returned zero value")
		}
		if len(id.String()) != 8 {
			t.Fatalf("NewSpecID length = %d, want 8", len(id.String()))
		}

		// Verify characters are from the expected URL-safe alphabet.
		for _, c := range id.String() {
			if !isURLSafeChar(c) {
				t.Fatalf("NewSpecID contains invalid character: %c", c)
			}
		}

		// Verify multiple calls produce different values.
		seen := make(map[SpecID]struct{})
		for i := 0; i < 100; i++ {
			newID := NewSpecID()
			if _, exists := seen[newID]; exists {
				t.Fatalf("NewSpecID produced duplicate ID after %d calls", i)
			}
			seen[newID] = struct{}{}
		}
	})

	t.Run("NewModRepoID", func(t *testing.T) {
		// Verify non-empty output with expected NanoID length (8 characters).
		// ModRepoID uses NanoID(8) for per-mod repository identifiers.
		id := NewModRepoID()
		if id.IsZero() {
			t.Fatal("NewModRepoID returned zero value")
		}
		if len(id.String()) != 8 {
			t.Fatalf("NewModRepoID length = %d, want 8", len(id.String()))
		}

		// Verify characters are from the expected URL-safe alphabet.
		for _, c := range id.String() {
			if !isURLSafeChar(c) {
				t.Fatalf("NewModRepoID contains invalid character: %c", c)
			}
		}

		// Verify multiple calls produce different values.
		seen := make(map[ModRepoID]struct{})
		for i := 0; i < 100; i++ {
			newID := NewModRepoID()
			if _, exists := seen[newID]; exists {
				t.Fatalf("NewModRepoID produced duplicate ID after %d calls", i)
			}
			seen[newID] = struct{}{}
		}
	})
}

// isURLSafeChar checks if a character is in the NanoID URL-safe alphabet.
// The alphabet is: 0-9, A-Z, a-z, _, -
func isURLSafeChar(c rune) bool {
	return (c >= '0' && c <= '9') ||
		(c >= 'A' && c <= 'Z') ||
		(c >= 'a' && c <= 'z') ||
		c == '_' || c == '-'
}
