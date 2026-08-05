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
	return append(mounts, ContainerMount{
		Source:   socketPath,
		Target:   socketPath,
		ReadOnly: false,
	})
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
