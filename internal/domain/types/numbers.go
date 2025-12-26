package types

import "math"

// IntFromAny coerces a value from a map-backed JSON payload to an int.
//
// JSON decoding produces float64 for numbers; this helper accepts integer-typed
// values (int, int8, int16, int32, int64) and float32/float64 if and only if
// the float represents a whole number (fractional part == 0).
//
// Returns (value, true) on success; (0, false) if v is nil, wrong type, or a
// non-integer float (e.g., 1.5).
func IntFromAny(v any) (int, bool) {
	if v == nil {
		return 0, false
	}
	switch n := v.(type) {
	case int:
		return n, true
	case int8:
		return int(n), true
	case int16:
		return int(n), true
	case int32:
		return int(n), true
	case int64:
		return int(n), true
	case float32:
		// Reject non-integer floats (e.g., 1.5).
		f := float64(n)
		if f != math.Trunc(f) {
			return 0, false
		}
		return int(n), true
	case float64:
		// Reject non-integer floats (e.g., 1.5).
		if n != math.Trunc(n) {
			return 0, false
		}
		return int(n), true
	default:
		return 0, false
	}
}

// Int64FromAny coerces a value from a map-backed JSON payload to an int64.
//
// JSON decoding produces float64 for numbers; this helper accepts integer-typed
// values (int, int8, int16, int32, int64) and float32/float64 if and only if
// the float represents a whole number (fractional part == 0).
//
// Returns (value, true) on success; (0, false) if v is nil, wrong type, or a
// non-integer float (e.g., 1.5).
func Int64FromAny(v any) (int64, bool) {
	if v == nil {
		return 0, false
	}
	switch n := v.(type) {
	case int:
		return int64(n), true
	case int8:
		return int64(n), true
	case int16:
		return int64(n), true
	case int32:
		return int64(n), true
	case int64:
		return n, true
	case float32:
		// Reject non-integer floats (e.g., 1.5).
		f := float64(n)
		if f != math.Trunc(f) {
			return 0, false
		}
		return int64(n), true
	case float64:
		// Reject non-integer floats (e.g., 1.5).
		if n != math.Trunc(n) {
			return 0, false
		}
		return int64(n), true
	default:
		return 0, false
	}
}
