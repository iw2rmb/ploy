package bootstrap

import (
	"strings"
	"testing"
)

func TestDefaultExportsAndPrefixedScript(t *testing.T) {
	env := DefaultExports()
	if env["PLOY_BOOTSTRAP_VERSION"] == "" {
		t.Fatalf("expected PLOY_BOOTSTRAP_VERSION in defaults")
	}

	script := PrefixedScript(map[string]string{
		"FOO": "bar baz",
		"QUX": "qu\"ote",
	})
	if !strings.Contains(script, "export FOO=\"bar baz\"") {
		t.Fatalf("missing FOO export in script: %q", script)
	}
	if !strings.Contains(script, "export QUX=\"qu\\\"ote\"") {
		t.Fatalf("missing escaped QUX export in script: %q", script)
	}
	if !strings.Contains(script, "# ploy bootstrap body") {
		t.Fatalf("missing body stub in script")
	}
}
