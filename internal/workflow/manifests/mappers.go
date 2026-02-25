package manifests

// mapSlice applies fn to each element and returns nil for empty input.
func mapSlice[T any, U any](in []T, fn func(T) U) []U {
	if len(in) == 0 {
		return nil
	}
	out := make([]U, len(in))
	for i, v := range in {
		out[i] = fn(v)
	}
	return out
}
