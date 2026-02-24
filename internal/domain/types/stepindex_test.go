package types

import (
	"encoding/json"
	"math"
	"testing"
)

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

			// DiffSummary boundary decode must preserve fractional next_id values.
			summary := DiffSummary([]byte(`{"next_id":` + lit + `}`))
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
				StepIndex StepIndex `json:"next_id"`
			}
			if err := json.Unmarshal([]byte(`{"next_id":`+lit+`}`), &payload); err != nil {
				t.Fatalf("json.Unmarshal(payload with next_id=%q) error: %v", lit, err)
			}
			if payload.StepIndex != idx {
				t.Fatalf("payload.StepIndex=%v; want %v", payload.StepIndex, idx)
			}
		})
	}
}
