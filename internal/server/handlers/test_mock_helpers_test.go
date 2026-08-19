package handlers

// mockResult holds a canned return value and error for simple return-only mock methods.
type mockResult[R any] struct {
	val R
	err error
}

func (r *mockResult[R]) ret() (R, error) { return r.val, r.err }

// mockCall tracks whether a method was called, its params, and return values.
type mockCall[P, R any] struct {
	called bool
	params P
	val    R
	err    error
}

func (c *mockCall[P, R]) record(p P) (R, error) {
	c.called = true
	c.params = p
	return c.val, c.err
}

// mockCallSlice tracks every invocation, accumulating all params in a slice.
type mockCallSlice[P, R any] struct {
	called bool
	params P
	calls  []P
	val    R
	err    error
}

func (c *mockCallSlice[P, R]) record(p P) (R, error) {
	c.called = true
	c.params = p
	c.calls = append(c.calls, p)
	return c.val, c.err
}

// mockCallSeq returns a different value/error for each successive call,
// latching on the last entry once exhausted. Mirrors mockCall in API surface
// (called, params, n) so callers can swap one for the other.
type mockCallSeq[P, R any] struct {
	called bool
	params P
	vals   []R
	errs   []error
	n      int
}

func (c *mockCallSeq[P, R]) record(p P) (R, error) {
	c.called = true
	c.params = p
	var v R
	var err error
	if len(c.vals) > 0 {
		idx := c.n
		if idx >= len(c.vals) {
			idx = len(c.vals) - 1
		}
		v = c.vals[idx]
	}
	if len(c.errs) > 0 {
		idx := c.n
		if idx >= len(c.errs) {
			idx = len(c.errs) - 1
		}
		err = c.errs[idx]
	}
	c.n++
	return v, err
}
