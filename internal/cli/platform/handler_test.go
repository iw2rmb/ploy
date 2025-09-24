package platform

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

func captureStdout(t *testing.T) (*bytes.Buffer, func()) {
	t.Helper()

	original := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stdout = w

	buf := &bytes.Buffer{}
	done := make(chan struct{})
	go func() {
		_, _ = io.Copy(buf, r)
		_ = r.Close()
		close(done)
	}()

	cleanup := func() {
		os.Stdout = original
		_ = w.Close()
		<-done
	}

	return buf, cleanup
}

func moveToTempDir(t *testing.T) func() {
	t.Helper()

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}

	tmp := t.TempDir()
	if err := os.Chdir(tmp); err != nil {
		t.Fatalf("chdir: %v", err)
	}

	if err := os.WriteFile("sample.txt", []byte("hello"), 0o644); err != nil {
		t.Fatalf("write sample: %v", err)
	}

	return func() {
		_ = os.Chdir(cwd)
	}
}

func TestPushCmdRoutesPlatformRequest(t *testing.T) {
	restoreWD := moveToTempDir(t)
	defer restoreWD()

	cases := []struct {
		name         string
		args         []string
		expectEnv    string
		expectLane   string
		expectDomain string
	}{
		{
			name:         "dev default",
			args:         []string{"-a", "ploy-api"},
			expectEnv:    "dev",
			expectLane:   "E",
			expectDomain: "ploy-api.dev.ployman.app",
		},
		{
			name:         "prod environment",
			args:         []string{"-a", "ploy-api", "-env", "prod"},
			expectEnv:    "prod",
			expectLane:   "E",
			expectDomain: "ploy-api.ployman.app",
		},
		{
			name:         "custom lane",
			args:         []string{"-a", "openrewrite", "-lane", "C"},
			expectEnv:    "dev",
			expectLane:   "C",
			expectDomain: "openrewrite.dev.ployman.app",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			reqCh := make(chan *http.Request, 1)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				reqCh <- r
				_, _ = io.Copy(io.Discard, r.Body)
				_ = r.Body.Close()
				w.Header().Set("X-Deployment-ID", "platform-456")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`{"status":"success"}`))
			}))
			defer server.Close()

			outBuf, finish := captureStdout(t)
			PushCmd(tc.args, server.URL)
			finish()

			select {
			case req := <-reqCh:
				if got := req.Header.Get("X-Platform-Service"); got != "true" {
					t.Fatalf("expected X-Platform-Service header true, got %q", got)
				}
				if got := req.Header.Get("X-Target-Domain"); got != "ployman.app" {
					t.Fatalf("expected target domain ployman.app, got %q", got)
				}
				if got := req.Header.Get("X-Environment"); got != tc.expectEnv {
					t.Fatalf("expected environment header %q, got %q", tc.expectEnv, got)
				}
				query := req.URL.Query()
				if query.Get("platform") != "true" {
					t.Fatalf("platform query missing: %s", query.Encode())
				}
				if lane := query.Get("lane"); lane != tc.expectLane {
					t.Fatalf("expected lane %q, got %q (query %s)", tc.expectLane, lane, query.Encode())
				}
				if env := query.Get("env"); env != tc.expectEnv {
					t.Fatalf("expected env query %q, got %q", tc.expectEnv, env)
				}
				if query.Get("autogen_dockerfile") != "" {
					t.Fatalf("autogen flag should be absent by default: %s", query.Encode())
				}
			case <-time.After(time.Second):
				t.Fatalf("expected request to controller")
			}

			output := outBuf.String()
			if !strings.Contains(output, "✅ Successfully deployed") {
				t.Fatalf("missing success marker: %s", output)
			}
			if !strings.Contains(output, tc.expectDomain) {
				t.Fatalf("missing expected domain %s in output %s", tc.expectDomain, output)
			}
			if !strings.Contains(output, "\"status\":\"success\"") {
				t.Fatalf("missing controller payload: %s", output)
			}
		})
	}
}

func TestPushCmdRequiresServiceName(t *testing.T) {
	reqCh := make(chan struct{}, 1)
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		reqCh <- struct{}{}
	}))
	defer server.Close()

	outBuf, finish := captureStdout(t)
	PushCmd([]string{}, server.URL)
	finish()

	select {
	case <-reqCh:
		t.Fatalf("push should not contact controller when service missing")
	default:
	}

	if !strings.Contains(outBuf.String(), "platform service name required") {
		t.Fatalf("expected validation message, got %s", outBuf.String())
	}
}

func TestPushCmdPropagatesAutogen(t *testing.T) {
	restoreWD := moveToTempDir(t)
	defer restoreWD()

	t.Setenv("PLOY_AUTOGEN_DOCKERFILE", "1")

	reqCh := make(chan *http.Request, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqCh <- r
		_, _ = io.Copy(io.Discard, r.Body)
		_ = r.Body.Close()
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"success"}`))
	}))
	defer server.Close()

	outBuf, finish := captureStdout(t)
	PushCmd([]string{"-a", "ploy-api"}, server.URL)
	finish()

	select {
	case req := <-reqCh:
		if req.URL.Query().Get("autogen_dockerfile") != "true" {
			t.Fatalf("autogen flag missing: %s", req.URL.RawQuery)
		}
	case <-time.After(time.Second):
		t.Fatalf("expected request to controller")
	}

	if !strings.Contains(outBuf.String(), "status") {
		t.Fatalf("missing controller output: %s", outBuf.String())
	}
}

func TestGetPlatformDomain(t *testing.T) {
	tests := []struct {
		name        string
		service     string
		environment string
		want        string
	}{
		{
			name:        "dev environment",
			service:     "ploy-api",
			environment: "dev",
			want:        "ploy-api.dev.ployman.app",
		},
		{
			name:        "prod environment",
			service:     "ploy-api",
			environment: "prod",
			want:        "ploy-api.ployman.app",
		},
		{
			name:        "staging defaults to dev domain",
			service:     "metrics",
			environment: "staging",
			want:        "metrics.dev.ployman.app",
		},
		{
			name:        "no environment defaults to dev",
			service:     "openrewrite",
			environment: "",
			want:        "openrewrite.dev.ployman.app",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("PLOY_PLATFORM_DOMAIN", "")
			t.Setenv("PLOY_ENVIRONMENT", tt.environment)

			got := getPlatformDomain(tt.service)
			if got != tt.want {
				t.Errorf("getPlatformDomain() = %v, want %v", got, tt.want)
			}
		})
	}
}
