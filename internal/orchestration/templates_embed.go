package orchestration

import (
    _ "embed"
    "strings"
)

// Embed the Lane C template needed by the build path.
//go:embed templates/lane-c-osv.hcl
var embeddedLaneCOsv []byte

func getEmbeddedTemplate(path string) []byte {
    if strings.HasSuffix(path, "lane-c-osv.hcl") {
        return embeddedLaneCOsv
    }
    return nil
}
