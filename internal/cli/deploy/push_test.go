package deploy

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

func TestDeployAppMultipartAndAutogen(t *testing.T) {
	restoreWD := moveToTempDir(t)
	defer restoreWD()

	if err := os.WriteFile("app.py", []byte("print('hello')"), 0o644); err != nil {
		t.Fatalf("write app.py: %v", err)
	}

	reqCh := make(chan *http.Request, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqCh <- r
		_, _ = io.Copy(io.Discard, r.Body)
		if err := r.Body.Close(); err != nil {
			t.Fatalf("close body: %v", err)
		}
		w.Header().Set("X-Deployment-ID", "dep-456")
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(`{"status":"queued"}`))
	}))
	defer server.Close()

	t.Setenv("PLOY_CONTROLLER", server.URL)
	t.Setenv("PLOY_PUSH_MULTIPART", "1")
	t.Setenv("PLOY_AUTOGEN_DOCKERFILE", "1")

	outBuf, finish := captureStdout(t)

	result, err := DeployApp("demo", "G", "main.Class", "sha-xyz", true, "")
	finish()

	if err != nil {
		t.Fatalf("DeployApp error: %v", err)
	}
	if !result.Success {
		t.Fatalf("expected success, got %#v", result)
	}
	if result.DeploymentID != "dep-456" {
		t.Fatalf("deployment id = %s, want dep-456", result.DeploymentID)
	}

	select {
	case req := <-reqCh:
		ct := req.Header.Get("Content-Type")
		if !strings.HasPrefix(ct, "multipart/form-data") {
			t.Fatalf("content-type = %s, want multipart/form-data", ct)
		}
		q := req.URL.Query()
		if q.Get("lane") != "G" {
			t.Fatalf("lane query missing: %s", req.URL.RawQuery)
		}
		if q.Get("main") != "main.Class" {
			t.Fatalf("main query missing: %s", req.URL.RawQuery)
		}
		if q.Get("blue_green") != "true" {
			t.Fatalf("blue_green query missing: %s", req.URL.RawQuery)
		}
		if q.Get("autogen_dockerfile") != "true" {
			t.Fatalf("autogen flag missing: %s", req.URL.RawQuery)
		}
		if q.Get("async") != "true" {
			t.Fatalf("async flag missing: %s", req.URL.RawQuery)
		}
	case <-time.After(time.Second):
		t.Fatalf("expected request to controller")
	}

	if _, err := os.Stat("Dockerfile"); err != nil {
		t.Fatalf("Dockerfile not generated: %v", err)
	}

	if !strings.Contains(outBuf.String(), "queued") {
		t.Fatalf("stdout missing response body: %s", outBuf.String())
	}
}

func TestDeployAppRequiresName(t *testing.T) {
	if _, err := DeployApp("", "", "", "", false, ""); err == nil {
		t.Fatalf("expected error for missing app name")
	}
}
