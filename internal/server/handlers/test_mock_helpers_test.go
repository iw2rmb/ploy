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
