package builders

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
)

// HTTPRequest represents a test HTTP request
type HTTPRequest struct {
	Method  string
	Path    string
	Headers map[string]string
	Query   map[string]string
	Body    interface{}
}

// HTTPResponse represents a test HTTP response
type HTTPResponse struct {
	StatusCode int
	Headers    map[string]string
	Body       interface{}
}

// RequestBuilder provides a fluent interface for creating test HTTP requests
type RequestBuilder struct {
	req HTTPRequest
}

// NewRequest creates a new request builder with defaults
func NewRequest() *RequestBuilder {
	return &RequestBuilder{
		req: HTTPRequest{
			Method:  "GET",
			Path:    "/",
			Headers: make(map[string]string),
			Query:   make(map[string]string),
		},
	}
}

// WithMethod sets the HTTP method
func (b *RequestBuilder) WithMethod(method string) *RequestBuilder {
	b.req.Method = method
	return b
}

// GET sets the method to GET
func (b *RequestBuilder) GET(path string) *RequestBuilder {
	b.req.Method = "GET"
	b.req.Path = path
	return b
}

// POST sets the method to POST
func (b *RequestBuilder) POST(path string) *RequestBuilder {
	b.req.Method = "POST"
	b.req.Path = path
	return b
}

// PUT sets the method to PUT
func (b *RequestBuilder) PUT(path string) *RequestBuilder {
	b.req.Method = "PUT"
	b.req.Path = path
	return b
}

// DELETE sets the method to DELETE
func (b *RequestBuilder) DELETE(path string) *RequestBuilder {
	b.req.Method = "DELETE"
	b.req.Path = path
	return b
}

// WithPath sets the request path
func (b *RequestBuilder) WithPath(path string) *RequestBuilder {
	b.req.Path = path
	return b
}

// WithHeader adds a header to the request
func (b *RequestBuilder) WithHeader(key, value string) *RequestBuilder {
	if b.req.Headers == nil {
		b.req.Headers = make(map[string]string)
	}
	b.req.Headers[key] = value
	return b
}

// WithAuth adds an Authorization header
func (b *RequestBuilder) WithAuth(token string) *RequestBuilder {
	return b.WithHeader("Authorization", "Bearer "+token)
}

// WithJSON sets Content-Type to application/json
func (b *RequestBuilder) WithJSON() *RequestBuilder {
	return b.WithHeader("Content-Type", "application/json")
}

// WithQuery adds a query parameter
func (b *RequestBuilder) WithQuery(key, value string) *RequestBuilder {
	if b.req.Query == nil {
		b.req.Query = make(map[string]string)
	}
	b.req.Query[key] = value
	return b
}

// WithBody sets the request body
func (b *RequestBuilder) WithBody(body interface{}) *RequestBuilder {
	b.req.Body = body
	return b
}

// Build creates an *http.Request
func (b *RequestBuilder) Build() (*http.Request, error) {
	var bodyReader io.Reader

	if b.req.Body != nil {
		switch v := b.req.Body.(type) {
		case string:
			bodyReader = bytes.NewBufferString(v)
		case []byte:
			bodyReader = bytes.NewBuffer(v)
		case io.Reader:
			bodyReader = v
		default:
			// Assume JSON encoding for other types
			data, err := json.Marshal(v)
			if err != nil {
				return nil, err
			}
			bodyReader = bytes.NewBuffer(data)
		}
	}

	req, err := http.NewRequest(b.req.Method, b.req.Path, bodyReader)
	if err != nil {
		return nil, err
	}

	// Add headers
	for k, v := range b.req.Headers {
		req.Header.Set(k, v)
	}

	// Add query parameters
	if len(b.req.Query) > 0 {
		q := req.URL.Query()
		for k, v := range b.req.Query {
			q.Add(k, v)
		}
		req.URL.RawQuery = q.Encode()
	}

	return req, nil
}

