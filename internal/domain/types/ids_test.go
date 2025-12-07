package types

import (
	"encoding/json"
	"errors"
	"testing"
)

func TestIDs_Basics(t *testing.T) {
	t.Run("zero", func(t *testing.T) {
		var (
			a TicketID
			b RunID
			d StepID
			e ClusterID
			f RunRepoID
		)
		if !a.IsZero() || a.String() != "" {
			t.Fatalf("TicketID zero failed")
		}
		if !b.IsZero() || b.String() != "" {
			t.Fatalf("RunID zero failed")
		}
		if !d.IsZero() || d.String() != "" {
			t.Fatalf("StepID zero failed")
		}
		if !e.IsZero() || e.String() != "" {
			t.Fatalf("ClusterID zero failed")
		}
		if !f.IsZero() || f.String() != "" {
			t.Fatalf("RunRepoID zero failed")
		}
	})

	t.Run("construct_compare", func(t *testing.T) {
		a1, a2 := TicketID("t1"), TicketID("t1")
		if a1 != a2 || a1.String() != "t1" {
			t.Fatalf("TicketID compare/string failed")
		}
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
		rr1, rr2 := RunRepoID("rr1"), RunRepoID("rr1")
		if rr1 != rr2 || rr1.String() != "rr1" {
			t.Fatalf("RunRepoID compare/string failed")
		}
	})
}

func TestIDs_TextAndJSONRoundTrip(t *testing.T) {
	// Use one representative value for each type.
	var (
		tid  TicketID
		rid  RunID
		step StepID
		cid  ClusterID
		rrid RunRepoID
	)

	if err := tid.UnmarshalText([]byte("  T-42  ")); err != nil {
		t.Fatalf("ticket UnmarshalText: %v", err)
	}
	if tid.String() != "T-42" {
		t.Fatalf("ticket normalize: %q", tid.String())
	}
	b, err := json.Marshal(tid)
	if err != nil {
		t.Fatalf("ticket marshal: %v", err)
	}
	if string(b) != "\"T-42\"" {
		t.Fatalf("ticket json string expected, got %s", string(b))
	}
	var tid2 TicketID
	if err := json.Unmarshal(b, &tid2); err != nil {
		t.Fatalf("ticket unmarshal json: %v", err)
	}
	if tid2 != tid {
		t.Fatalf("ticket roundtrip mismatch: %v != %v", tid2, tid)
	}

	if err := rid.UnmarshalText([]byte(" r-1 ")); err != nil {
		t.Fatalf("run UnmarshalText: %v", err)
	}
	if err := step.UnmarshalText([]byte(" step-1 ")); err != nil {
		t.Fatalf("step UnmarshalText: %v", err)
	}
	if err := cid.UnmarshalText([]byte(" c-1 ")); err != nil {
		t.Fatalf("cluster UnmarshalText: %v", err)
	}
	if err := rrid.UnmarshalText([]byte(" repo-1 ")); err != nil {
		t.Fatalf("runrepo UnmarshalText: %v", err)
	}

	for name, v := range map[string]any{"run": rid, "step": step, "cluster": cid, "runrepo": rrid} {
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
	tests := []struct {
		name string
		fn   func([]byte) error
	}{
		{"ticket", func(b []byte) error { var v TicketID; return v.UnmarshalText(b) }},
		{"run", func(b []byte) error { var v RunID; return v.UnmarshalText(b) }},
		{"step", func(b []byte) error { var v StepID; return v.UnmarshalText(b) }},
		{"cluster", func(b []byte) error { var v ClusterID; return v.UnmarshalText(b) }},
		{"runrepo", func(b []byte) error { var v RunRepoID; return v.UnmarshalText(b) }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.fn([]byte("   ")); !errors.Is(err, ErrEmpty) {
				t.Fatalf("expected ErrEmpty, got %v", err)
			}
		})
	}
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

	t.Run("NewBuildID", func(t *testing.T) {
		// Verify non-empty output with expected KSUID length (27 characters).
		id := NewBuildID()
		if id == "" {
			t.Fatal("NewBuildID returned empty string")
		}
		if len(id) != 27 {
			t.Fatalf("NewBuildID length = %d, want 27", len(id))
		}

		// Verify multiple calls produce different values.
		seen := make(map[string]struct{})
		for i := 0; i < 100; i++ {
			newID := NewBuildID()
			if _, exists := seen[newID]; exists {
				t.Fatalf("NewBuildID produced duplicate ID after %d calls", i)
			}
			seen[newID] = struct{}{}
		}
	})

	t.Run("NewRunRepoID", func(t *testing.T) {
		// Verify non-empty output with expected NanoID length (8 characters).
		id := NewRunRepoID()
		if id.IsZero() {
			t.Fatal("NewRunRepoID returned zero value")
		}
		if len(id.String()) != 8 {
			t.Fatalf("NewRunRepoID length = %d, want 8", len(id.String()))
		}

		// Verify characters are from the expected URL-safe alphabet.
		for _, c := range id.String() {
			if !isURLSafeChar(c) {
				t.Fatalf("NewRunRepoID contains invalid character: %c", c)
			}
		}

		// Verify multiple calls produce different values.
		seen := make(map[RunRepoID]struct{})
		for i := 0; i < 100; i++ {
			newID := NewRunRepoID()
			if _, exists := seen[newID]; exists {
				t.Fatalf("NewRunRepoID produced duplicate ID after %d calls", i)
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
}

// isURLSafeChar checks if a character is in the NanoID URL-safe alphabet.
// The alphabet is: 0-9, A-Z, a-z, _, -
func isURLSafeChar(c rune) bool {
	return (c >= '0' && c <= '9') ||
		(c >= 'A' && c <= 'Z') ||
		(c >= 'a' && c <= 'z') ||
		c == '_' || c == '-'
}
