package main

import (
	"net/http"
)

// cloneForStream returns a shallow copy of the provided HTTP client with
// Timeout disabled (0). Used for SSE calls which should not have a global
// client timeout.
func cloneForStream(c *http.Client) *http.Client {
	if c == nil {
		return &http.Client{Timeout: 0}
	}
	clone := *c
	clone.Timeout = 0
	return &clone
}
