package nodeagent

import (
	"fmt"
	"io"
	"net/http"
)

// drainAndClose drains any remaining data from the response body and closes it.
// Draining ensures the underlying connection can be reused by the connection pool.
// Without draining, the connection may be closed and a new one must be established.
func drainAndClose(resp *http.Response) {
	if resp == nil || resp.Body == nil {
		return
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()
}

// readErrorBody reads and returns the response body as a string.
// If reading fails, returns a fallback message.
func readErrorBody(resp *http.Response) string {
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "(failed to read body)"
	}
	return string(body)
}

// httpError creates a formatted error for unexpected HTTP response status codes.
// Reads the response body to include server error details in the message.
func httpError(resp *http.Response, expected int, action string) error {
	if resp.StatusCode == expected {
		return nil
	}
	return fmt.Errorf("%s failed: status %d: %s", action, resp.StatusCode, readErrorBody(resp))
}
