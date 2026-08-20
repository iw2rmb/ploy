package nodeagent

import (
	"net/http"
	"net/http/httptest"
	"testing"

	types "github.com/iw2rmb/ploy/internal/domain/types"
)

func TestCreateHTTPClientAddsHeadersWithoutMutatingOriginal(t *testing.T) {
	t.Parallel()

	var gotHeader http.Header
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		gotHeader = req.Header.Clone()
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)

	client, err := createHTTPClient(Config{NodeID: types.NodeID("local1")})
	if err != nil {
		t.Fatalf("createHTTPClient() failed: %v", err)
	}
	req, err := http.NewRequest(http.MethodGet, server.URL+"/v1/health", nil)
	if err != nil {
		t.Fatalf("NewRequest() failed: %v", err)
	}
	req.Header.Set("X-Test", "1")

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Do() failed: %v", err)
	}
	_ = resp.Body.Close()

	if got := gotHeader.Get("Authorization"); got != "Bearer test-token" {
		t.Fatalf("Authorization header = %q, want %q", got, "Bearer test-token")
	}
	if got := gotHeader.Get("PLOY_NODE_UUID"); got != "local1" {
		t.Fatalf("PLOY_NODE_UUID header = %q, want %q", got, "local1")
	}
	if got := gotHeader.Get("X-Test"); got != "1" {
		t.Fatalf("X-Test header = %q, want %q", got, "1")
	}

	if got := req.Header.Get("Authorization"); got != "" {
		t.Fatalf("original Authorization header = %q, want empty", got)
	}
	if got := req.Header.Get("PLOY_NODE_UUID"); got != "" {
		t.Fatalf("original PLOY_NODE_UUID header = %q, want empty", got)
	}
}

func TestCreateHTTPClientHandlesNilHeaderMap(t *testing.T) {
	t.Parallel()

	var gotHeader http.Header
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		gotHeader = req.Header.Clone()
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)

	client, err := createHTTPClient(Config{NodeID: types.NodeID("local1")})
	if err != nil {
		t.Fatalf("createHTTPClient() failed: %v", err)
	}
	req, err := http.NewRequest(http.MethodGet, server.URL+"/v1/health", nil)
	if err != nil {
		t.Fatalf("NewRequest() failed: %v", err)
	}
	req.Header = nil

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Do() failed: %v", err)
	}
	_ = resp.Body.Close()
	if got := gotHeader.Get("Authorization"); got != "Bearer test-token" {
		t.Fatalf("Authorization header = %q, want %q", got, "Bearer test-token")
	}
	if got := gotHeader.Get("PLOY_NODE_UUID"); got != "local1" {
		t.Fatalf("PLOY_NODE_UUID header = %q, want %q", got, "local1")
	}
}
