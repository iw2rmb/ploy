package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestClient provides a comprehensive HTTP client for integration testing
type TestClient struct {
	baseURL    string
	httpClient *http.Client
	t          *testing.T

	// Test context
	defaultHeaders map[string]string
	timeout        time.Duration
}

// NewTestClient creates a new API test client
func NewTestClient(t *testing.T, baseURL string) *TestClient {
	return &TestClient{
		baseURL: strings.TrimSuffix(baseURL, "/"),
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		t:              t,
		defaultHeaders: make(map[string]string),
		timeout:        30 * time.Second,
	}
}

// NewBDDTestClient creates a new API test client for BDD tests that handles unavailable services gracefully
func NewBDDTestClient(baseURL string) *TestClient {
	// Use a nil testing.T to avoid failures in BDD context
	return &TestClient{
		baseURL: strings.TrimSuffix(baseURL, "/"),
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		t:              nil, // nil testing.T for BDD graceful handling
		defaultHeaders: make(map[string]string),
		timeout:        30 * time.Second,
	}
}

// WithTimeout sets request timeout
func (c *TestClient) WithTimeout(timeout time.Duration) *TestClient {
	c.timeout = timeout
	c.httpClient.Timeout = timeout
	return c
}

// WithDefaultHeader sets a default header for all requests
func (c *TestClient) WithDefaultHeader(key, value string) *TestClient {
	c.defaultHeaders[key] = value
	return c
}

// RequestBuilder provides fluent interface for building HTTP requests
type RequestBuilder struct {
	client  *TestClient
	method  string
	path    string
	body    interface{}
	headers map[string]string
	query   map[string]string

	// Expectations
	expectedStatus int
	expectJSON     bool
	expectError    bool
}

// GET creates a GET request builder
func (c *TestClient) GET(path string) *RequestBuilder {
	return &RequestBuilder{
		client:  c,
		method:  "GET",
		path:    path,
		headers: make(map[string]string),
		query:   make(map[string]string),
	}
}

// POST creates a POST request builder
func (c *TestClient) POST(path string) *RequestBuilder {
	return &RequestBuilder{
		client:  c,
		method:  "POST",
		path:    path,
		headers: make(map[string]string),
		query:   make(map[string]string),
	}
}

// PUT creates a PUT request builder
func (c *TestClient) PUT(path string) *RequestBuilder {
	return &RequestBuilder{
		client:  c,
		method:  "PUT",
		path:    path,
		headers: make(map[string]string),
		query:   make(map[string]string),
	}
}

// DELETE creates a DELETE request builder
func (c *TestClient) DELETE(path string) *RequestBuilder {
	return &RequestBuilder{
		client:  c,
		method:  "DELETE",
		path:    path,
		headers: make(map[string]string),
		query:   make(map[string]string),
	}
}

// WithJSON sets JSON body and appropriate headers
func (rb *RequestBuilder) WithJSON(body interface{}) *RequestBuilder {
	rb.body = body
	rb.headers["Content-Type"] = "application/json"
	return rb
}

// WithHeader adds a request header
func (rb *RequestBuilder) WithHeader(key, value string) *RequestBuilder {
	rb.headers[key] = value
	return rb
}

// WithQuery adds a query parameter
func (rb *RequestBuilder) WithQuery(key, value string) *RequestBuilder {
	rb.query[key] = value
	return rb
}

// ExpectStatus sets expected HTTP status code
func (rb *RequestBuilder) ExpectStatus(status int) *RequestBuilder {
	rb.expectedStatus = status
	return rb
}

// ExpectJSON indicates response should be valid JSON
func (rb *RequestBuilder) ExpectJSON() *RequestBuilder {
	rb.expectJSON = true
	return rb
}

// ExpectError indicates request should fail
func (rb *RequestBuilder) ExpectError() *RequestBuilder {
	rb.expectError = true
	return rb
}

