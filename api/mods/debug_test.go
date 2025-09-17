package mods

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/gofiber/fiber/v2"
)

func TestDebugNomadForbiddenWhenDisabled(t *testing.T) {
	kv := &kvMem{}
	app := fiber.New()
	h := NewHandler(nil, nil, kv)
	h.RegisterRoutes(app)

	resp, err := app.Test(httptest.NewRequest(http.MethodGet, "/v1/mods/debug/nomad", nil))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer func() {
		if resp != nil && resp.Body != nil {
			_ = resp.Body.Close()
		}
	}()

	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", resp.StatusCode)
	}
}

func TestDebugNomadReturnsSummary(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "nomad-job-manager.sh")
	scriptBody := `#!/bin/sh
case "$1" in
  jobs)
    printf '[{"Name":"mod-planner-123","SubmitTime":1893456000000}]'
    ;;
  allocs)
    printf '[{"ClientStatus":"running"}]'
    ;;
  *)
    printf '[]'
    ;;
esac
`
	if err := os.WriteFile(script, []byte(scriptBody), 0o755); err != nil {
		t.Fatalf("write script: %v", err)
	}
	t.Setenv("NOMAD_JOB_MANAGER", script)
	t.Setenv("PLOY_DEBUG", "1")

	ln, err := net.Listen("tcp", "127.0.0.1:4646")
	if err != nil {
		t.Skipf("unable to bind debug listener: %v", err)
	}
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/job/mod-planner-123/evaluations" {
			_, _ = w.Write([]byte(`[{"Status":"complete","TriggeredBy":"job-submit"}]`))
			return
		}
		_, _ = w.Write([]byte(`[]`))
	})}
	go func() { _ = srv.Serve(ln) }()
	defer func() { _ = srv.Shutdown(context.Background()) }()

	kv := &kvMem{}
	app := fiber.New()
	h := NewHandler(nil, nil, kv)
	h.RegisterRoutes(app)

	resp, err := app.Test(httptest.NewRequest(http.MethodGet, "/v1/mods/debug/nomad", nil))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer func() {
		if resp != nil && resp.Body != nil {
			_ = resp.Body.Close()
		}
	}()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var payload struct {
		RecentJobs []map[string]any `json:"recent_jobs"`
		Count      int              `json:"count"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if payload.Count == 0 || len(payload.RecentJobs) == 0 {
		t.Fatalf("expected recent jobs, got %#v", payload)
	}
}
