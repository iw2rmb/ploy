// Package integration provides utilities for integration testing
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

	"github.com/stretchr/testify/require"
)

// HTTPClient provides utilities for HTTP integration testing
type HTTPClient struct {
	BaseURL    string
	HTTPClient *http.Client
	Headers    map[string]string
}

// NewHTTPClient creates a new HTTP client for testing
func NewHTTPClient(baseURL string) *HTTPClient {
	return &HTTPClient{
		BaseURL: strings.TrimSuffix(baseURL, "/"),
		HTTPClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		Headers: make(map[string]string),
	}
}

// WithTimeout sets the client timeout
func (c *HTTPClient) WithTimeout(timeout time.Duration) *HTTPClient {
	c.HTTPClient.Timeout = timeout
	return c
}

// WithHeader adds a header to all requests
func (c *HTTPClient) WithHeader(key, value string) *HTTPClient {
	c.Headers[key] = value
	return c
}

// WithBearerToken adds a Bearer token to all requests
func (c *HTTPClient) WithBearerToken(token string) *HTTPClient {
	c.Headers["Authorization"] = "Bearer " + token
	return c
}

// WithContentType sets the Content-Type header
func (c *HTTPClient) WithContentType(contentType string) *HTTPClient {
	c.Headers["Content-Type"] = contentType
	return c
}

// GET performs a GET request
func (c *HTTPClient) GET(ctx context.Context, path string) (*http.Response, error) {
	return c.request(ctx, "GET", path, nil)
}

// POST performs a POST request with JSON body
func (c *HTTPClient) POST(ctx context.Context, path string, body interface{}) (*http.Response, error) {
	var bodyReader io.Reader
	
	if body != nil {
		jsonData, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal request body: %w", err)
		}
		bodyReader = bytes.NewReader(jsonData)
	}
	
	return c.request(ctx, "POST", path, bodyReader)
}

// PUT performs a PUT request with JSON body
func (c *HTTPClient) PUT(ctx context.Context, path string, body interface{}) (*http.Response, error) {
	var bodyReader io.Reader
	
	if body != nil {
		jsonData, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal request body: %w", err)
		}
		bodyReader = bytes.NewReader(jsonData)
	}
	
	return c.request(ctx, "PUT", path, bodyReader)
}

// DELETE performs a DELETE request
func (c *HTTPClient) DELETE(ctx context.Context, path string) (*http.Response, error) {
	return c.request(ctx, "DELETE", path, nil)
}

// request performs the actual HTTP request
func (c *HTTPClient) request(ctx context.Context, method, path string, body io.Reader) (*http.Response, error) {
	fullURL := c.BaseURL + "/" + strings.TrimPrefix(path, "/")
	
	req, err := http.NewRequestWithContext(ctx, method, fullURL, body)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	
	// Add headers
	for key, value := range c.Headers {
		req.Header.Set(key, value)
	}
	
	// Set default Content-Type for POST/PUT with body
	if (method == "POST" || method == "PUT") && body != nil {
		if req.Header.Get("Content-Type") == "" {
			req.Header.Set("Content-Type", "application/json")
		}
	}
	
	return c.HTTPClient.Do(req)
}

// APITestHelper provides utilities for API integration testing
type APITestHelper struct {
	t      *testing.T
	client *HTTPClient
}

// NewAPITestHelper creates a new API test helper
func NewAPITestHelper(t *testing.T, baseURL string) *APITestHelper {
	return &APITestHelper{
		t:      t,
		client: NewHTTPClient(baseURL),
	}
}

// WithTimeout sets the client timeout
func (h *APITestHelper) WithTimeout(timeout time.Duration) *APITestHelper {
	h.client.WithTimeout(timeout)
	return h
}

// WithHeader adds a header to all requests
func (h *APITestHelper) WithHeader(key, value string) *APITestHelper {
	h.client.WithHeader(key, value)
	return h
}

// ExpectGET performs a GET request and validates the response
func (h *APITestHelper) ExpectGET(path string, expectedStatus int) *APIResponse {
	h.t.Helper()
	
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	
	resp, err := h.client.GET(ctx, path)
	require.NoError(h.t, err, "GET request failed")
	
	return h.validateResponse(resp, expectedStatus)
}

// ExpectPOST performs a POST request and validates the response
func (h *APITestHelper) ExpectPOST(path string, body interface{}, expectedStatus int) *APIResponse {
	h.t.Helper()
	
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	
	resp, err := h.client.POST(ctx, path, body)
	require.NoError(h.t, err, "POST request failed")
	
	return h.validateResponse(resp, expectedStatus)
}

// ExpectPUT performs a PUT request and validates the response
func (h *APITestHelper) ExpectPUT(path string, body interface{}, expectedStatus int) *APIResponse {
	h.t.Helper()
	
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	
	resp, err := h.client.PUT(ctx, path, body)
	require.NoError(h.t, err, "PUT request failed")
	
	return h.validateResponse(resp, expectedStatus)
}

