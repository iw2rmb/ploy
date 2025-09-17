package build

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/iw2rmb/ploy/internal/config"
	"github.com/iw2rmb/ploy/internal/detect/project"
	mem "github.com/iw2rmb/ploy/internal/storage/providers/memory"
)

// TestLaneE_BuilderFailure_UploadsLogsAndPointer verifies that on builder job failure
// we upload full logs to storage and include logs_key/logs_url in the JSON response.
func TestLaneE_BuilderFailure_UploadsLogsAndPointer(t *testing.T) {
	// Save and restore injected functions
	oldSubmit := submitAndWaitFn
	oldFetch := fetchJobLogsFullFn
	submitAndWaitFn = func(hcl string, d time.Duration) error { return assertErr("builder failed") }
	fetchJobLogsFullFn = func(job string, lines int) string { return "[ERROR] cannot find symbol Foo\nCompilation failure" }
	defer func() { submitAndWaitFn = oldSubmit; fetchJobLogsFullFn = oldFetch }()

	// Prepare temp dirs and inputs
	tmpDir := t.TempDir()
	srcDir := filepath.Join(tmpDir, "src")
	if err := os.MkdirAll(srcDir, 0755); err != nil {
		t.Fatal(err)
	}
	// Minimal Dockerfile to avoid autogen path
	if err := os.WriteFile(filepath.Join(srcDir, "Dockerfile"), []byte("FROM scratch\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// Memory storage
	storage := mem.NewMemoryStorage(0)
	deps := &BuildDependencies{Storage: storage}
	buildCtx := &BuildContext{APIContext: "apps", AppType: config.UserApp}

	// Ensure logs_url is constructed in response JSON
	_ = os.Setenv("PLOY_SEAWEEDFS_URL", "http://seaweedfs-filer.service.consul:8888")
	t.Cleanup(func() { _ = os.Unsetenv("PLOY_SEAWEEDFS_URL") })

	// Fiber app to capture JSON response from buildLaneE
	app := fiber.New()
	app.Get("/test", func(c *fiber.Ctx) error {
		// Facts: indicate Dockerfile exists
		facts := project.BuildFacts{Language: "java", HasDockerfile: true}
		// Call the function under test; expect JSON 500 with builder pointers
		_, _, _, err := buildLaneE(c, deps, buildCtx, "app-e", srcDir, "sha123", tmpDir, "java", facts, map[string]string{})
		if err != nil {
			return err
		}
		return c.SendStatus(200)
	})

	req := httptest.NewRequest("GET", "/test", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	// ensure body close to satisfy linters
	t.Cleanup(func() {
		if resp != nil && resp.Body != nil {
			_ = resp.Body.Close()
		}
	})
	// Expect 500 from c.Status(500).JSON(...)
	if resp.StatusCode != 500 {
		t.Fatalf("status=%d want 500", resp.StatusCode)
	}
	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	builder, ok := body["builder"].(map[string]any)
	if !ok {
		t.Fatalf("missing builder in response: %#v", body)
	}
	logsKey, _ := builder["logs_key"].(string)
	logsURL, _ := builder["logs_url"].(string)
	if logsKey == "" || !strings.Contains(logsKey, "artifacts/build-logs/") {
		t.Fatalf("logs_key not set or invalid: %q", logsKey)
	}
	if logsURL == "" || !strings.Contains(logsURL, logsKey) {
		t.Fatalf("logs_url not set or does not contain key: %q", logsURL)
	}
	// Verify full logs stored in memory storage
	rc, err := storage.Get(context.Background(), logsKey)
	if err != nil {
		t.Fatalf("storage.Get: %v", err)
	}
	defer func() { _ = rc.Close() }()
	buf := make([]byte, 256)
	n, _ := rc.Read(buf)
	if n == 0 {
		t.Fatalf("stored builder logs are empty")
	}
}

// assertErr is a helper to return a non-nil error inline
func assertErr(msg string) error { return &fakeErr{msg: msg} }

type fakeErr struct{ msg string }

func (e *fakeErr) Error() string { return e.msg }
