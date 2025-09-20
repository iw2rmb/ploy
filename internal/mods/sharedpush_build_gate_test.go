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
			_, _ = fmt.Fprintf(w, `{"error":{"code":"builder_failed","message":"compile failed","details":"mvn package failed"},"builder":{"job":"%s","logs":%q,"logs_key":"build-logs/%s.log","logs_url":"https://storage.example/build-logs/%s.log"}}`, deploymentID, builderLogs, deploymentID, deploymentID)
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
	downloadURL := fmt.Sprintf("%s/apps/%s/builds/%s/logs/download", strings.TrimRight(server.URL, "/"), appName, deploymentID)
	if !strings.Contains(res.Message, fmt.Sprintf("builder_log:%s", downloadURL)) {
		t.Fatalf("expected message to include builder_log reference, got %q", res.Message)
	}
	if strings.Contains(res.Message, "download full builder log via") {
		t.Fatalf("expected message to omit legacy download helper text, got %q", res.Message)
	}
	if strings.Contains(res.Message, "storage.example") {
		t.Fatalf("expected message to omit direct storage URLs, got %q", res.Message)
	}
	if !strings.Contains(res.Message, "error code: builder_failed") {
		t.Fatalf("expected message to include error code, got %q", res.Message)
	}
	if !strings.Contains(res.Message, "builder job:") {
		t.Fatalf("expected message to include builder job, got %q", res.Message)
	}
	if strings.Contains(res.Message, "{\"id\"") {
		t.Fatalf("expected message to omit raw JSON detail chunks, got %q", res.Message)
	}

	if res.BuilderLogs != builderLogs {
		t.Fatalf("expected DeployResult.BuilderLogs to match, got %q", res.BuilderLogs)
	}
	wantKey := fmt.Sprintf("build-logs/%s.log", deploymentID)
	if res.BuilderLogsKey != wantKey {
		t.Fatalf("expected logs key %q, got %q", wantKey, res.BuilderLogsKey)
	}
	wantURL := downloadURL
	if res.BuilderLogsURL != wantURL {
		t.Fatalf("expected logs url %q, got %q", wantURL, res.BuilderLogsURL)
	}
	if res.BuilderJob != deploymentID {
		t.Fatalf("expected builder job %q, got %q", deploymentID, res.BuilderJob)
	}
	if buildRequests != 1 {
		t.Fatalf("expected 1 build request, got %d", buildRequests)
	}
	if logRequests != 1 {
		t.Fatalf("expected 1 log fetch request, got %d", logRequests)
	}
}
