package step

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"github.com/moby/moby/api/types/registry"
)

type dockerAuthConfig struct {
	Auths map[string]dockerAuthEntry `json:"auths"`
}

type dockerAuthEntry struct {
	Auth          string `json:"auth"`
	Username      string `json:"username"`
	Password      string `json:"password"`
	IdentityToken string `json:"identitytoken"`
	RegistryToken string `json:"registrytoken"`
}

func (r *DockerContainerRuntime) registryAuthForImage(imageRef string) (string, error) {
	raw := strings.TrimSpace(r.opts.RegistryAuthConfigJSON)
	if raw == "" {
		return "", nil
	}

	var cfg dockerAuthConfig
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		return "", fmt.Errorf("parse registry auth config: %w", err)
	}
	if len(cfg.Auths) == 0 {
		return "", nil
	}

	registryHost := imageRegistryHost(imageRef)
	entry, ok := matchAuthEntry(cfg.Auths, registryHost)
	if !ok {
		return "", nil
	}

	authCfg, ok := toDockerAuthConfig(entry, registryHost)
	if !ok {
		return "", nil
	}
	encoded, err := encodeRegistryAuth(authCfg)
	if err != nil {
		return "", fmt.Errorf("encode registry auth: %w", err)
	}
	return encoded, nil
}

func toDockerAuthConfig(entry dockerAuthEntry, serverAddress string) (registry.AuthConfig, bool) {
	cfg := registry.AuthConfig{
		ServerAddress: serverAddress,
		IdentityToken: strings.TrimSpace(entry.IdentityToken),
		RegistryToken: strings.TrimSpace(entry.RegistryToken),
	}

	if user := strings.TrimSpace(entry.Username); user != "" {
		cfg.Username = user
	}
	if pass := strings.TrimSpace(entry.Password); pass != "" {
		cfg.Password = pass
	}

	if cfg.Username == "" && cfg.Password == "" {
		auth := strings.TrimSpace(entry.Auth)
		if auth != "" {
			if decoded, err := base64.StdEncoding.DecodeString(auth); err == nil {
				parts := strings.SplitN(string(decoded), ":", 2)
				if len(parts) == 2 {
					cfg.Username = parts[0]
					cfg.Password = parts[1]
				}
			}
			if cfg.Username == "" && cfg.Password == "" {
				cfg.Auth = auth
			}
		}
	}

	if cfg.Username == "" && cfg.Password == "" && cfg.IdentityToken == "" && cfg.RegistryToken == "" && cfg.Auth == "" {
		return registry.AuthConfig{}, false
	}
	return cfg, true
}

func encodeRegistryAuth(cfg registry.AuthConfig) (string, error) {
	data, err := json.Marshal(cfg)
	if err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(data), nil
}

func imageRegistryHost(imageRef string) string {
	ref := strings.TrimSpace(imageRef)
	if ref == "" {
		return "docker.io"
	}

	slash := strings.IndexByte(ref, '/')
	if slash < 0 {
		// No registry/repository separator means Docker Hub namespace.
		return "docker.io"
	}

	first := ref[:slash]
	if strings.Contains(first, ".") || strings.Contains(first, ":") || first == "localhost" {
		return strings.ToLower(first)
	}
	return "docker.io"
}

func matchAuthEntry(auths map[string]dockerAuthEntry, registryHost string) (dockerAuthEntry, bool) {
	if len(auths) == 0 {
		return dockerAuthEntry{}, false
	}

	normalized := make(map[string]dockerAuthEntry, len(auths))
	for key, entry := range auths {
		norm := normalizeAuthRegistryKey(key)
		if norm == "" {
			continue
		}
		normalized[norm] = entry
	}

	if entry, ok := normalized[registryHost]; ok {
		return entry, true
	}

	if isDockerHubHost(registryHost) {
		for _, alias := range []string{"docker.io", "index.docker.io", "registry-1.docker.io"} {
			if entry, ok := normalized[alias]; ok {
				return entry, true
			}
		}
	}

	return dockerAuthEntry{}, false
}

func normalizeAuthRegistryKey(key string) string {
	k := strings.TrimSpace(key)
	if k == "" {
		return ""
	}

	if strings.Contains(k, "://") {
		u, err := url.Parse(k)
		if err == nil {
			host := strings.ToLower(strings.TrimSpace(u.Host))
			if host != "" {
				return host
			}
		}
	}

	k = strings.TrimPrefix(k, "//")
	k = strings.TrimSpace(k)
	k = strings.TrimRight(k, "/")
	if slash := strings.IndexByte(k, '/'); slash >= 0 {
		k = k[:slash]
	}
	return strings.ToLower(strings.TrimSpace(k))
}

func isDockerHubHost(host string) bool {
	switch strings.ToLower(strings.TrimSpace(host)) {
	case "docker.io", "index.docker.io", "registry-1.docker.io":
		return true
	default:
		return false
	}
}
