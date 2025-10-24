package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Descriptor captures the minimal metadata required to establish SSH tunnels to a cluster.
type Descriptor struct {
	ID            string    `json:"id"`
	NodeAddress   string    `json:"node_address"`
	IdentityPath  string    `json:"identity_path,omitempty"`
	LastRefreshed time.Time `json:"last_refreshed,omitempty"`
	Default       bool      `json:"default,omitempty"`
}

// SaveDescriptor persists a cluster descriptor to disk, returning the stored copy.
func SaveDescriptor(desc Descriptor) (Descriptor, error) {
	trimmedID := strings.TrimSpace(desc.ID)
	if trimmedID == "" {
		return Descriptor{}, errors.New("descriptor id required")
	}
	if strings.TrimSpace(desc.NodeAddress) == "" {
		return Descriptor{}, errors.New("descriptor node address required")
	}
	sanitizedID := sanitizeID(trimmedID)
	if desc.LastRefreshed.IsZero() {
		desc.LastRefreshed = time.Now().UTC()
	}
	desc.ID = sanitizedID
	root, err := clusterConfigDir()
	if err != nil {
		return Descriptor{}, err
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return Descriptor{}, fmt.Errorf("create config directory: %w", err)
	}
	path := filepath.Join(root, sanitizedID+".json")
	if err := writeDescriptorFile(path, desc); err != nil {
		return Descriptor{}, err
	}
	return desc, nil
}

// LoadDescriptor reads a descriptor from disk by identifier.
func LoadDescriptor(id string) (Descriptor, error) {
	path, err := descriptorPath(id)
	if err != nil {
		return Descriptor{}, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return Descriptor{}, fmt.Errorf("cluster %s not found", id)
		}
		return Descriptor{}, fmt.Errorf("read descriptor: %w", err)
	}
	var desc Descriptor
	if err := json.Unmarshal(data, &desc); err != nil {
		return Descriptor{}, fmt.Errorf("decode descriptor: %w", err)
	}
	return desc, nil
}

// ListDescriptors returns all cached descriptors sorted by identifier.
func ListDescriptors() ([]Descriptor, error) {
	root, err := clusterConfigDir()
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("read config directory: %w", err)
	}
	descs := make([]Descriptor, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		id := strings.TrimSuffix(entry.Name(), ".json")
		desc, err := LoadDescriptor(id)
		if err != nil {
			return nil, err
		}
		desc.ID = idFromFilename(entry.Name())
		descs = append(descs, desc)
	}
	sort.Slice(descs, func(i, j int) bool {
		return descs[i].ID < descs[j].ID
	})
	return descs, nil
}

// SetDefault marks the supplied cluster identifier as the default descriptor.
func SetDefault(id string) error {
	trimmed := strings.TrimSpace(id)
	if trimmed == "" {
		return errors.New("cluster id required")
	}
	descriptors, err := ListDescriptors()
	if err != nil {
		return err
	}
	found := false
	for _, desc := range descriptors {
		desc.Default = desc.ID == trimmed
		if desc.Default {
			found = true
		}
		path, err := descriptorPath(desc.ID)
		if err != nil {
			return err
		}
		if err := writeDescriptorFile(path, desc); err != nil {
			return err
		}
	}
	if !found {
		return fmt.Errorf("cluster %s not found", trimmed)
	}
	return nil
}

func clusterConfigDir() (string, error) {
	if override := strings.TrimSpace(os.Getenv("PLOY_CONFIG_HOME")); override != "" {
		return filepath.Join(override, "clusters"), nil
	}
	if base := strings.TrimSpace(os.Getenv("XDG_CONFIG_HOME")); base != "" {
		return filepath.Join(base, "ploy", "clusters"), nil
	}
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("resolve config dir: %w", err)
	}
	return filepath.Join(configDir, "ploy", "clusters"), nil
}

func descriptorPath(id string) (string, error) {
	root, err := clusterConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, sanitizeID(strings.TrimSpace(id))+".json"), nil
}

func writeDescriptorFile(path string, desc Descriptor) error {
	data, err := json.MarshalIndent(desc, "", "  ")
	if err != nil {
		return fmt.Errorf("encode descriptor: %w", err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("write descriptor temp file: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("activate descriptor: %w", err)
	}
	return nil
}

func sanitizeID(id string) string {
	id = strings.ToLower(strings.TrimSpace(id))
	replacer := strings.NewReplacer("/", "-", "\\", "-", "..", "-", " ", "-")
	id = replacer.Replace(id)
	clean := make([]rune, 0, len(id))
	for _, r := range id {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			clean = append(clean, r)
		}
	}
	if len(clean) == 0 {
		return "cluster"
	}
	return string(clean)
}

func idFromFilename(name string) string {
	trimmed := strings.TrimSuffix(name, ".json")
	return trimmed
}
