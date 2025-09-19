package mods

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/iw2rmb/ploy/internal/cli/common"
)

func TestEnrichBuilderLogsParsesJSONPayload(t *testing.T) {
	t.Parallel()

	const (
		appName      = "sample-app"
		deploymentID = "deploy-123"
	)
	compileLogs := "[ERROR] Compilation failure\n[INFO] hint: Missing symbol Foo"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("unexpected method %q", r.Method)
		}
		if r.URL.Path != fmt.Sprintf("/apps/%s/builds/%s/logs", appName, deploymentID) {
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"logs":%q,"lines":1200}`, compileLogs)
	}))
	t.Cleanup(server.Close)

	result := &common.DeployResult{Success: false, DeploymentID: deploymentID}

	enrichBuilderLogs(server.URL, appName, result)

	if result.BuilderLogs != compileLogs {
		t.Fatalf("expected BuilderLogs to contain parsed logs, got %q", result.BuilderLogs)
	}
	if result.BuilderLogsKey == "" {
		t.Fatalf("expected BuilderLogsKey to be populated")
	}
	if result.BuilderLogsURL == "" {
		t.Fatalf("expected BuilderLogsURL to be populated")
	}
	if result.Message == "" {
		t.Fatalf("expected result message to capture log snippet")
	}
	if !containsAll(result.Message, []string{"Compilation failure", result.BuilderLogsKey}) {
		t.Fatalf("expected message to include log snippet and key, got %q", result.Message)
	}
}

func containsAll(haystack string, needles []string) bool {
	for _, needle := range needles {
		if !strings.Contains(haystack, needle) {
			return false
		}
	}
	return true
}
