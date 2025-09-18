package build

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/iw2rmb/ploy/internal/config"
	"github.com/iw2rmb/ploy/internal/detect/project"
	mem "github.com/iw2rmb/ploy/internal/storage/providers/memory"
)

// TestLaneE_VerifyPushFailure_ReturnsBuilderPointers ensures that when verify push fails
// we attach full builder logs, set X-Deployment-ID, and include builder.logs_key/logs_url.
func TestLaneE_VerifyPushFailure_ReturnsBuilderPointers(t *testing.T) {
	// Stub builder phases to succeed until verify, then force verify failure
	oldRender := renderKanikoBuilderFn
	oldValidate := validateJobFn
	oldSubmit := submitAndWaitFn
	oldVerify := verifyOCIPushFn
	renderKanikoBuilderFn = func(appName, version, tag, contextURL, dockerfilePath, lang string) (string, error) {
		path := filepath.Join(t.TempDir(), "builder.hcl")
		if err := os.WriteFile(path, []byte("job \"test\" {}"), 0644); err != nil {
			t.Fatalf("write hcl: %v", err)
		}
		return path, nil
	}
	validateJobFn = func(string) error { return nil }
	submitAndWaitFn = func(hcl string, d time.Duration) error { return nil }
	verifyOCIPushFn = func(tag string) verifyResult {
		return verifyResult{OK: false, Status: 404, Message: "manifest unknown"}
	}
	t.Cleanup(func() {
		renderKanikoBuilderFn = oldRender
		validateJobFn = oldValidate
		submitAndWaitFn = oldSubmit
		verifyOCIPushFn = oldVerify
	})

	// Memory storage
	storage := mem.NewMemoryStorage(0)
	deps := &BuildDependencies{Storage: storage}
	buildCtx := &BuildContext{APIContext: "apps", AppType: config.UserApp}

	// Ensure logs base
	_ = os.Setenv("PLOY_SEAWEEDFS_URL", "http://seaweedfs-filer.service.consul:8888")
	t.Cleanup(func() { _ = os.Unsetenv("PLOY_SEAWEEDFS_URL") })

	// Prepare a minimal source tree with Dockerfile (to avoid autogen path)
	tmp := t.TempDir()
	src := filepath.Join(tmp, "src")
	_ = os.MkdirAll(src, 0755)
	_ = os.WriteFile(filepath.Join(src, "Dockerfile"), []byte("FROM scratch\n"), 0644)

	app := fiber.New()
	app.Get("/t", func(c *fiber.Ctx) error {
		facts := project.BuildFacts{Language: "java", HasDockerfile: true}
		_, _, _, err := buildLaneE(c, deps, buildCtx, "appv", src, "sha0001", tmp, "java", facts, map[string]string{})
		if err != nil {
			return err
		}
		return nil
	})

	req := httptest.NewRequest("GET", "/t", nil)
	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	t.Cleanup(func() {
		if resp != nil && resp.Body != nil {
			_ = resp.Body.Close()
		}
	})
	if resp.StatusCode != 500 {
		t.Fatalf("status=%d want 500", resp.StatusCode)
	}
	if depID := resp.Header.Get("X-Deployment-ID"); depID == "" {
		t.Fatalf("missing X-Deployment-ID header")
	}
	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["stage"] != "verify_push" {
		t.Fatalf("expected stage=verify_push, got %#v", body["stage"])
	}
	b, ok := body["builder"].(map[string]any)
	if !ok {
		t.Fatalf("missing builder in response: %#v", body)
	}
	if b["logs_key"] == nil || b["logs_url"] == nil {
		t.Fatalf("missing logs_key/logs_url in builder: %#v", b)
	}
}
