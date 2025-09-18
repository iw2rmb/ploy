package unit_test

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestPloyAPI_RoutersIncludeAppsAlias(t *testing.T) {
	p := filepath.FromSlash(filepath.Join("iac", "common", "templates", "nomad-ploy-api.hcl.j2"))
	content := mustReadFile(t, p)
	if !strings.Contains(content, "traefik.http.routers.ploy-api.rule=Host(`api.{{ ploy.platform_domain }}`)") {
		t.Fatalf("missing platform domain router for ploy-api in %s", p)
	}
	if !strings.Contains(content, "traefik.http.routers.ploy-api-apps.rule=Host(`api.{{ ploy.apps_domain }}`)") {
		t.Fatalf("missing apps domain alias router for ploy-api in %s", p)
	}
	if !strings.Contains(content, "traefik.http.routers.ploy-api-apps.tls.certresolver=default-acme") {
		t.Fatalf("missing default-acme certresolver for alias in %s", p)
	}
	if strings.Contains(content, "http://localhost:8888/artifacts/api-binaries/") {
		t.Fatalf("nomad-ploy-api.hcl.j2 still references localhost SeaweedFS filer")
	}
	expectedFiler := "http://{{ seaweedfs.filer_domain }}:{{ seaweedfs.filer_port }}/artifacts/api-binaries/{{ api_version }}/linux/amd64/api"
	if !strings.Contains(content, expectedFiler) {
		t.Fatalf("expected SeaweedFS filer reference %q in %s", expectedFiler, p)
	}
}
