package step

import "strings"

const sbomGradleImageMarker = "sbom-gradle"

func buildSBOMToolCacheMounts(image string) ([]ContainerMount, error) {
	name := strings.ToLower(strings.TrimSpace(image))
	if !strings.Contains(name, sbomGradleImageMarker) {
		return nil, nil
	}
	return buildGateToolCacheMounts("java", "gradle", "17")
}
