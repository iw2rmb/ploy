package types

import (
	"encoding/json"
	"errors"
	"math"
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

// TestStepIndex_Valid verifies the StepIndex.Valid() method correctly
// identifies valid and invalid step index values.
func TestStepIndex_Valid(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		value float64
		want  bool
	}{
		// Valid finite values (common step indices).
		{"zero", 0, true},
		{"positive_integer", 1000, true},
		{"midpoint_healing", 1500, true},
		{"large_integer", 9999999, true},
		{"negative_integer", -1000, true},

		// Valid: fractional values (used for healing/re-gate insertion).
		{"fractional_half", 1000.5, true},
		{"fractional_small", 1000.1, true},
		{"fractional_large", 1000.999, true},
		{"negative_fractional", -500.25, true},

		// Invalid: NaN.
		{"nan", math.NaN(), false},

		// Invalid: positive/negative infinity.
		{"positive_inf", math.Inf(1), false},
		{"negative_inf", math.Inf(-1), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			idx := StepIndex(tt.value)
			got := idx.Valid()
			if got != tt.want {
				t.Errorf("StepIndex(%v).Valid() = %v, want %v", tt.value, got, tt.want)
			}
		})
	}
}

// TestStepIndex_Float64 verifies the Float64 accessor returns the correct value.
func TestStepIndex_Float64(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		value float64
	}{
		{"zero", 0},
		{"positive", 1000},
		{"negative", -500},
		{"large", 999999999},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			idx := StepIndex(tt.value)
			if got := idx.Float64(); got != tt.value {
				t.Errorf("StepIndex(%v).Float64() = %v, want %v", tt.value, got, tt.value)
			}
		})
	}
}

// TestStepIndex_IsZero verifies the IsZero method.
func TestStepIndex_IsZero(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		value float64
		want  bool
	}{
		{"zero", 0, true},
		{"positive", 1000, false},
		{"negative", -1, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			idx := StepIndex(tt.value)
			if got := idx.IsZero(); got != tt.want {
				t.Errorf("StepIndex(%v).IsZero() = %v, want %v", tt.value, got, tt.want)
			}
		})
	}
}

// TestStepIndexNoTruncation verifies that fractional step indices are preserved
// (not silently truncated) at boundaries where StepIndex is decoded.
func TestStepIndexNoTruncation(t *testing.T) {
	t.Parallel()

	fractionalJSON := []string{
		"1000.5",
		"2500.25",
		"1999.9999",
		"3000.1",
		"-500.5",
		"1750.75",
	}

	for _, lit := range fractionalJSON {
		lit := lit
		t.Run("accept_fractional_json_"+lit, func(t *testing.T) {
			t.Parallel()

			var idx StepIndex
			if err := json.Unmarshal([]byte(lit), &idx); err != nil {
				t.Fatalf("json.Unmarshal(%q, *StepIndex) error: %v", lit, err)
			}
			if !idx.Valid() {
				t.Fatalf("StepIndex(%v).Valid() = false; want true", idx)
			}

			// DiffSummary boundary decode must preserve fractional step_index values.
			summary := DiffSummary([]byte(`{"step_index":` + lit + `}`))
			got, ok := summary.StepIndex()
			if !ok {
				t.Fatalf("DiffSummary.StepIndex() ok=false for %q; want ok=true", lit)
			}
			if got != idx {
				t.Fatalf("DiffSummary.StepIndex()=%v for %q; want %v", got, lit, idx)
			}
		})
	}

	integerJSON := []string{"0", "1000", "1500", "2000", "3000", "-1000"}
	for _, lit := range integerJSON {
		lit := lit
		t.Run("accept_integer_json_"+lit, func(t *testing.T) {
			t.Parallel()

			var idx StepIndex
			if err := json.Unmarshal([]byte(lit), &idx); err != nil {
				t.Fatalf("json.Unmarshal(%q, *StepIndex) error: %v", lit, err)
			}
			if !idx.Valid() {
				t.Fatalf("StepIndex(%v).Valid() = false; want true", idx)
			}

			var payload struct {
				StepIndex StepIndex `json:"step_index"`
			}
			if err := json.Unmarshal([]byte(`{"step_index":`+lit+`}`), &payload); err != nil {
				t.Fatalf("json.Unmarshal(payload with step_index=%q) error: %v", lit, err)
			}
			if payload.StepIndex != idx {
				t.Fatalf("payload.StepIndex=%v; want %v", payload.StepIndex, idx)
			}
		})
	}
}

