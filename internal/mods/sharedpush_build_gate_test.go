package mods

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/iw2rmb/ploy/internal/cli/common"
)

func TestSharedPushBuildGateIntegration_AppendsBuilderLogs(t *testing.T) {
	t.Parallel()

	const (
		appName      = "test-app"
		deploymentID = "test-app-c-build-dev"
	)
	builderLogs := "[ERROR] Compilation failure\n[INFO] hint: Missing symbol Foo"

	var buildRequests int
	var logRequests int

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/apps/"):
			buildRequests++
			_, _ = io.Copy(io.Discard, r.Body)
			_ = r.Body.Close()
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("X-Deployment-ID", deploymentID)
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = fmt.Fprintf(w, `{"error":{"code":"builder_failed","message":"compile failed"},"builder":{"job":"%s","logs":%q,"logs_key":"build-logs/%s.log","logs_url":"https://storage.example/build-logs/%s.log"}}`, deploymentID, builderLogs, deploymentID, deploymentID)
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/apps/") && strings.Contains(r.URL.Path, "/builds/") && strings.HasSuffix(r.URL.Path, "/logs"):
			logRequests++
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprintf(w, `{"id":"%s","app":"%s","lines":1200,"logs":""}`, deploymentID, appName)
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	t.Cleanup(server.Close)

	workDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(workDir, "pom.xml"), []byte("<project/>"), 0o644); err != nil {
		t.Fatalf("write pom.xml: %v", err)
	}

	checker := NewSharedPushBuildChecker(server.URL)
	cfg := common.DeployConfig{
		App:           appName,
		Lane:          "C",
		SHA:           "dev",
		Environment:   "dev",
		ControllerURL: server.URL,
		BuildOnly:     true,
		WorkingDir:    workDir,
		Timeout:       2 * time.Second,
	}

	res, err := checker.CheckBuild(context.Background(), cfg)
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if res == nil {
		t.Fatalf("expected DeployResult, got nil")
	}
	if res.Success {
		t.Fatalf("expected unsuccessful result")
	}
	if strings.Contains(res.Message, "{\"error\"") {
		t.Fatalf("expected sanitized error message without raw JSON, got %q", res.Message)
	}
	if !strings.Contains(res.Message, "Missing symbol Foo") {
		t.Fatalf("expected result message to include builder logs, got %q", res.Message)
	}
	if buildRequests != 1 {
		t.Fatalf("expected 1 build request, got %d", buildRequests)
	}
	if logRequests != 1 {
		t.Fatalf("expected 1 log fetch request, got %d", logRequests)
	}
}
