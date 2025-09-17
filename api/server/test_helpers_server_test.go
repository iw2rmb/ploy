package server

import (
	"net/http"

	"github.com/gofiber/fiber/v2"
)

// createMockServer provides a minimal Server instance suitable for handler tests.
// It initializes a Fiber app and empty dependencies; tests can override fields.
func createMockServer() *Server {
	return &Server{
		app:          fiber.New(),
		config:       &ControllerConfig{},
		dependencies: &ServiceDependencies{},
	}
}

// mustResponse wraps a Fiber test response, asserting errors are nil and
// registering cleanup so response bodies are always closed for lint/vet checks.
func mustResponse(t testingT) func(*http.Response, error) *http.Response {
	t.Helper()
	return func(resp *http.Response, err error) *http.Response {
		t.Helper()
		if err != nil {
			t.Fatalf("app.Test error: %v", err)
		}
		respCopy := resp
		t.Cleanup(func() {
			if respCopy != nil && respCopy.Body != nil {
				if cerr := respCopy.Body.Close(); cerr != nil {
					t.Errorf("close body: %v", cerr)
				}
			}
		})
		return resp
	}
}

// testingT abstracts *testing.T to keep helpers lightweight.
type testingT interface {
	Cleanup(func())
	Errorf(string, ...interface{})
	Fatalf(string, ...interface{})
	Helper()
}
