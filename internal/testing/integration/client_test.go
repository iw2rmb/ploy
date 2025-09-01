package integration

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestAPITestingFramework(t *testing.T) {
	// Create mock server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/health":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"status":"healthy","version":"1.0.0"}`))
		case "/error":
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(`{"error":"internal server error"}`))
		case "/echo":
			if r.Method == "POST" {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusCreated)
				w.Write([]byte(`{"received":true}`))
			}
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	client := NewTestClient(t, server.URL)

	t.Run("GET request with JSON response", func(t *testing.T) {
		resp := client.
			GET("/health").
			ExpectStatus(http.StatusOK).
			ExpectJSON().
			Execute()

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Contains(t, string(resp.Body), "healthy")

		resp.AssertJSONPath("status", "healthy")
		resp.AssertJSONPath("version", "1.0.0")
	})

	t.Run("POST request with JSON body", func(t *testing.T) {
		payload := map[string]interface{}{
			"test": "data",
		}

		resp := client.
			POST("/echo").
			WithJSON(payload).
			ExpectStatus(http.StatusCreated).
			Execute()

		resp.AssertJSONPath("received", true)
	})

	t.Run("Error handling", func(t *testing.T) {
		resp := client.
			GET("/error").
			ExpectStatus(http.StatusInternalServerError).
			Execute()

		assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
	})

	t.Run("Client configuration", func(t *testing.T) {
		configuredClient := client.
			WithTimeout(5*time.Second).
			WithDefaultHeader("User-Agent", "ploy-test-client")

		assert.Equal(t, 5*time.Second, configuredClient.timeout)
		assert.Equal(t, "ploy-test-client", configuredClient.defaultHeaders["User-Agent"])
	})
}

func TestRequestBuilder(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Echo request details
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		// Simple JSON response for testing
		w.Write([]byte(`{"method":"` + r.Method + `","received":true}`))
	}))
	defer server.Close()

	client := NewTestClient(t, server.URL)

	t.Run("HTTP methods", func(t *testing.T) {
		methods := []string{"GET", "POST", "PUT", "DELETE"}

		for _, method := range methods {
			t.Run(method, func(t *testing.T) {
				var resp *APIResponse
				switch method {
				case "GET":
					resp = client.GET("/test").Execute()
				case "POST":
					resp = client.POST("/test").Execute()
				case "PUT":
					resp = client.PUT("/test").Execute()
				case "DELETE":
					resp = client.DELETE("/test").Execute()
				}

				assert.NotNil(t, resp)
				resp.AssertJSONPath("method", method)
			})
		}
	})

	t.Run("Query parameters", func(t *testing.T) {
		// Note: This is a simplified test since the mock server doesn't fully echo query params
		resp := client.
			GET("/test").
			WithQuery("param1", "value1").
			WithQuery("param2", "value2").
			Execute()

		assert.NotNil(t, resp)
		assert.Equal(t, http.StatusOK, resp.StatusCode)
	})

	t.Run("Headers", func(t *testing.T) {
		resp := client.
			GET("/test").
			WithHeader("Custom-Header", "test-value").
			Execute()

		assert.NotNil(t, resp)
		assert.Equal(t, http.StatusOK, resp.StatusCode)
	})
}

func TestJSONPathExtraction(t *testing.T) {
	data := map[string]interface{}{
		"simple": "value",
		"nested": map[string]interface{}{
			"key": "nested-value",
		},
	}

	tests := []struct {
		name     string
		path     string
		expected interface{}
	}{
		{"simple path", "simple", "value"},
		{"nested path", "nested.key", "nested-value"},
		{"non-existent path", "missing", nil},
		{"invalid nested path", "simple.invalid", nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getJSONPath(data, tt.path)
			assert.Equal(t, tt.expected, result)
		})
	}
}
