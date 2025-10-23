package deploy

import (
	_ "embed"
)

// scriptTemplate holds the bootstrap shell script embedded from assets.
//
//go:embed assets/bootstrap.sh
var scriptTemplate string

// ScriptTemplate returns the embedded bootstrap script.
func ScriptTemplate() string {
	return scriptTemplate
}
