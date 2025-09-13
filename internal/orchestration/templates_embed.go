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

func getEmbeddedTemplate(path string) []byte {
	if strings.HasSuffix(path, "lane-c-osv.hcl") {
		return embeddedLaneCOsv
	}
	if strings.HasSuffix(path, "lane-c-java.hcl") {
		return embeddedLaneCJava
	}
	return nil
}
