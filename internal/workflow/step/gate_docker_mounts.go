package step

import (
	"os"
	"path/filepath"
	"strings"
)

func appendDockerHostSocketMount(mounts []ContainerMount, env map[string]string) []ContainerMount {
	socketPath := dockerHostSocketPathFromEnv(env)
	if socketPath == "" {
		return mounts
	}
	info, err := os.Stat(socketPath)
	if err != nil || info.IsDir() {
		return mounts
	}
	for _, mount := range mounts {
		if mount.Target == socketPath {
			return mounts
		}
	}
	return append(mounts, ContainerMount{
		Source:   socketPath,
		Target:   socketPath,
		ReadOnly: false,
	})
}

func buildGateToolCacheMounts(language, tool, release string) ([]ContainerMount, error) {
	target := buildGateToolCacheTarget(tool)
	if target == "" {
		return nil, nil
	}
	cacheRoot, err := resolveBuildGateCacheRoot()
	if err != nil {
		return nil, err
	}
	hostPath := filepath.Join(
		cacheRoot,
		sanitizeCachePathPart(language, "unknown-lang"),
		sanitizeCachePathPart(tool, "unknown-tool"),
		sanitizeCachePathPart(release, "unknown-release"),
	)
	if err := os.MkdirAll(hostPath, 0o750); err != nil {
		return nil, err
	}
	return []ContainerMount{{
		Source:   hostPath,
		Target:   target,
		ReadOnly: false,
	}}, nil
}

func resolveBuildGateCacheRoot() (string, error) {
	if override := strings.TrimSpace(os.Getenv(buildGateCacheRootEnv)); override != "" {
		if err := os.MkdirAll(override, 0o750); err != nil {
			return "", err
		}
		return override, nil
	}
	if err := os.MkdirAll(buildGateCacheRootDir, 0o750); err == nil {
		return buildGateCacheRootDir, nil
	} else if !os.IsPermission(err) {
		return "", err
	}
	fallback := filepath.Join(os.TempDir(), buildGateTmpCacheRoot)
	if err := os.MkdirAll(fallback, 0o750); err != nil {
		return "", err
	}
	return fallback, nil
}

func buildGateToolCacheTarget(tool string) string {
	switch strings.ToLower(strings.TrimSpace(tool)) {
	case "gradle":
		return BuildGateGradleUserHomeDir
	case "maven":
		return BuildGateMavenUserHomeDir
	default:
		return ""
	}
}

func sanitizeCachePathPart(value, fallback string) string {
	s := strings.ToLower(strings.TrimSpace(value))
	if s == "" {
		return fallback
	}
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch {
		case (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'):
			b.WriteRune(r)
		case r == '.' || r == '_' || r == '-':
			b.WriteRune(r)
		default:
			b.WriteByte('-')
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		return fallback
	}
	return out
}

func dockerHostSocketPathFromEnv(env map[string]string) string {
	if len(env) == 0 {
		return ""
	}
	dockerHost := strings.TrimSpace(env["DOCKER_HOST"])
	if dockerHost == "" || !strings.HasPrefix(dockerHost, "unix://") {
		return ""
	}
	socketPath := strings.TrimSpace(strings.TrimPrefix(dockerHost, "unix://"))
	if socketPath == "" || !filepath.IsAbs(socketPath) {
		return ""
	}
	return socketPath
}
