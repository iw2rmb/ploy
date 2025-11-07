package config

// Lightweight CLI descriptor store for local clusters.
// This replaces the legacy config package and is intentionally minimal —
// sufficient for deploy/bootstrap tests to persist and list descriptors.

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ClusterID identifies a Ploy cluster.
//
// It is a distinct type to harden APIs that accept or return cluster
// identifiers while remaining JSON-compatible with a plain string.
type ClusterID string

// Descriptor represents a local cluster descriptor written by bootstrap.
type Descriptor struct {
	ClusterID       ClusterID `json:"cluster_id"`
	Address         string    `json:"address"`
	Scheme          string    `json:"scheme,omitempty"`
	SSHIdentityPath string    `json:"ssh_identity_path,omitempty"`
	CAPath          string    `json:"ca_path,omitempty"`
	CertPath        string    `json:"cert_path,omitempty"`
	KeyPath         string    `json:"key_path,omitempty"`
	Default         bool      `json:"default,omitempty"`
}

// SaveDescriptor persists the descriptor under the clusters directory.
func SaveDescriptor(desc Descriptor) (Descriptor, error) {
	dir, err := clustersDir()
	if err != nil {
		return Descriptor{}, err
	}
	if strings.TrimSpace(string(desc.ClusterID)) == "" {
		return Descriptor{}, errors.New("descriptor: cluster id required")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return Descriptor{}, fmt.Errorf("descriptor: ensure dir: %w", err)
	}
	path := filepath.Join(dir, sanitizeFilename(string(desc.ClusterID))+".json")
	data, _ := json.MarshalIndent(desc, "", "  ")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return Descriptor{}, fmt.Errorf("descriptor: write %s: %w", path, err)
	}
	return desc, nil
}

// SetDefault records the default cluster id.
func SetDefault(clusterID ClusterID) error {
	// Optional guard to avoid mutating the real default in shared environments
	// (e.g., during higher-level test suites). When set to a non-empty value,
	// this function becomes a no-op.
	if os.Getenv("PLOY_NO_DEFAULT_MUTATION") != "" {
		return nil
	}
	dir, err := clustersDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("descriptor: ensure dir: %w", err)
	}
	marker := filepath.Join(dir, "default")
	// Remove existing marker (file or symlink)
	if fi, err := os.Lstat(marker); err == nil {
		// If it's a symlink or file, remove it; ignore directories.
		if fi.Mode()&os.ModeSymlink != 0 || fi.Mode().IsRegular() {
			_ = os.Remove(marker)
		}
	}
	// Prefer symlink only when target json exists; otherwise, write legacy marker content.
	target := sanitizeFilename(strings.TrimSpace(string(clusterID))) + ".json"
	if _, err := os.Stat(filepath.Join(dir, target)); err == nil {
		if err := os.Symlink(target, marker); err == nil {
			return nil
		}
		// If symlink creation failed, fall through to legacy marker content.
	}
	return os.WriteFile(marker, []byte(strings.TrimSpace(string(clusterID))), 0o644)
}

// LoadDefault loads the default cluster descriptor.
func LoadDefault() (Descriptor, error) {
	dir, err := clustersDir()
	if err != nil {
		return Descriptor{}, err
	}
	marker := filepath.Join(dir, "default")
	// If marker is a symlink, resolve it to the descriptor path.
	fi, err := os.Lstat(marker)
	if err != nil {
		return Descriptor{}, fmt.Errorf("descriptor: read default marker: %w", err)
	}
	var path string
	if fi.Mode()&os.ModeSymlink != 0 {
		if target, err := os.Readlink(marker); err == nil {
			if filepath.IsAbs(target) {
				path = target
			} else {
				path = filepath.Join(dir, target)
			}
		} else {
			return Descriptor{}, fmt.Errorf("descriptor: readlink default: %w", err)
		}
	} else {
		// Legacy format: file contains cluster ID string
		data, err := os.ReadFile(marker)
		if err != nil {
			return Descriptor{}, fmt.Errorf("descriptor: read default marker: %w", err)
		}
		clusterID := strings.TrimSpace(string(data))
		if clusterID == "" {
			return Descriptor{}, errors.New("descriptor: default marker is empty")
		}
		path = filepath.Join(dir, sanitizeFilename(clusterID)+".json")
	}
	descData, err := os.ReadFile(path)
	if err != nil {
		return Descriptor{}, fmt.Errorf("descriptor: read %s: %w", path, err)
	}
	var d Descriptor
	if err := json.Unmarshal(descData, &d); err != nil {
		return Descriptor{}, fmt.Errorf("descriptor: parse %s: %w", path, err)
	}
	d.Default = true
	return d, nil
}

// ListDescriptors returns descriptors with Default marked when matching the marker file.
func ListDescriptors() ([]Descriptor, error) {
	dir, err := clustersDir()
	if err != nil {
		return nil, err
	}
	marker := filepath.Join(dir, "default")
	var def string
	if fi, err := os.Lstat(marker); err == nil {
		if fi.Mode()&os.ModeSymlink != 0 {
			if target, err := os.Readlink(marker); err == nil {
				def = strings.TrimSuffix(filepath.Base(target), ".json")
			}
		} else if data, err := os.ReadFile(marker); err == nil {
			def = strings.TrimSpace(string(data))
		}
	}
	entries, err := os.ReadDir(dir)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("descriptor: read dir: %w", err)
	}
	out := make([]Descriptor, 0, len(entries))
	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return nil, fmt.Errorf("descriptor: read %s: %w", name, err)
		}
		var d Descriptor
		if err := json.Unmarshal(data, &d); err != nil {
			return nil, fmt.Errorf("descriptor: parse %s: %w", name, err)
		}
		if strings.TrimSpace(string(d.ClusterID)) == strings.TrimSpace(def) {
			d.Default = true
		}
		out = append(out, d)
	}
	return out, nil
}

func clustersDir() (string, error) {
	base := strings.TrimSpace(os.Getenv("PLOY_CONFIG_HOME"))
	if base == "" {
		xdg := strings.TrimSpace(os.Getenv("XDG_CONFIG_HOME"))
		if xdg != "" {
			base = filepath.Join(xdg, "ploy")
		} else {
			home, err := os.UserHomeDir()
			if err != nil {
				return "", fmt.Errorf("descriptor: find home: %w", err)
			}
			base = filepath.Join(home, ".config", "ploy")
		}
	}
	return filepath.Join(base, "clusters"), nil
}

func sanitizeFilename(s string) string {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, "/", "_")
	s = strings.ReplaceAll(s, "\\", "_")
	return s
}