// BuildTest creates an *http.Request for testing
func (b *RequestBuilder) BuildTest() *http.Request {
	req, _ := b.Build()
	return httptest.NewRequest(req.Method, req.URL.String(), req.Body)
}

// ResponseBuilder provides a fluent interface for creating test HTTP responses
type ResponseBuilder struct {
	resp HTTPResponse
}

// NewResponse creates a new response builder with defaults
func NewResponse() *ResponseBuilder {
	return &ResponseBuilder{
		resp: HTTPResponse{
			StatusCode: http.StatusOK,
			Headers:    make(map[string]string),
		},
	}
}

// WithStatus sets the status code
func (b *ResponseBuilder) WithStatus(code int) *ResponseBuilder {
	b.resp.StatusCode = code
	return b
}

// OK sets status to 200
func (b *ResponseBuilder) OK() *ResponseBuilder {
	return b.WithStatus(http.StatusOK)
}

// Created sets status to 201
func (b *ResponseBuilder) Created() *ResponseBuilder {
	return b.WithStatus(http.StatusCreated)
}

// BadRequest sets status to 400
func (b *ResponseBuilder) BadRequest() *ResponseBuilder {
	return b.WithStatus(http.StatusBadRequest)
}

// NotFound sets status to 404
func (b *ResponseBuilder) NotFound() *ResponseBuilder {
	return b.WithStatus(http.StatusNotFound)
}

// InternalServerError sets status to 500
func (b *ResponseBuilder) InternalServerError() *ResponseBuilder {
	return b.WithStatus(http.StatusInternalServerError)
}

// WithHeader adds a response header
func (b *ResponseBuilder) WithHeader(key, value string) *ResponseBuilder {
	if b.resp.Headers == nil {
		b.resp.Headers = make(map[string]string)
	}
	b.resp.Headers[key] = value
	return b
}

// WithJSONHeader sets Content-Type to application/json
func (b *ResponseBuilder) WithJSONHeader() *ResponseBuilder {
	return b.WithHeader("Content-Type", "application/json")
}

// WithBody sets the response body
func (b *ResponseBuilder) WithBody(body interface{}) *ResponseBuilder {
	b.resp.Body = body
	return b
}

// Build creates an *httptest.ResponseRecorder with the configured response
func (b *ResponseBuilder) Build() *httptest.ResponseRecorder {
	w := httptest.NewRecorder()

	// Set headers
	for k, v := range b.resp.Headers {
		w.Header().Set(k, v)
	}

	// Write status code
	w.WriteHeader(b.resp.StatusCode)

	// Write body
	if b.resp.Body != nil {
		switch v := b.resp.Body.(type) {
		case string:
			_, _ = w.WriteString(v)
		case []byte:
			_, _ = w.Write(v)
		default:
			// Assume JSON encoding for other types
			_ = json.NewEncoder(w).Encode(v)
		}
	}

	return w
}

// Common request presets

// APIGetRequest creates a typical API GET request
func APIGetRequest(path string) *RequestBuilder {
	return NewRequest().
		GET(path).
		WithHeader("Accept", "application/json")
}

// APIPostRequest creates a typical API POST request with JSON body
func APIPostRequest(path string, body interface{}) *RequestBuilder {
	return NewRequest().
		POST(path).
		WithJSON().
		WithBody(body)
}

// AuthenticatedRequest creates a request with authentication
func AuthenticatedRequest(token string) *RequestBuilder {
	return NewRequest().
		WithAuth(token).
		WithHeader("Accept", "application/json")
}

// Common response presets

// JSONResponse creates a JSON response
func JSONResponse(data interface{}) *ResponseBuilder {
	return NewResponse().
		OK().
		WithJSONHeader().
		WithBody(data)
}

// ErrorResponse creates an error response
func ErrorResponse(code int, message string) *ResponseBuilder {
	return NewResponse().
		WithStatus(code).
		WithJSONHeader().
		WithBody(map[string]string{"error": message})
}
