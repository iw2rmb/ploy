package unit_test

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestPloyAPI_RoutersIncludeAppsAlias(t *testing.T) {
	p := filepath.FromSlash(filepath.Join("..", "..", "iac", "common", "templates", "nomad-ploy-api.hcl.j2"))
	content := mustReadFile(t, p)
	if !strings.Contains(content, "traefik.http.routers.ploy-api.rule=Host(`api.dev.ployman.app`)") {
		t.Fatalf("missing platform domain router for ploy-api in %s", p)
	}
	if !strings.Contains(content, "traefik.http.routers.ploy-api-apps.rule=Host(`api.dev.{{ ploy.apps_domain }}`)") {
		t.Fatalf("missing apps domain alias router for ploy-api in %s", p)
	}
	if !strings.Contains(content, "traefik.http.routers.ploy-api-apps.tls.certresolver=apps-wildcard") {
		t.Fatalf("missing apps-wildcard certresolver for alias in %s", p)
	}
}
