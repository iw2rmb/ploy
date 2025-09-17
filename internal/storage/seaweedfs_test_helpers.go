package storage

import (
	"fmt"
	"io"
	"net/http"
	"strings"
)

// mockRoundTripper implements http.RoundTripper for controlled HTTP responses
type mockRoundTripper struct {
	responses []mockResponse
	index     int
	requests  []*http.Request // Track requests for assertions
}

type mockResponse struct {
	statusCode int
	body       string
	headers    map[string]string
	err        error
}

func (m *mockRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	m.requests = append(m.requests, req)
	if m.index >= len(m.responses) {
		return nil, fmt.Errorf("no more mock responses available")
	}
	resp := m.responses[m.index]
	m.index++
	if resp.err != nil {
		return nil, resp.err
	}
	response := &http.Response{
		StatusCode: resp.statusCode,
		Status:     fmt.Sprintf("%d", resp.statusCode),
		Body:       io.NopCloser(strings.NewReader(resp.body)),
		Header:     make(http.Header),
	}
	for k, v := range resp.headers {
		response.Header.Set(k, v)
	}
	return response, nil
}
