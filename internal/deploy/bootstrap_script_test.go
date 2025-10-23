package deploy_test

import (
	"strings"
	"testing"

	"github.com/iw2rmb/ploy/internal/deploy"
)

func TestBootstrapScriptInstallsPloydService(t *testing.T) {
	script := deploy.ScriptTemplate()
	if !strings.Contains(script, "ployd.service") {
		t.Fatalf("bootstrap script missing ployd service declaration")
	}
	if !strings.Contains(script, "ExecStart=${BIN_DIR}/ployd") {
		t.Fatalf("bootstrap script missing ployd ExecStart")
	}
}