// ExpectDELETE performs a DELETE request and validates the response
func (h *APITestHelper) ExpectDELETE(path string, expectedStatus int) *APIResponse {
	h.t.Helper()
	
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	
	resp, err := h.client.DELETE(ctx, path)
	require.NoError(h.t, err, "DELETE request failed")
	
	return h.validateResponse(resp, expectedStatus)
}

// validateResponse validates the HTTP response status
func (h *APITestHelper) validateResponse(resp *http.Response, expectedStatus int) *APIResponse {
	h.t.Helper()
	
	require.Equal(h.t, expectedStatus, resp.StatusCode, 
		"Expected status %d, got %d", expectedStatus, resp.StatusCode)
	
	return &APIResponse{
		t:        h.t,
		Response: resp,
	}
}

// APIResponse wraps an HTTP response with testing utilities
type APIResponse struct {
	t *testing.T
	*http.Response
}

// JSONBody parses the response body as JSON
func (r *APIResponse) JSONBody(target interface{}) *APIResponse {
	r.t.Helper()
	
	defer r.Body.Close()
	
	body, err := io.ReadAll(r.Body)
	require.NoError(r.t, err, "Failed to read response body")
	
	if len(body) > 0 {
		err = json.Unmarshal(body, target)
		require.NoError(r.t, err, "Failed to unmarshal JSON response")
	}
	
	return r
}

// TextBody returns the response body as a string
func (r *APIResponse) TextBody() string {
	r.t.Helper()
	
	defer r.Body.Close()
	
	body, err := io.ReadAll(r.Body)
	require.NoError(r.t, err, "Failed to read response body")
	
	return string(body)
}

// HasHeader checks if the response has a specific header
func (r *APIResponse) HasHeader(key, expectedValue string) *APIResponse {
	r.t.Helper()
	
	actualValue := r.Header.Get(key)
	require.Equal(r.t, expectedValue, actualValue, 
		"Expected header %s to be %s, got %s", key, expectedValue, actualValue)
	
	return r
}

// ContainsHeader checks if the response contains a header
func (r *APIResponse) ContainsHeader(key string) *APIResponse {
	r.t.Helper()
	
	_, exists := r.Header[key]
	require.True(r.t, exists, "Expected header %s to exist", key)
	
	return r
}

// ServiceTestHelper provides utilities for service integration testing
type ServiceTestHelper struct {
	t         *testing.T
	endpoints map[string]string
}

// NewServiceTestHelper creates a new service test helper
func NewServiceTestHelper(t *testing.T) *ServiceTestHelper {
	return &ServiceTestHelper{
		t:         t,
		endpoints: make(map[string]string),
	}
}

// WithEndpoint adds a service endpoint
func (h *ServiceTestHelper) WithEndpoint(service, endpoint string) *ServiceTestHelper {
	h.endpoints[service] = endpoint
	return h
}

// WaitForService waits for a service to be healthy
func (h *ServiceTestHelper) WaitForService(service string, timeout time.Duration) {
	h.t.Helper()
	
	endpoint, exists := h.endpoints[service]
	require.True(h.t, exists, "Endpoint not configured for service: %s", service)
	
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	
	client := &http.Client{Timeout: 2 * time.Second}
	healthURL := endpoint
	
	// Try to construct a health check URL
	if u, err := url.Parse(endpoint); err == nil {
		switch service {
		case "consul":
			u.Path = "/v1/status/leader"
		case "nomad":
			u.Path = "/v1/status/leader"
		case "seaweedfs":
			u.Path = "/dir/status"
		default:
			u.Path = "/health"
		}
		healthURL = u.String()
	}
	
	h.t.Logf("Waiting for service %s at %s", service, healthURL)
	
	for {
		select {
		case <-ctx.Done():
			h.t.Fatalf("Service %s did not become healthy within %v", service, timeout)
		default:
			resp, err := client.Get(healthURL)
			if err == nil && resp.StatusCode >= 200 && resp.StatusCode < 300 {
				resp.Body.Close()
				h.t.Logf("Service %s is healthy", service)
				return
			}
			if resp != nil {
				resp.Body.Close()
			}
			time.Sleep(500 * time.Millisecond)
		}
	}
}

// WaitForAllServices waits for all configured services to be healthy
func (h *ServiceTestHelper) WaitForAllServices(timeout time.Duration) {
	h.t.Helper()
	
	for service := range h.endpoints {
		h.WaitForService(service, timeout)
	}
}

