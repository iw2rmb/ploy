package step

import "errors"

// ErrRepoCancelled signals that execution for the current repo should be marked
// as Cancelled (not Fail). This is used for policy-driven early exits where
// further jobs must be cancelled (e.g., stack detection is required and failed).
var ErrRepoCancelled = errors.New("repo cancelled")
