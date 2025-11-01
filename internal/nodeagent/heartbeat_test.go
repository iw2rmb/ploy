package nodeagent

import (
    "net/url"
    "path"
    "testing"
)

func TestBuildURLBasic(t *testing.T) {
    u, err := buildURL("https://server.example.com:8443", "/v1/nodes/x/heartbeat")
    if err != nil {
        t.Fatalf("buildURL error: %v", err)
    }
    want := "https://server.example.com:8443/v1/nodes/x/heartbeat"
    if u != want {
        t.Fatalf("url = %q, want %q", u, want)
    }
}

func TestBuildURLTrailingSlash(t *testing.T) {
    u, err := buildURL("https://server.example.com:8443/", "/v1/foo")
    if err != nil {
        t.Fatalf("buildURL error: %v", err)
    }
    want := "https://server.example.com:8443/v1/foo"
    if u != want {
        t.Fatalf("url = %q, want %q", u, want)
    }
}

func TestBuildURLEscapesNodeID(t *testing.T) {
    base := "https://server.example.com:8443"
    nodeID := "node/01 abc"
    p := path.Join("/v1/nodes", url.PathEscape(nodeID), "heartbeat")
    u, err := buildURL(base, p)
    if err != nil {
        t.Fatalf("buildURL error: %v", err)
    }
    want := "https://server.example.com:8443/v1/nodes/node%2F01%20abc/heartbeat"
    if u != want {
        t.Fatalf("url = %q, want %q", u, want)
    }
}

