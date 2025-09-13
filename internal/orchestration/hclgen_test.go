package orchestration

import (
	"strings"
	"testing"
)

func TestRenderServiceDockerJobHCL_Basic(t *testing.T) {
	hcl := RenderServiceDockerJobHCL("platform-api", "api", "api", "ghcr.io/x/y:1", map[string]string{"FOO": "bar"}, "api.dev.ployman.app", "dev-wildcard", "dev")
	if !strings.Contains(hcl, "job \"platform-api\"") {
		t.Fatalf("missing job header: %s", hcl)
	}
	if !strings.Contains(hcl, "image = \"ghcr.io/x/y:1\"") {
		t.Fatalf("missing image")
	}
	if !strings.Contains(hcl, "FOO = \"bar\"") {
		t.Fatalf("missing env var")
	}
	if !strings.Contains(hcl, "traefik.http.routers.platform-api.rule=Host(\"api.dev.ployman.app\")") {
		t.Fatalf("missing traefik tag")
	}
}

func TestRenderBatchDockerJobHCL_Basic(t *testing.T) {
	env := map[string]string{"X": "1", "Y": "2"}
	hcl := RenderBatchDockerJobHCL("job-1", "grp", "task", "img:latest", env, "http://example/art.tar")
	if !strings.Contains(hcl, "job \"job-1\"") {
		t.Fatalf("missing job header")
	}
	if !strings.Contains(hcl, "config { image = \"img:latest\" }") {
		t.Fatalf("missing image config")
	}
	if !strings.Contains(hcl, "source = \"http://example/art.tar\"") {
		t.Fatalf("missing artifact")
	}
}
