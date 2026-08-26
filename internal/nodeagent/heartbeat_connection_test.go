package nodeagent

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path"
	"strings"
	"testing"
	"time"

	domainapi "github.com/iw2rmb/ploy/internal/domain/api"
)

func TestBuildURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		base string
		path string
		want string
	}{
		{
			name: "basic",
			base: "http://server.example.com:8080",
			path: "/v1/nodes/x/heartbeat",
			want: "http://server.example.com:8080/v1/nodes/x/heartbeat",
		},
		{
			name: "trailing_slash",
			base: "http://server.example.com:8080/",
			path: "/v1/foo",
			want: "http://server.example.com:8080/v1/foo",
		},
		{
			name: "escapes_node_id",
			base: "http://server.example.com:8080",
			path: path.Join("/v1/nodes", url.PathEscape("node/01 abc"), "heartbeat"),
			want: "http://server.example.com:8080/v1/nodes/node%2F01%20abc/heartbeat",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			u, err := BuildURL(tt.base, tt.path)
			if err != nil {
				t.Fatalf("BuildURL error: %v", err)
			}
			if u != tt.want {
				t.Fatalf("url = %q, want %q", u, tt.want)
			}
		})
	}
}

func TestBuildURLRejectsAbsoluteOrAuthorityPath(t *testing.T) {
	t.Parallel()

	base := "http://server.example.com:8080"
	tests := []struct {
		name string
		p    string
	}{
		{
			name: "https absolute",
			p:    "https://evil.example/x",
		},
		{
			name: "http absolute",
			p:    "http://evil.example/x",
		},
		{
			name: "authority reference",
			p:    "//evil.example/x",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := BuildURL(base, tt.p)
			if err == nil {
				t.Fatalf("BuildURL(%q, %q) expected error, got nil", base, tt.p)
			}
			const want = "path must not include scheme or host"
			if !strings.Contains(err.Error(), want) {
				t.Fatalf("BuildURL(%q, %q) err = %q, want substring %q", base, tt.p, err.Error(), want)
			}
		})
	}
}

func TestSendHeartbeatSuccess(t *testing.T) {
	var receivedPayload domainapi.NodeHeartbeatRequest
	var receivedMap map[string]any
	heartbeatPath := "/v1/nodes/" + testNodeID + "/heartbeat"
	diagnosticsPath := "/v1/nodes/" + testNodeID + "/diagnostics"
	heartbeatRequests := 0

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}

		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("content-type = %s, want application/json", ct)
		}

		if r.URL.Path == diagnosticsPath {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if r.URL.Path != heartbeatPath {
			t.Errorf("path = %s, want %s or %s", r.URL.Path, heartbeatPath, diagnosticsPath)
		}
		heartbeatRequests++

		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body error: %v", err)
		}

		if err := json.Unmarshal(body, &receivedMap); err != nil {
			t.Fatalf("unmarshal payload map error: %v", err)
		}
		if err := json.Unmarshal(body, &receivedPayload); err != nil {
			t.Fatalf("unmarshal payload error: %v", err)
		}

		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	cfg := newAgentConfig(srv.URL,
		withHeartbeatInterval(30*time.Second),
		withHeartbeatTimeout(10*time.Second))

	mgr, err := NewHeartbeatManager(cfg)
	if err != nil {
		t.Fatalf("NewHeartbeatManager error: %v", err)
	}

	ctx := context.Background()
	if err := mgr.sendHeartbeat(ctx); err != nil {
		t.Fatalf("sendHeartbeat error: %v", err)
	}
	if heartbeatRequests != 1 {
		t.Fatalf("heartbeat requests = %d, want 1", heartbeatRequests)
	}

	if _, ok := receivedMap["node_id"]; ok {
		t.Errorf("payload includes node_id, want absent (identity is in URL path)")
	}
	if _, ok := receivedMap["timestamp"]; ok {
		t.Errorf("payload includes timestamp, want absent")
	}

	if receivedPayload.CPUTotalMillis <= 0 {
		t.Error("cpu_total_millis should be > 0")
	}

	if receivedPayload.MemTotalBytes <= 0 {
		t.Error("mem_total_bytes should be > 0")
	}

	if receivedPayload.DiskTotalBytes <= 0 {
		t.Error("disk_total_bytes should be > 0")
	}
}

func TestSendHeartbeatHandlesServerError(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		wantErr    string
	}{
		{
			name:       "bad_request",
			statusCode: http.StatusBadRequest,
			wantErr:    "heartbeat failed with status 400",
		},
		{
			name:       "unauthorized",
			statusCode: http.StatusUnauthorized,
			wantErr:    "heartbeat failed with status 401",
		},
		{
			name:       "internal_error",
			statusCode: http.StatusInternalServerError,
			wantErr:    "heartbeat failed with status 500",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.statusCode)
			}))
			defer srv.Close()

			cfg := newAgentConfig(srv.URL, withHeartbeatTimeout(10*time.Second))

			mgr, err := NewHeartbeatManager(cfg)
			if err != nil {
				t.Fatalf("NewHeartbeatManager error: %v", err)
			}

			ctx := context.Background()
			err = mgr.sendHeartbeat(ctx)
			if err == nil {
				t.Fatal("expected error, got nil")
			}

			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error = %v, want substring %q", err, tt.wantErr)
			}
		})
	}
}
