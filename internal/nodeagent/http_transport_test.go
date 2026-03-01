package nodeagent

import (
	"io"
	"net/http"
	"strings"
	"testing"

	types "github.com/iw2rmb/ploy/internal/domain/types"
)

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestBearerTokenTransportAddsHeadersWithoutMutatingOriginal(t *testing.T) {
	t.Parallel()

	var gotReq *http.Request
	rt := roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		gotReq = req
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader("ok")),
			Header:     make(http.Header),
		}, nil
	})

	tr := &bearerTokenTransport{
		base:   rt,
		token:  "test-token",
		nodeID: types.NodeID("local1"),
	}

	req, err := http.NewRequest(http.MethodGet, "http://example.test/v1/health", nil)
	if err != nil {
		t.Fatalf("NewRequest() failed: %v", err)
	}
	req.Header.Set("X-Test", "1")

	resp, err := tr.RoundTrip(req)
	if err != nil {
		t.Fatalf("RoundTrip() failed: %v", err)
	}
	_ = resp.Body.Close()

	if gotReq == nil {
		t.Fatalf("base transport did not receive request")
	}
	if got := gotReq.Header.Get("Authorization"); got != "Bearer test-token" {
		t.Fatalf("Authorization header = %q, want %q", got, "Bearer test-token")
	}
	if got := gotReq.Header.Get("PLOY_NODE_UUID"); got != "local1" {
		t.Fatalf("PLOY_NODE_UUID header = %q, want %q", got, "local1")
	}
	if got := gotReq.Header.Get("X-Test"); got != "1" {
		t.Fatalf("X-Test header = %q, want %q", got, "1")
	}

	// Ensure the original request object remains unchanged.
	if got := req.Header.Get("Authorization"); got != "" {
		t.Fatalf("original Authorization header = %q, want empty", got)
	}
	if got := req.Header.Get("PLOY_NODE_UUID"); got != "" {
		t.Fatalf("original PLOY_NODE_UUID header = %q, want empty", got)
	}
}

func TestBearerTokenTransportHandlesNilHeaderMap(t *testing.T) {
	t.Parallel()

	rt := roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		if got := req.Header.Get("Authorization"); got != "Bearer test-token" {
			t.Fatalf("Authorization header = %q, want %q", got, "Bearer test-token")
		}
		if got := req.Header.Get("PLOY_NODE_UUID"); got != "local1" {
			t.Fatalf("PLOY_NODE_UUID header = %q, want %q", got, "local1")
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader("ok")),
			Header:     make(http.Header),
		}, nil
	})

	tr := &bearerTokenTransport{
		base:   rt,
		token:  "test-token",
		nodeID: types.NodeID("local1"),
	}

	req, err := http.NewRequest(http.MethodGet, "http://example.test/v1/health", nil)
	if err != nil {
		t.Fatalf("NewRequest() failed: %v", err)
	}
	req.Header = nil

	resp, err := tr.RoundTrip(req)
	if err != nil {
		t.Fatalf("RoundTrip() failed: %v", err)
	}
	_ = resp.Body.Close()
}
