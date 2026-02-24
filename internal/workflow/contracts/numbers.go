package contracts

import "math"

func int64FromFloat64(f float64) (int64, bool) {
	if math.IsNaN(f) || math.IsInf(f, 0) {
		return 0, false
	}
	if f != math.Trunc(f) {
		return 0, false
	}
	if f < float64(math.MinInt64) || f >= float64(math.MaxInt64) {
		return 0, false
	}
	return int64(f), true
}

// intFromAny coerces a value from a map-backed JSON payload to an int.
//
// JSON decoding produces float64 for numbers; this helper accepts integer-typed
// values (int, int8, int16, int32, int64) and float32/float64 if and only if
// the float represents a whole number (fractional part == 0).
func intFromAny(v any) (int, bool) {
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
		if n < int64(math.MinInt) || n > int64(math.MaxInt) {
			return 0, false
		}
		return int(n), true
	case float32:
		i64, ok := int64FromFloat64(float64(n))
		if !ok || i64 < int64(math.MinInt) || i64 > int64(math.MaxInt) {
			return 0, false
		}
		return int(i64), true
	case float64:
		i64, ok := int64FromFloat64(n)
		if !ok || i64 < int64(math.MinInt) || i64 > int64(math.MaxInt) {
			return 0, false
		}
		return int(i64), true
	default:
		return 0, false
	}
}

// int64FromAny coerces a value from a map-backed JSON payload to an int64.
//
// JSON decoding produces float64 for numbers; this helper accepts integer-typed
// values (int, int8, int16, int32, int64) and float32/float64 if and only if
// the float represents a whole number (fractional part == 0).
func int64FromAny(v any) (int64, bool) {
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
		return int64FromFloat64(float64(n))
	case float64:
		return int64FromFloat64(n)
	default:
		return 0, false
	}
}
