package nodeagent

import (
	"testing"
)

func TestResolveDockerRegistryAuthConfigFile(t *testing.T) {
	tests := []struct {
		name     string
		filePath string
		want     string
	}{
		{
			name:     "configured_file_used",
			filePath: "/etc/ploy/docker-auth-config/config.json",
			want:     "/etc/ploy/docker-auth-config/config.json",
		},
		{
			name: "no_file_configured",
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("PLOY_DOCKER_AUTH_CONFIG_FILE", tt.filePath)

			got := resolveDockerRegistryAuthConfigFile()
			if got != tt.want {
				t.Fatalf("resolveDockerRegistryAuthConfigFile() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestResolveDockerRegistryAuthRefreshSocket(t *testing.T) {
	tests := []struct {
		name       string
		socketPath string
		want       string
	}{
		{
			name:       "configured_socket_used",
			socketPath: "/run/ploy/node-auth-refresh.sock",
			want:       "/run/ploy/node-auth-refresh.sock",
		},
		{
			name: "no_socket_configured",
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("PLOY_DOCKER_AUTH_REFRESH_SOCKET", tt.socketPath)

			got := resolveDockerRegistryAuthRefreshSocket()
			if got != tt.want {
				t.Fatalf("resolveDockerRegistryAuthRefreshSocket() = %q, want %q", got, tt.want)
			}
		})
	}
}
