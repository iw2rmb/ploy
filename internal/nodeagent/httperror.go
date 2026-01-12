package nodeagent

import (
	"fmt"
	"io"
	"net/http"
)

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
