package handlers

import "github.com/jackc/pgx/v5"

// mockResult holds a canned return value and error for simple return-only mock methods.
type mockResult[R any] struct {
	val R
	err error
}

func (r *mockResult[R]) ret() (R, error) { return r.val, r.err }

// resolveOrNoRows returns the mockResult's value, or pgx.ErrNoRows when
// the id extractor returns 0 (signaling "not configured").
func resolveOrNoRows[R any](mr *mockResult[R], getID func(R) int64) (R, error) {
	if mr.err != nil {
		var zero R
		return zero, mr.err
	}
	if getID(mr.val) == 0 {
		var zero R
		return zero, pgx.ErrNoRows
	}
	return mr.val, nil
}

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
	calls  []P
	val    R
	err    error
}

func (c *mockCallSlice[P, R]) record(p P) (R, error) {
	c.called = true
	c.calls = append(c.calls, p)
	return c.val, c.err
}

func (c *mockCallSlice[P, R]) last() P {
	if len(c.calls) == 0 {
		var zero P
		return zero
	}
	return c.calls[len(c.calls)-1]
}