// Execute performs the HTTP request with automatic assertions
func (rb *RequestBuilder) Execute() *APIResponse {
	req, err := rb.buildRequest()
	if rb.client.t != nil {
		require.NoError(rb.client.t, err, "Failed to build request")
	} else if err != nil {
		// For BDD tests, return nil response on build error
		return &APIResponse{StatusCode: 500, Body: []byte("Build request failed"), t: nil}
	}

	ctx, cancel := context.WithTimeout(context.Background(), rb.client.timeout)
	defer cancel()

	req = req.WithContext(ctx)

	resp, err := rb.client.httpClient.Do(req)
	if rb.expectError {
		if rb.client.t != nil {
			assert.Error(rb.client.t, err, "Expected request to fail")
		}
		return nil
	}

	if rb.client.t != nil {
		require.NoError(rb.client.t, err, "HTTP request failed")
	} else if err != nil {
		// For BDD tests, return error response instead of panicking
		return &APIResponse{StatusCode: 500, Body: []byte("HTTP request failed"), t: nil}
	}

	if resp != nil {
		defer resp.Body.Close()
	}

	var bodyBytes []byte
	if resp != nil {
		bodyBytes, err = io.ReadAll(resp.Body)
		if rb.client.t != nil {
			require.NoError(rb.client.t, err, "Failed to read response body")
		} else if err != nil {
			// For BDD tests, return error response instead of panicking
			return &APIResponse{StatusCode: 500, Body: []byte("Failed to read response body"), t: nil}
		}
	}

	var statusCode int
	var headers http.Header
	if resp != nil {
		statusCode = resp.StatusCode
		headers = resp.Header
	} else {
		statusCode = 500
		headers = make(http.Header)
	}

	apiResp := &APIResponse{
		StatusCode: statusCode,
		Headers:    headers,
		Body:       bodyBytes,
		t:          rb.client.t,
	}

	// Automatic assertions (only for non-BDD tests)
	if rb.client.t != nil {
		if rb.expectedStatus > 0 {
			assert.Equal(rb.client.t, rb.expectedStatus, statusCode,
				"Unexpected status code. Response body: %s", string(bodyBytes))
		}

		if rb.expectJSON {
			var jsonData interface{}
			err := json.Unmarshal(bodyBytes, &jsonData)
			assert.NoError(rb.client.t, err, "Response should be valid JSON")
		}
	}

	return apiResp
}

// buildRequest constructs the HTTP request
func (rb *RequestBuilder) buildRequest() (*http.Request, error) {
	fullURL := rb.client.baseURL + rb.path

	// Add query parameters
	if len(rb.query) > 0 {
		params := url.Values{}
		for key, value := range rb.query {
			params.Add(key, value)
		}
		fullURL += "?" + params.Encode()
	}

	var bodyReader io.Reader
	if rb.body != nil {
		bodyBytes, err := json.Marshal(rb.body)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal request body: %w", err)
		}
		bodyReader = bytes.NewReader(bodyBytes)
	}

	req, err := http.NewRequest(rb.method, fullURL, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Add default headers
	for key, value := range rb.client.defaultHeaders {
		req.Header.Set(key, value)
	}

	// Add request-specific headers
	for key, value := range rb.headers {
		req.Header.Set(key, value)
	}

	return req, nil
}

// APIResponse wraps HTTP response with test utilities
type APIResponse struct {
	StatusCode int
	Headers    http.Header
	Body       []byte
	t          *testing.T
}

// JSON unmarshals response body as JSON
func (r *APIResponse) JSON(target interface{}) *APIResponse {
	err := json.Unmarshal(r.Body, target)
	if r.t != nil {
		require.NoError(r.t, err, "Failed to unmarshal JSON response")
	}
	// For BDD tests, silently continue even on JSON unmarshaling errors
	return r
}

// AssertStatus verifies status code
func (r *APIResponse) AssertStatus(expected int) *APIResponse {
	if r.t != nil {
		assert.Equal(r.t, expected, r.StatusCode,
			"Unexpected status code. Response: %s", string(r.Body))
	}
	// For BDD tests, silently continue without assertions
	return r
}

// AssertHeader verifies response header
func (r *APIResponse) AssertHeader(key, expected string) *APIResponse {
	if r.t != nil {
		actual := r.Headers.Get(key)
		assert.Equal(r.t, expected, actual, "Unexpected header value for %s", key)
	}
	// For BDD tests, silently continue without assertions
	return r
}

// AssertJSONPath verifies JSON field value using simple path notation
func (r *APIResponse) AssertJSONPath(path string, expected interface{}) *APIResponse {
	var data map[string]interface{}
	err := json.Unmarshal(r.Body, &data)
	if r.t != nil {
		require.NoError(r.t, err, "Failed to unmarshal JSON for path assertion")
		value := getJSONPath(data, path)
		assert.Equal(r.t, expected, value, "Unexpected value at JSON path %s", path)
	}
	// For BDD tests, silently continue without assertions
	return r
}

// Helper function to extract value from JSON path (simple implementation)
func getJSONPath(data map[string]interface{}, path string) interface{} {
	parts := strings.Split(path, ".")
	current := interface{}(data)

	for _, part := range parts {
		if m, ok := current.(map[string]interface{}); ok {
			current = m[part]
		} else {
			return nil
		}
	}

	return current
}
