package orchestration

import (
	_ "embed"
	"strings"
)

// Embed the Lane C templates needed by the build path.
//
//go:embed templates/lane-c-osv.hcl
var embeddedLaneCOsv []byte

//go:embed templates/lane-c-java.hcl
var embeddedLaneCJava []byte

// Embed builder job templates used by internal/orchestration renderers
//
//go:embed templates/lane-e-kaniko-builder.hcl
var embeddedLaneEKanikoBuilder []byte

//go:embed templates/lane-c-osv-builder.hcl
var embeddedLaneCOsvBuilder []byte

//go:embed templates/lane-g-wasm-builder.hcl
var embeddedLaneGWasmBuilder []byte

func getEmbeddedTemplate(path string) []byte {
	if strings.HasSuffix(path, "lane-c-osv.hcl") {
		return embeddedLaneCOsv
	}
	if strings.HasSuffix(path, "lane-c-java.hcl") {
		return embeddedLaneCJava
	}
	if strings.HasSuffix(path, "lane-e-kaniko-builder.hcl") {
		return embeddedLaneEKanikoBuilder
	}
	if strings.HasSuffix(path, "lane-c-osv-builder.hcl") {
		return embeddedLaneCOsvBuilder
	}
	if strings.HasSuffix(path, "lane-g-wasm-builder.hcl") {
		return embeddedLaneGWasmBuilder
	}
	return nil
}
