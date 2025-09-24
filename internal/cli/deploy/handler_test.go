package deploy

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

func TestPushCmdUsesControllerOverride(t *testing.T) {
	restoreWD := moveToTempDir(t)
	defer restoreWD()

	t.Setenv("PLOY_CONTROLLER", "")
	t.Setenv("PLOY_ASYNC", "0")

	reqCh := make(chan *http.Request, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqCh <- r
		_, _ = io.Copy(io.Discard, r.Body)
		if err := r.Body.Close(); err != nil {
			t.Fatalf("close body: %v", err)
		}
		w.Header().Set("X-Deployment-ID", "dep-123")
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	defer server.Close()

	outBuf, finish := captureStdout(t)

	PushCmd([]string{"-a", "test-app", "-sha", "abc123"}, server.URL)

	finish()

	select {
	case req := <-reqCh:
		if got := req.URL.Path; got != "/apps/test-app/builds" {
			t.Fatalf("path = %s, want /apps/test-app/builds", got)
		}
		if req.URL.Query().Get("sha") != "abc123" {
			t.Fatalf("sha query missing: %v", req.URL.RawQuery)
		}
		if got := req.URL.Query().Get("async"); got != "" {
			t.Fatalf("async query should be omitted when disabled, got %q", got)
		}
		if ct := req.Header.Get("Content-Type"); ct != "application/x-tar" {
			t.Fatalf("content-type = %s, want application/x-tar", ct)
		}
	case <-time.After(time.Second):
		t.Fatalf("expected request to controller override")
	}

	if !strings.Contains(outBuf.String(), "✅ Successfully deployed") {
		t.Fatalf("output missing success message: %s", outBuf.String())
	}
	if !strings.Contains(outBuf.String(), "\"status\":\"ok\"") {
		t.Fatalf("output missing controller payload: %s", outBuf.String())
	}
}

func TestPushCmdMultipartAndAutogen(t *testing.T) {
	restoreWD := moveToTempDir(t)
	defer restoreWD()

	if err := os.WriteFile("app.py", []byte("print('hello')"), 0o644); err != nil {
		t.Fatalf("write app.py: %v", err)
	}

	t.Setenv("PLOY_AUTOGEN_DOCKERFILE", "1")
	t.Setenv("PLOY_PUSH_MULTIPART", "1")

	reqCh := make(chan *http.Request, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqCh <- r
		_, _ = io.Copy(io.Discard, r.Body)
		_ = r.Body.Close()
		w.Header().Set("X-Deployment-ID", "dep-456")
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(`{"status":"queued"}`))
	}))
	defer server.Close()

	outBuf, finish := captureStdout(t)

	PushCmd([]string{"-a", "demo", "-main", "main.Class", "-sha", "sha-xyz"}, server.URL)

	finish()

	select {
	case req := <-reqCh:
		ct := req.Header.Get("Content-Type")
		if !strings.HasPrefix(ct, "multipart/form-data") {
			t.Fatalf("content-type = %s, want multipart/form-data", ct)
		}
		q := req.URL.Query()
		if q.Get("autogen_dockerfile") != "true" {
			t.Fatalf("autogen flag missing: %s", q.Encode())
		}
		if q.Get("async") != "true" {
			t.Fatalf("async flag missing: %s", q.Encode())
		}
	case <-time.After(time.Second):
		t.Fatalf("expected request to controller")
	}

	if _, err := os.Stat("Dockerfile"); err != nil {
		t.Fatalf("Dockerfile not generated: %v", err)
	}

	if !strings.Contains(outBuf.String(), "queued") {
		t.Fatalf("output missing queued status: %s", outBuf.String())
	}
}
func TestPushCmdBlueGreen(t *testing.T) {
	restoreWD := moveToTempDir(t)
	defer restoreWD()

	reqCh := make(chan struct{}, 1)
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		reqCh <- struct{}{}
	}))
	defer server.Close()

	outBuf, finish := captureStdout(t)

	PushCmd([]string{"-a", "test-app", "-blue-green"}, server.URL)

	finish()

	select {
	case <-reqCh:
		t.Fatalf("blue-green path should not contact server")
	default:
	}

	if !strings.Contains(outBuf.String(), "Blue-green deployments are handled") {
		t.Fatalf("output missing guidance: %s", outBuf.String())
	}
}

func TestPushCmdIgnoresLaneOverride(t *testing.T) {
	restoreWD := moveToTempDir(t)
	defer restoreWD()

	reqCh := make(chan *http.Request, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqCh <- r
		_, _ = io.Copy(io.Discard, r.Body)
		_ = r.Body.Close()
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	defer server.Close()

	outBuf, finish := captureStdout(t)

	PushCmd([]string{"-a", "lane-override", "-lane", "G", "-sha", "123"}, server.URL)

	finish()

	select {
	case req := <-reqCh:
		if got := req.URL.Query().Get("lane"); got != "" {
			t.Fatalf("lane query should be empty, got %q", got)
		}
		if req.URL.Query().Get("async") != "true" {
			t.Fatalf("async query should default to true: %v", req.URL.Query())
		}
	case <-time.After(time.Second):
		t.Fatalf("expected request to controller")
	}

	if !strings.Contains(outBuf.String(), "Lane overrides are ignored") {
		t.Fatalf("expected informational message about lane overrides, got: %s", outBuf.String())
	}
}