// TestEventID tests the EventID type for SSE cursor validation and serialization.
func TestEventID(t *testing.T) {
	t.Parallel()

	t.Run("Valid", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			name  string
			value int64
			want  bool
		}{
			// Valid: non-negative values.
			{"zero", 0, true},
			{"positive_small", 1, true},
			{"positive_large", 999999999, true},
			{"max_int64", 9223372036854775807, true},

			// Invalid: negative values.
			{"negative_small", -1, false},
			{"negative_large", -999999999, false},
			{"min_int64", -9223372036854775808, false},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				eid := EventID(tt.value)
				if got := eid.Valid(); got != tt.want {
					t.Errorf("EventID(%d).Valid() = %v, want %v", tt.value, got, tt.want)
				}
			})
		}
	})

	t.Run("Int64", func(t *testing.T) {
		t.Parallel()
		tests := []int64{0, 1, 100, 999999999}
		for _, v := range tests {
			eid := EventID(v)
			if got := eid.Int64(); got != v {
				t.Errorf("EventID(%d).Int64() = %d, want %d", v, got, v)
			}
		}
	})

	t.Run("String", func(t *testing.T) {
		t.Parallel()
		tests := []struct {
			value int64
			want  string
		}{
			{0, "0"},
			{1, "1"},
			{42, "42"},
			{999999999, "999999999"},
		}
		for _, tt := range tests {
			eid := EventID(tt.value)
			if got := eid.String(); got != tt.want {
				t.Errorf("EventID(%d).String() = %q, want %q", tt.value, got, tt.want)
			}
		}
	})

	t.Run("IsZero", func(t *testing.T) {
		t.Parallel()
		if !EventID(0).IsZero() {
			t.Error("EventID(0).IsZero() = false, want true")
		}
		if EventID(1).IsZero() {
			t.Error("EventID(1).IsZero() = true, want false")
		}
	})

	t.Run("TextRoundTrip", func(t *testing.T) {
		t.Parallel()

		tests := []int64{0, 1, 42, 999999999}
		for _, v := range tests {
			eid := EventID(v)
			b, err := eid.MarshalText()
			if err != nil {
				t.Fatalf("MarshalText(%d): %v", v, err)
			}
			var eid2 EventID
			if err := eid2.UnmarshalText(b); err != nil {
				t.Fatalf("UnmarshalText(%q): %v", string(b), err)
			}
			if eid2 != eid {
				t.Errorf("text roundtrip: got %d, want %d", eid2, eid)
			}
		}
	})

	t.Run("TextUnmarshalRejectsNegative", func(t *testing.T) {
		t.Parallel()

		var eid EventID
		err := eid.UnmarshalText([]byte("-1"))
		if err == nil {
			t.Error("UnmarshalText(-1) should fail, got nil")
		}
		err = eid.UnmarshalText([]byte("-999"))
		if err == nil {
			t.Error("UnmarshalText(-999) should fail, got nil")
		}
	})

	t.Run("TextUnmarshalRejectsEmpty", func(t *testing.T) {
		t.Parallel()

		var eid EventID
		err := eid.UnmarshalText([]byte(""))
		if err == nil {
			t.Error("UnmarshalText(\"\") should fail, got nil")
		}
		err = eid.UnmarshalText([]byte("   "))
		if err == nil {
			t.Error("UnmarshalText(\"   \") should fail, got nil")
		}
	})

	t.Run("TextUnmarshalRejectsInvalid", func(t *testing.T) {
		t.Parallel()

		var eid EventID
		err := eid.UnmarshalText([]byte("abc"))
		if err == nil {
			t.Error("UnmarshalText(\"abc\") should fail, got nil")
		}
		err = eid.UnmarshalText([]byte("12.5"))
		if err == nil {
			t.Error("UnmarshalText(\"12.5\") should fail, got nil")
		}
	})

	t.Run("TextMarshalRejectsNegative", func(t *testing.T) {
		t.Parallel()

		eid := EventID(-1)
		_, err := eid.MarshalText()
		if err == nil {
			t.Error("MarshalText(-1) should fail, got nil")
		}
	})

	t.Run("JSONRoundTrip", func(t *testing.T) {
		t.Parallel()

		tests := []int64{0, 1, 42, 999999999}
		for _, v := range tests {
			eid := EventID(v)
			b, err := json.Marshal(eid)
			if err != nil {
				t.Fatalf("json.Marshal(%d): %v", v, err)
			}
			var eid2 EventID
			if err := json.Unmarshal(b, &eid2); err != nil {
				t.Fatalf("json.Unmarshal(%q): %v", string(b), err)
			}
			if eid2 != eid {
				t.Errorf("json roundtrip: got %d, want %d", eid2, eid)
			}
		}
	})

	t.Run("JSONUnmarshalRejectsNegative", func(t *testing.T) {
		t.Parallel()

		var eid EventID
		err := json.Unmarshal([]byte("-1"), &eid)
		if err == nil {
			t.Error("json.Unmarshal(-1) should fail, got nil")
		}
	})

	t.Run("JSONUnmarshalRejectsNull", func(t *testing.T) {
		t.Parallel()

		var eid EventID
		err := json.Unmarshal([]byte("null"), &eid)
		if err == nil {
			t.Fatalf("json.Unmarshal(null) should fail, got nil (eid=%d)", eid)
		}
	})

	t.Run("JSONUnmarshalRejectsString", func(t *testing.T) {
		t.Parallel()

		var eid EventID
		err := json.Unmarshal([]byte(`"42"`), &eid)
		if err == nil {
			t.Fatalf("json.Unmarshal(\"42\") should fail, got nil (eid=%d)", eid)
		}
	})
}