// TestHealthEndpoint tests a service health endpoint
func (h *ServiceTestHelper) TestHealthEndpoint(service string) {
	h.t.Helper()
	
	endpoint, exists := h.endpoints[service]
	require.True(h.t, exists, "Endpoint not configured for service: %s", service)
	
	client := NewHTTPClient(endpoint)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	
	resp, err := client.GET(ctx, "/health")
	if err != nil {
		// Try service-specific health endpoints
		switch service {
		case "consul":
			resp, err = client.GET(ctx, "/v1/status/leader")
		case "nomad":
			resp, err = client.GET(ctx, "/v1/status/leader")
		case "seaweedfs":
			resp, err = client.GET(ctx, "/dir/status")
		}
	}
	
	require.NoError(h.t, err, "Health check failed for service: %s", service)
	require.True(h.t, resp.StatusCode >= 200 && resp.StatusCode < 300, 
		"Health check returned status %d for service: %s", resp.StatusCode, service)
	
	resp.Body.Close()
}

// LoadTestHelper provides utilities for load testing
type LoadTestHelper struct {
	t       *testing.T
	client  *HTTPClient
	results []LoadTestResult
}

// LoadTestResult represents the result of a load test request
type LoadTestResult struct {
	Duration   time.Duration
	StatusCode int
	Error      error
}

// NewLoadTestHelper creates a new load test helper
func NewLoadTestHelper(t *testing.T, baseURL string) *LoadTestHelper {
	return &LoadTestHelper{
		t:      t,
		client: NewHTTPClient(baseURL),
	}
}

// RunLoadTest performs a simple load test
func (h *LoadTestHelper) RunLoadTest(path string, concurrent int, duration time.Duration) []LoadTestResult {
	h.t.Helper()
	
	h.t.Logf("Running load test: %d concurrent requests for %v", concurrent, duration)
	
	ctx, cancel := context.WithTimeout(context.Background(), duration)
	defer cancel()
	
	results := make(chan LoadTestResult, concurrent*100) // Buffer for results
	
	// Start concurrent workers
	for i := 0; i < concurrent; i++ {
		go func() {
			for {
				select {
				case <-ctx.Done():
					return
				default:
					start := time.Now()
					resp, err := h.client.GET(context.Background(), path)
					elapsed := time.Since(start)
					
					result := LoadTestResult{
						Duration: elapsed,
						Error:    err,
					}
					
					if resp != nil {
						result.StatusCode = resp.StatusCode
						resp.Body.Close()
					}
					
					results <- result
				}
			}
		}()
	}
	
	// Wait for test duration
	<-ctx.Done()
	
	// Collect results
	close(results)
	var collected []LoadTestResult
	for result := range results {
		collected = append(collected, result)
	}
	
	h.results = collected
	h.t.Logf("Load test completed: %d requests", len(collected))
	
	return collected
}

// AnalyzeResults analyzes load test results
func (h *LoadTestHelper) AnalyzeResults() LoadTestAnalysis {
	h.t.Helper()
	
	analysis := LoadTestAnalysis{
		TotalRequests: len(h.results),
	}
	
	if len(h.results) == 0 {
		return analysis
	}
	
	var totalDuration time.Duration
	statusCounts := make(map[int]int)
	errorCount := 0
	
	for _, result := range h.results {
		totalDuration += result.Duration
		
		if result.Error != nil {
			errorCount++
		} else {
			statusCounts[result.StatusCode]++
		}
	}
	
	analysis.AverageLatency = totalDuration / time.Duration(len(h.results))
	analysis.ErrorRate = float64(errorCount) / float64(len(h.results))
	analysis.StatusCodes = statusCounts
	
	return analysis
}

// LoadTestAnalysis contains the results of load test analysis
type LoadTestAnalysis struct {
	TotalRequests   int
	AverageLatency  time.Duration
	ErrorRate       float64
	StatusCodes     map[int]int
}

// AssertPerformance asserts that performance metrics meet expectations
func (a LoadTestAnalysis) AssertPerformance(t *testing.T, maxLatency time.Duration, maxErrorRate float64) {
	t.Helper()
	
	require.True(t, a.AverageLatency <= maxLatency, 
		"Average latency %v exceeds maximum %v", a.AverageLatency, maxLatency)
	
	require.True(t, a.ErrorRate <= maxErrorRate, 
		"Error rate %.2f%% exceeds maximum %.2f%%", a.ErrorRate*100, maxErrorRate*100)
}

// LogAnalysis logs the analysis results
func (a LoadTestAnalysis) LogAnalysis(t *testing.T) {
	t.Helper()
	
	t.Logf("Load Test Analysis:")
	t.Logf("  Total Requests: %d", a.TotalRequests)
	t.Logf("  Average Latency: %v", a.AverageLatency)
	t.Logf("  Error Rate: %.2f%%", a.ErrorRate*100)
	t.Logf("  Status Codes:")
	for code, count := range a.StatusCodes {
		t.Logf("    %d: %d requests", code, count)
	}
}