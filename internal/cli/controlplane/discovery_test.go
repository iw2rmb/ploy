package controlplane

import (
    "io"
    "net/http"
    "net/http/httptest"
    "net/url"
    "strings"
    "testing"
)

// Test that multiEndpointTransport fails over from a 503 to the next endpoint
// and returns the successful response.
func TestMultiEndpointTransport_Failover(t *testing.T) {
    s1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.WriteHeader(http.StatusServiceUnavailable)
        _, _ = w.Write([]byte("one"))
    }))
    defer s1.Close()
    s2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        _, _ = io.WriteString(w, "two")
    }))
    defer s2.Close()

    u1, _ := url.Parse(s1.URL)
    u2, _ := url.Parse(s2.URL)

    tr := &multiEndpointTransport{
        endpoints:    []*url.URL{u1, u2},
        base:         http.DefaultTransport,
        retryStatuses: map[int]struct{}{502: {}, 503: {}, 504: {}},
    }
    client := &http.Client{Transport: tr}

    // The path on the request should be preserved; host/scheme replaced.
    req, _ := http.NewRequest(http.MethodGet, "http://placeholder.local/test", nil)
    resp, err := client.Do(req)
    if err != nil {
        t.Fatalf("client.Do error: %v", err)
    }
    defer resp.Body.Close()
    if resp.StatusCode != http.StatusOK {
        t.Fatalf("unexpected status: %s", resp.Status)
    }
    data, _ := io.ReadAll(resp.Body)
    if got := strings.TrimSpace(string(data)); got != "two" {
        t.Fatalf("unexpected body: %q", got)
    }
}

