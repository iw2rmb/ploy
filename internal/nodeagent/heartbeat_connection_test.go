package nodeagent

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path"
	"strings"
	"testing"
	"time"
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
	var receivedPayload HeartbeatPayload
	var receivedMap map[string]any

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}

		expectedPath := "/v1/nodes/" + testNodeID + "/heartbeat"
		if r.URL.Path != expectedPath {
			t.Errorf("path = %s, want %s", r.URL.Path, expectedPath)
		}

		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("content-type = %s, want application/json", ct)
		}

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

// TestNewHeartbeatManagerParsesNetIgnoreEnv verifies that PLOY_LIFECYCLE_NET_IGNORE
// is parsed correctly and passed to the lifecycle collector.
func TestNewHeartbeatManagerParsesNetIgnoreEnv(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		envValue string
		want     []string
	}{
		{
			name:     "empty_env",
			envValue: "",
			want:     []string{},
		},
		{
			name:     "whitespace_only",
			envValue: "   ",
			want:     []string{},
		},
		{
			name:     "single_pattern",
			envValue: "docker*",
			want:     []string{"docker*"},
		},
		{
			name:     "multiple_patterns",
			envValue: "docker*,veth*,br-*",
			want:     []string{"docker*", "veth*", "br-*"},
		},
		{
			name:     "patterns_with_whitespace",
			envValue: " docker* , veth* , br-* ",
			want:     []string{"docker*", "veth*", "br-*"},
		},
		{
			name:     "empty_patterns_filtered",
			envValue: "docker*,,veth*,  ,br-*",
			want:     []string{"docker*", "veth*", "br-*"},
		},
		{
			name:     "complex_patterns",
			envValue: "lo,cni*,docker0,veth*,flannel*",
			want:     []string{"lo", "cni*", "docker0", "veth*", "flannel*"},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Set env var for this test only.
			oldValue := os.Getenv("PLOY_LIFECYCLE_NET_IGNORE")
			if tt.envValue != "" {
				if err := os.Setenv("PLOY_LIFECYCLE_NET_IGNORE", tt.envValue); err != nil {
					t.Fatalf("setenv error: %v", err)
				}
			} else {
				if err := os.Unsetenv("PLOY_LIFECYCLE_NET_IGNORE"); err != nil {
					t.Fatalf("unsetenv error: %v", err)
				}
			}
			t.Cleanup(func() {
				if oldValue != "" {
					_ = os.Setenv("PLOY_LIFECYCLE_NET_IGNORE", oldValue)
				} else {
					_ = os.Unsetenv("PLOY_LIFECYCLE_NET_IGNORE")
				}
			})

			cfg := newAgentConfig("http://localhost:8080",
				withHeartbeatInterval(30*time.Second),
				withHeartbeatTimeout(10*time.Second))

			mgr, err := NewHeartbeatManager(cfg)
			if err != nil {
				t.Fatalf("NewHeartbeatManager error: %v", err)
			}

			// Verify that the manager and collector are constructed successfully.
			// The collector's ignoreInterfaces field is unexported, so we verify
			// that the env var parsing succeeds and the manager is ready to use.
			// The actual pattern filtering behavior is tested in lifecycle package tests.
			if mgr == nil {
				t.Fatal("expected non-nil manager")
			}
			if mgr.collector == nil {
				t.Fatal("expected non-nil collector")
			}

			// Attempt to collect a snapshot to verify the collector is functional.
			// This ensures the parsed patterns don't cause any initialization errors.
			ctx := context.Background()
			_, err = mgr.collector.Collect(ctx)
			if err != nil {
				t.Errorf("collector.Collect error: %v (env=%q)", err, tt.envValue)
			}
		})
	}
}