// TestModRef tests the ModRef type for mod reference validation and serialization.
func TestModRef(t *testing.T) {
	t.Parallel()

	t.Run("String", func(t *testing.T) {
		t.Parallel()
		ref := ModRef("my-mod")
		if ref.String() != "my-mod" {
			t.Errorf("ModRef.String() = %q, want %q", ref.String(), "my-mod")
		}
	})

	t.Run("IsZero", func(t *testing.T) {
		t.Parallel()
		var empty ModRef
		if !empty.IsZero() {
			t.Error("zero ModRef.IsZero() = false, want true")
		}
		nonEmpty := ModRef("mod123")
		if nonEmpty.IsZero() {
			t.Error("non-zero ModRef.IsZero() = true, want false")
		}
	})

	t.Run("Validate", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			name    string
			value   string
			wantErr error
		}{
			// Valid cases: mod IDs and mod names.
			{"valid_nanoid", "abc123", nil},
			{"valid_name", "my-mod", nil},
			{"valid_name_underscore", "my_mod_name", nil},
			{"valid_alphanumeric", "ModName123", nil},
			{"valid_uuid_like", "12345678-1234-1234-1234-123456789012", nil}, // No special treatment

			// Invalid: empty.
			{"empty", "", ErrEmpty},
			{"whitespace_only", "   ", ErrEmpty},

			// Invalid: contains URL-unsafe characters.
			{"contains_slash", "my/mod", ErrInvalidModRef},
			{"contains_question", "mod?name", ErrInvalidModRef},
			{"contains_space", "my mod", ErrInvalidModRef},
			{"contains_tab", "my\tmod", ErrInvalidModRef},
			{"contains_newline", "my\nmod", ErrInvalidModRef},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				ref := ModRef(tt.value)
				err := ref.Validate()
				if !errors.Is(err, tt.wantErr) {
					t.Errorf("ModRef(%q).Validate() = %v, want %v", tt.value, err, tt.wantErr)
				}
			})
		}
	})

	t.Run("TextRoundTrip", func(t *testing.T) {
		t.Parallel()

		tests := []string{"mod123", "my-mod", "ModName_v2"}
		for _, v := range tests {
			ref := ModRef(v)
			b, err := ref.MarshalText()
			if err != nil {
				t.Fatalf("MarshalText(%q): %v", v, err)
			}
			var ref2 ModRef
			if err := ref2.UnmarshalText(b); err != nil {
				t.Fatalf("UnmarshalText(%q): %v", string(b), err)
			}
			if ref2 != ref {
				t.Errorf("text roundtrip: got %q, want %q", ref2, ref)
			}
		}
	})

	t.Run("TextUnmarshalTrimsWhitespace", func(t *testing.T) {
		t.Parallel()

		var ref ModRef
		if err := ref.UnmarshalText([]byte("  my-mod  ")); err != nil {
			t.Fatalf("UnmarshalText: %v", err)
		}
		if ref != "my-mod" {
			t.Errorf("got %q, want %q", ref, "my-mod")
		}
	})

	t.Run("TextUnmarshalRejectsEmpty", func(t *testing.T) {
		t.Parallel()

		var ref ModRef
		err := ref.UnmarshalText([]byte(""))
		if !errors.Is(err, ErrEmpty) {
			t.Errorf("UnmarshalText(\"\") = %v, want ErrEmpty", err)
		}
		err = ref.UnmarshalText([]byte("   "))
		if !errors.Is(err, ErrEmpty) {
			t.Errorf("UnmarshalText(\"   \") = %v, want ErrEmpty", err)
		}
	})

	t.Run("TextUnmarshalRejectsInvalidChars", func(t *testing.T) {
		t.Parallel()

		var ref ModRef
		err := ref.UnmarshalText([]byte("my/mod"))
		if !errors.Is(err, ErrInvalidModRef) {
			t.Errorf("UnmarshalText(\"my/mod\") = %v, want ErrInvalidModRef", err)
		}
		err = ref.UnmarshalText([]byte("mod?name"))
		if !errors.Is(err, ErrInvalidModRef) {
			t.Errorf("UnmarshalText(\"mod?name\") = %v, want ErrInvalidModRef", err)
		}
	})

	t.Run("TextMarshalRejectsEmpty", func(t *testing.T) {
		t.Parallel()

		ref := ModRef("")
		_, err := ref.MarshalText()
		if !errors.Is(err, ErrEmpty) {
			t.Errorf("MarshalText(\"\") = %v, want ErrEmpty", err)
		}
	})

	t.Run("TextMarshalRejectsInvalidChars", func(t *testing.T) {
		t.Parallel()

		ref := ModRef("my/mod")
		_, err := ref.MarshalText()
		if !errors.Is(err, ErrInvalidModRef) {
			t.Errorf("MarshalText(\"my/mod\") = %v, want ErrInvalidModRef", err)
		}
	})

	t.Run("JSONRoundTrip", func(t *testing.T) {
		t.Parallel()

		tests := []string{"mod123", "my-mod", "ModName_v2"}
		for _, v := range tests {
			ref := ModRef(v)
			b, err := json.Marshal(ref)
			if err != nil {
				t.Fatalf("json.Marshal(%q): %v", v, err)
			}
			var ref2 ModRef
			if err := json.Unmarshal(b, &ref2); err != nil {
				t.Fatalf("json.Unmarshal(%q): %v", string(b), err)
			}
			if ref2 != ref {
				t.Errorf("json roundtrip: got %q, want %q", ref2, ref)
			}
		}
	})

	t.Run("JSONUnmarshalRejectsInvalid", func(t *testing.T) {
		t.Parallel()

		var ref ModRef
		err := json.Unmarshal([]byte(`"my/mod"`), &ref)
		if !errors.Is(err, ErrInvalidModRef) {
			t.Errorf("json.Unmarshal(\"my/mod\") = %v, want ErrInvalidModRef", err)
		}
	})
}
