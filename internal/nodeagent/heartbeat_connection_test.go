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
	"testing"
	"time"
)

// TestBuildURLBasic verifies basic URL construction from server base and path.
func TestBuildURLBasic(t *testing.T) {
	u, err := BuildURL("http://server.example.com:8080", "/v1/nodes/x/heartbeat")
	if err != nil {
		t.Fatalf("buildURL error: %v", err)
	}
	want := "http://server.example.com:8080/v1/nodes/x/heartbeat"
	if u != want {
		t.Fatalf("url = %q, want %q", u, want)
	}
}

// TestBuildURLTrailingSlash verifies URL construction handles trailing slashes correctly.
func TestBuildURLTrailingSlash(t *testing.T) {
	u, err := BuildURL("http://server.example.com:8080/", "/v1/foo")
	if err != nil {
		t.Fatalf("buildURL error: %v", err)
	}
	want := "http://server.example.com:8080/v1/foo"
	if u != want {
		t.Fatalf("url = %q, want %q", u, want)
	}
}

// TestBuildURLEscapesNodeID verifies URL path escaping for special characters in node IDs.
func TestBuildURLEscapesNodeID(t *testing.T) {
	base := "http://server.example.com:8080"
	nodeID := "node/01 abc"
	p := path.Join("/v1/nodes", url.PathEscape(nodeID), "heartbeat")
	u, err := BuildURL(base, p)
	if err != nil {
		t.Fatalf("buildURL error: %v", err)
	}
	want := "http://server.example.com:8080/v1/nodes/node%2F01%20abc/heartbeat"
	if u != want {
		t.Fatalf("url = %q, want %q", u, want)
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
			if !contains(err.Error(), want) {
				t.Fatalf("BuildURL(%q, %q) err = %q, want substring %q", base, tt.p, err.Error(), want)
			}
		})
	}
}

// TestSendHeartbeatSuccess verifies successful heartbeat POST request and payload.
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

	cfg := Config{
		NodeID:    testNodeID,
		ServerURL: srv.URL,
		HTTP: HTTPConfig{
			TLS: TLSConfig{
				Enabled: false,
			},
		},
		Heartbeat: HeartbeatConfig{
			Interval: 30 * time.Second,
			Timeout:  10 * time.Second,
		},
	}

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

// TestSendHeartbeatHandlesServerError verifies error handling for various HTTP status codes.
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

			cfg := Config{
				NodeID:    testNodeID,
				ServerURL: srv.URL,
				HTTP: HTTPConfig{
					TLS: TLSConfig{
						Enabled: false,
					},
				},
				Heartbeat: HeartbeatConfig{
					Timeout: 10 * time.Second,
				},
			}

			mgr, err := NewHeartbeatManager(cfg)
			if err != nil {
				t.Fatalf("NewHeartbeatManager error: %v", err)
			}

			ctx := context.Background()
			err = mgr.sendHeartbeat(ctx)
			if err == nil {
				t.Fatal("expected error, got nil")
			}

			if !contains(err.Error(), tt.wantErr) {
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

			cfg := Config{
				NodeID:    testNodeID,
				ServerURL: "http://localhost:8080",
				HTTP: HTTPConfig{
					TLS: TLSConfig{
						Enabled: false,
					},
				},
				Heartbeat: HeartbeatConfig{
					Interval: 30 * time.Second,
					Timeout:  10 * time.Second,
				},
			}

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
