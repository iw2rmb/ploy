package contracts

import (
	"math"
	"testing"
)

func TestIntFromAny(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		input     any
		wantValue int
		wantOK    bool
	}{
		{name: "int", input: 42, wantValue: 42, wantOK: true},
		{name: "int zero", input: 0, wantValue: 0, wantOK: true},
		{name: "int negative", input: -10, wantValue: -10, wantOK: true},
		{name: "int8", input: int8(8), wantValue: 8, wantOK: true},
		{name: "int16", input: int16(16), wantValue: 16, wantOK: true},
		{name: "int32", input: int32(32), wantValue: 32, wantOK: true},
		{name: "int64", input: int64(64), wantValue: 64, wantOK: true},
		{name: "float32 whole", input: float32(100.0), wantValue: 100, wantOK: true},
		{name: "float64 whole", input: float64(200.0), wantValue: 200, wantOK: true},
		{name: "float64 zero", input: float64(0.0), wantValue: 0, wantOK: true},
		{name: "float64 negative whole", input: float64(-5.0), wantValue: -5, wantOK: true},
		{name: "float32 fractional", input: float32(1.5), wantValue: 0, wantOK: false},
		{name: "float64 fractional", input: float64(2.7), wantValue: 0, wantOK: false},
		{name: "float64 out of int range", input: math.Pow(2, 63), wantValue: 0, wantOK: false},
		{name: "nil", input: nil, wantValue: 0, wantOK: false},
		{name: "string", input: "42", wantValue: 0, wantOK: false},
		{name: "bool", input: true, wantValue: 0, wantOK: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			gotValue, gotOK := intFromAny(tt.input)
			if gotValue != tt.wantValue || gotOK != tt.wantOK {
				t.Errorf("intFromAny(%v) = (%d, %v), want (%d, %v)",
					tt.input, gotValue, gotOK, tt.wantValue, tt.wantOK)
			}
		})
	}
}

func TestInt64FromAny(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		input     any
		wantValue int64
		wantOK    bool
	}{
		{name: "int", input: 42, wantValue: 42, wantOK: true},
		{name: "int64 large", input: int64(1 << 40), wantValue: 1 << 40, wantOK: true},
		{name: "float64 whole", input: float64(200.0), wantValue: 200, wantOK: true},
		{name: "float64 min int64", input: float64(math.MinInt64), wantValue: math.MinInt64, wantOK: true},
		{name: "float64 out of int64 range", input: math.Pow(2, 63), wantValue: 0, wantOK: false},
		{name: "nil", input: nil, wantValue: 0, wantOK: false},
		{name: "string", input: "42", wantValue: 0, wantOK: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			gotValue, gotOK := int64FromAny(tt.input)
			if gotValue != tt.wantValue || gotOK != tt.wantOK {
				t.Errorf("int64FromAny(%v) = (%d, %v), want (%d, %v)",
					tt.input, gotValue, gotOK, tt.wantValue, tt.wantOK)
			}
		})
	}
}

func TestSpecialFloatsRejected(t *testing.T) {
	t.Parallel()
	specials := []float64{math.NaN(), math.Inf(1), math.Inf(-1)}
	for _, f := range specials {
		if _, ok := intFromAny(f); ok {
			t.Errorf("intFromAny(%v) should be rejected", f)
		}
		if _, ok := int64FromAny(f); ok {
			t.Errorf("int64FromAny(%v) should be rejected", f)
		}
	}
}
