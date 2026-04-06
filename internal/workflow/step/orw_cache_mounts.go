package step

import (
	"os"
	"path/filepath"
	"strings"
)

const (
	orwCacheRootEnv = "PLOY_ORW_CACHE_ROOT"
	orwCacheRootDir = "/var/cache/ploy/orw"
	orwTmpCacheRoot = "ploy/orw"

	orwMavenUserHomeDir = "/root/.m2"
)

func buildORWToolCacheMounts(image string) ([]ContainerMount, error) {
	tool, target := orwToolCacheTarget(image)
	if target == "" {
		return nil, nil
	}
	cacheRoot, err := resolveORWCacheRoot()
	if err != nil {
		return nil, err
	}
	hostPath := filepath.Join(
		cacheRoot,
		sanitizeCachePathPart(tool, "unknown-tool"),
		sanitizeCachePathPart(image, "unknown-image"),
	)
	if err := os.MkdirAll(hostPath, 0o755); err != nil {
		return nil, err
	}
	if err := pruneGateCacheDirOldestFirst(hostPath); err != nil {
		return nil, err
	}
	return []ContainerMount{{
		Source:   hostPath,
		Target:   target,
		ReadOnly: false,
	}}, nil
}

func resolveORWCacheRoot() (string, error) {
	if override := strings.TrimSpace(os.Getenv(orwCacheRootEnv)); override != "" {
		if err := os.MkdirAll(override, 0o755); err != nil {
			return "", err
		}
		return override, nil
	}
	if err := os.MkdirAll(orwCacheRootDir, 0o755); err == nil {
		return orwCacheRootDir, nil
	} else if !os.IsPermission(err) {
		return "", err
	}
	fallback := filepath.Join(os.TempDir(), orwTmpCacheRoot)
	if err := os.MkdirAll(fallback, 0o755); err != nil {
		return "", err
	}
	return fallback, nil
}

func orwToolCacheTarget(image string) (tool string, target string) {
	name := strings.ToLower(strings.TrimSpace(image))
	switch {
	case strings.Contains(name, "orw-cli-maven"):
		return "maven", orwMavenUserHomeDir
	case strings.Contains(name, "orw-cli-gradle"):
		// Gradle lane still resolves recipe artifacts via Maven Resolver (~/.m2).
		return "maven", orwMavenUserHomeDir
	default:
		return "", ""
	}
}
