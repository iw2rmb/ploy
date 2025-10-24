package bootstrap

import (
	"strings"
	"testing"
)

func TestScriptIncludesPloydService(t *testing.T) {
	script := Script()
	if !strings.Contains(script, "ployd.service") {
		t.Fatalf("bootstrap script missing ployd.service declaration")
	}
	if !strings.Contains(script, "ExecStart=${BIN_DIR}/ployd") {
		t.Fatalf("bootstrap script missing ployd ExecStart line")
	}
}
