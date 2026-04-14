package step

import (
	"strings"
)

const orwCacheRelease = "17"

func buildORWToolCacheMounts(image string) ([]ContainerMount, error) {
	if !isORWRuntimeImage(image) {
		return nil, nil
	}
	// ORW maven and gradle lanes both resolve runtime artifacts via Maven Resolver.
	return buildGateToolCacheMounts("java", "maven", orwCacheRelease)
}

func isORWRuntimeImage(image string) bool {
	name := strings.ToLower(strings.TrimSpace(image))
	switch {
	case strings.Contains(name, "orw-cli-maven"):
		return true
	case strings.Contains(name, "orw-cli-gradle"):
		return true
	default:
		return false
	}
}
