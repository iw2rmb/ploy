package step

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"strings"
)

func dockerJobRequestHandler(mounts []ContainerMount, owner DockerJobOwner, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.Method == http.MethodPost && dockerCreateResource(req.URL.Path) != "" {
			body, err := rewriteDockerCreate(req.Body, mounts, owner)
			_ = req.Body.Close()
			if err != nil {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusBadRequest)
				_ = json.NewEncoder(w).Encode(map[string]string{"message": "Ploy job Docker request: " + err.Error()})
				return
			}
			req.Body = io.NopCloser(bytes.NewReader(body))
			req.ContentLength = int64(len(body))
			req.TransferEncoding = nil
			req.Header.Del("Content-Length")
		}
		next.ServeHTTP(w, req)
	})
}

func dockerCreateResource(p string) string {
	p = strings.TrimPrefix(p, "/")
	if version, rest, ok := strings.Cut(p, "/"); ok && len(version) > 1 && version[0] == 'v' && version[1] >= '0' && version[1] <= '9' {
		major, minor, valid := strings.Cut(strings.TrimPrefix(version, "v"), ".")
		if !valid || !allDigits(major) || !allDigits(minor) {
			return ""
		}
		p = rest
	}
	switch p {
	case "containers/create":
		return "container"
	case "networks/create":
		return "network"
	case "volumes/create":
		return "volume"
	default:
		return ""
	}
}

func allDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, ch := range s {
		if ch < '0' || ch > '9' {
			return false
		}
	}
	return true
}

func rewriteDockerCreate(body io.Reader, mounts []ContainerMount, owner DockerJobOwner) ([]byte, error) {
	// Preserve Docker API extensions and integer precision while stamping ownership.
	decoder := json.NewDecoder(body)
	decoder.UseNumber()
	var config map[string]any
	if err := decoder.Decode(&config); err != nil {
		return nil, fmt.Errorf("decode container create request: %w", err)
	}
	if config == nil {
		return nil, fmt.Errorf("container create request must be an object")
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return nil, fmt.Errorf("container create request must contain one JSON object")
	}
	if raw := config["HostConfig"]; raw != nil {
		hostConfig, ok := raw.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("HostConfig must be an object")
		}
		if err := rewriteDockerHostBinds(hostConfig, mounts); err != nil {
			return nil, err
		}
	}
	labels := map[string]any{}
	// Docker decodes JSON field names without regard to case. Remove every
	// spelling so a later lowercase field cannot replace the injected identity.
	labelsSeen := false
	for key, raw := range config {
		if !strings.EqualFold(key, "Labels") {
			continue
		}
		if labelsSeen {
			return nil, fmt.Errorf("duplicate Labels fields")
		}
		labelsSeen = true
		if raw != nil {
			var ok bool
			labels, ok = raw.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("Labels must be an object")
			}
		}
		delete(config, key)
	}
	for key, value := range owner.labels() {
		labels[key] = value
	}
	config["Labels"] = labels
	return json.Marshal(config)
}

func rewriteDockerHostBinds(config map[string]any, mounts []ContainerMount) error {
	if raw := config["Binds"]; raw != nil {
		binds, ok := raw.([]any)
		if !ok {
			return fmt.Errorf("HostConfig.Binds must be an array")
		}
		for i, raw := range binds {
			bind, ok := raw.(string)
			if !ok {
				return fmt.Errorf("HostConfig.Binds entries must be strings")
			}
			source, suffix, ok := strings.Cut(bind, ":")
			if !ok {
				continue
			}
			translated, _, err := translateDockerBindSource(source, mounts)
			if err != nil {
				return err
			}
			binds[i] = translated + ":" + suffix
		}
	}
	if raw := config["Mounts"]; raw != nil {
		entries, ok := raw.([]any)
		if !ok {
			return fmt.Errorf("HostConfig.Mounts must be an array")
		}
		for _, raw := range entries {
			entry, ok := raw.(map[string]any)
			if !ok {
				return fmt.Errorf("HostConfig.Mounts entries must be objects")
			}
			if entry["Type"] != "bind" {
				continue
			}
			source, ok := entry["Source"].(string)
			if !ok {
				return fmt.Errorf("bind mount Source must be a string")
			}
			translated, mapped, err := translateDockerBindSource(source, mounts)
			if err != nil {
				return err
			}
			if !mapped {
				continue
			}
			entry["Source"] = translated
			options := map[string]any{}
			if raw := entry["BindOptions"]; raw != nil {
				var ok bool
				options, ok = raw.(map[string]any)
				if !ok {
					return fmt.Errorf("BindOptions must be an object")
				}
			}
			options["CreateMountpoint"] = false
			entry["BindOptions"] = options
		}
	}
	return nil
}

func translateDockerBindSource(source string, mounts []ContainerMount) (string, bool, error) {
	if !path.IsAbs(source) {
		return source, false, nil
	}
	clean := path.Clean(source)
	var best *ContainerMount
	for i := range mounts {
		m := &mounts[i]
		if clean == m.Target || strings.HasPrefix(clean, strings.TrimSuffix(m.Target, "/")+"/") {
			if best == nil || len(m.Target) > len(best.Target) {
				best = m
			}
		}
	}
	if best == nil {
		return source, false, nil
	}
	translated := path.Join(best.Source, strings.TrimPrefix(clean, best.Target))
	if _, err := os.Stat(translated); err != nil {
		return "", true, fmt.Errorf("bind source %q maps to unavailable host path %q: %w", source, translated, err)
	}
	return translated, true, nil
}
