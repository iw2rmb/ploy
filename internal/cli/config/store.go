package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Descriptor captures the SSH metadata required to establish tunnels to a cluster.
type Descriptor struct {
	ClusterID       string            `json:"cluster_id"`
	Address         string            `json:"address"`
	SSHIdentityPath string            `json:"ssh_identity_path,omitempty"`
	Labels          map[string]string `json:"labels,omitempty"`
	Scheme          string            `json:"scheme,omitempty"`
	CABundle        string            `json:"ca_bundle,omitempty"`

	Default bool `json:"-"`
}

type descriptorFile struct {
	ClusterID       string            `json:"cluster_id"`
	Address         string            `json:"address"`
	SSHIdentityPath string            `json:"ssh_identity_path,omitempty"`
	Labels          map[string]string `json:"labels,omitempty"`
	Scheme          string            `json:"scheme,omitempty"`
	CABundle        string            `json:"ca_bundle,omitempty"`
}

type legacyDescriptorFile struct {
	ClusterID       string            `json:"cluster_id"`
	ID              string            `json:"id"`
	Address         string            `json:"address"`
	NodeAddress     string            `json:"node_address"`
	SSHIdentityPath string            `json:"ssh_identity_path"`
	IdentityPath    string            `json:"identity_path"`
	Labels          map[string]string `json:"labels"`
	Default         bool              `json:"default"`
}

type descriptorLoad struct {
	descriptor    Descriptor
	needsWrite    bool
	legacyDefault bool
}

// SaveDescriptor persists a cluster descriptor to disk, returning the stored copy.
func SaveDescriptor(desc Descriptor) (Descriptor, error) {
	trimmedID := strings.TrimSpace(desc.ClusterID)
	if trimmedID == "" {
		return Descriptor{}, errors.New("descriptor cluster id required")
	}
	trimmedAddress := strings.TrimSpace(desc.Address)
	if trimmedAddress == "" {
		return Descriptor{}, errors.New("descriptor address required")
	}

	sanitizedID := sanitizeID(trimmedID)
	desc.ClusterID = sanitizedID
	desc.Address = trimmedAddress
	desc.SSHIdentityPath = strings.TrimSpace(desc.SSHIdentityPath)
	desc.Labels = cloneLabels(desc.Labels)
	desc.Scheme = strings.TrimSpace(desc.Scheme)
	desc.CABundle = strings.TrimSpace(desc.CABundle)

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
	defaultID, err := readDefaultMarker(root)
	if err != nil {
		return Descriptor{}, err
	}
	desc.Default = defaultID != "" && defaultID == desc.ClusterID
	return desc, nil
}

// LoadDescriptor reads a descriptor from disk by identifier.
func LoadDescriptor(id string) (Descriptor, error) {
	path, err := descriptorPath(id)
	if err != nil {
		return Descriptor{}, err
	}
	load, err := loadDescriptorFromPath(path)
	if err != nil {
		return Descriptor{}, err
	}
	if load.needsWrite {
		if err := writeDescriptorFile(path, load.descriptor); err != nil {
			return Descriptor{}, err
		}
	}
	root := filepath.Dir(path)
	defaultID, err := readDefaultMarker(root)
	if err != nil {
		return Descriptor{}, err
	}
	if defaultID == "" && load.legacyDefault {
		if err := writeDefaultMarker(root, load.descriptor.ClusterID); err != nil {
			return Descriptor{}, err
		}
		defaultID = load.descriptor.ClusterID
	}
	load.descriptor.Default = defaultID != "" && load.descriptor.ClusterID == defaultID
	return load.descriptor, nil
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
	defaultID, err := readDefaultMarker(root)
	if err != nil {
		return nil, err
	}
	hasMarker := defaultID != ""
	descs := make([]Descriptor, 0, len(entries))

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		path := filepath.Join(root, entry.Name())
		load, err := loadDescriptorFromPath(path)
		if err != nil {
			return nil, err
		}
		if load.needsWrite {
			if err := writeDescriptorFile(path, load.descriptor); err != nil {
				return nil, err
			}
		}
		if !hasMarker && load.legacyDefault {
			if err := writeDefaultMarker(root, load.descriptor.ClusterID); err != nil {
				return nil, err
			}
			defaultID = load.descriptor.ClusterID
			hasMarker = true
		}
		load.descriptor.Default = defaultID != "" && load.descriptor.ClusterID == defaultID
		descs = append(descs, load.descriptor)
	}

	sort.Slice(descs, func(i, j int) bool {
		return descs[i].ClusterID < descs[j].ClusterID
	})

	if defaultID == "" && len(descs) == 1 {
		descs[0].Default = true
	}
	return descs, nil
}

// SetDefault marks the supplied cluster identifier as the default descriptor.
func SetDefault(id string) error {
	trimmed := strings.TrimSpace(id)
	if trimmed == "" {
		return errors.New("cluster id required")
	}
	if _, err := LoadDescriptor(trimmed); err != nil {
		return err
	}
	root, err := clusterConfigDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return fmt.Errorf("create config directory: %w", err)
	}
	return writeDefaultMarker(root, sanitizeID(trimmed))
}

func clusterConfigDir() (string, error) {
    // Canonical location by default: ~/.config/ploy/clusters
    // Allow test harness to override via PLOY_CONFIG_HOME or XDG_CONFIG_HOME.
    if override := strings.TrimSpace(os.Getenv("PLOY_CONFIG_HOME")); override != "" {
        return filepath.Join(override, "clusters"), nil
    }
    if base := strings.TrimSpace(os.Getenv("XDG_CONFIG_HOME")); base != "" {
        return filepath.Join(base, "ploy", "clusters"), nil
    }
    home, err := os.UserHomeDir()
    if err != nil {
        // Fallback to OS config dir if HOME is not resolvable
        configDir, derr := os.UserConfigDir()
        if derr != nil {
            return "", fmt.Errorf("resolve config dir: %w", err)
        }
        return filepath.Join(configDir, "ploy", "clusters"), nil
    }
    return filepath.Join(home, ".config", "ploy", "clusters"), nil
}

func descriptorPath(id string) (string, error) {
	root, err := clusterConfigDir()
	if err != nil {
		return "", err
	}
	slug := sanitizeID(strings.TrimSpace(id))
	if slug == "" {
		return "", errors.New("cluster id required")
	}
	return filepath.Join(root, slug+".json"), nil
}

func writeDescriptorFile(path string, desc Descriptor) error {
	payload := descriptorFile{
		ClusterID:       desc.ClusterID,
		Address:         desc.Address,
		SSHIdentityPath: desc.SSHIdentityPath,
		Labels:          nil,
		Scheme:          desc.Scheme,
		CABundle:        desc.CABundle,
	}
	if len(desc.Labels) > 0 {
		payload.Labels = cloneLabels(desc.Labels)
	}
	data, err := json.MarshalIndent(payload, "", "  ")
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

func loadDescriptorFromPath(path string) (descriptorLoad, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return descriptorLoad{}, fmt.Errorf("cluster %s not found", strings.TrimSuffix(filepath.Base(path), ".json"))
		}
		return descriptorLoad{}, fmt.Errorf("read descriptor: %w", err)
	}

	var current descriptorFile
	if err := json.Unmarshal(data, &current); err == nil && strings.TrimSpace(current.ClusterID) != "" {
		desc := Descriptor{
			ClusterID:       sanitizeID(current.ClusterID),
			Address:         strings.TrimSpace(current.Address),
			SSHIdentityPath: strings.TrimSpace(current.SSHIdentityPath),
			Labels:          cloneLabels(current.Labels),
			Scheme:          strings.TrimSpace(current.Scheme),
			CABundle:        strings.TrimSpace(current.CABundle),
		}
		return descriptorLoad{descriptor: desc}, nil
	}

	var legacy legacyDescriptorFile
	if err := json.Unmarshal(data, &legacy); err != nil {
		return descriptorLoad{}, fmt.Errorf("decode descriptor: %w", err)
	}
	clusterID := legacy.ClusterID
	if clusterID == "" {
		clusterID = legacy.ID
	}
	address := legacy.Address
	if address == "" {
		address = legacy.NodeAddress
	}
	identity := legacy.SSHIdentityPath
	if identity == "" {
		identity = legacy.IdentityPath
	}
	desc := Descriptor{
		ClusterID:       sanitizeID(clusterID),
		Address:         strings.TrimSpace(address),
		SSHIdentityPath: strings.TrimSpace(identity),
		Labels:          cloneLabels(legacy.Labels),
		Scheme:          "",
		CABundle:        "",
	}
	if desc.ClusterID == "" || desc.Address == "" {
		return descriptorLoad{}, errors.New("invalid legacy descriptor")
	}
	return descriptorLoad{
		descriptor:    desc,
		needsWrite:    true,
		legacyDefault: legacy.Default,
	}, nil
}

func cloneLabels(labels map[string]string) map[string]string {
	if len(labels) == 0 {
		return nil
	}
	copied := maps.Clone(labels)
	for k, v := range copied {
		trimmedKey := strings.TrimSpace(k)
		trimmedVal := strings.TrimSpace(v)
		if trimmedKey == "" || trimmedVal == "" {
			delete(copied, k)
			continue
		}
		if trimmedKey != k || trimmedVal != v {
			delete(copied, k)
			copied[trimmedKey] = trimmedVal
		}
	}
	if len(copied) == 0 {
		return nil
	}
	return copied
}

// SanitizeID normalizes a cluster identifier for persistent storage.
func SanitizeID(value string) string {
	return sanitizeID(value)
}

func defaultMarkerPath(root string) string {
	return filepath.Join(root, "default")
}

func readDefaultMarker(root string) (string, error) {
	data, err := os.ReadFile(defaultMarkerPath(root))
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return "", nil
		}
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}

func writeDefaultMarker(root, id string) error {
	return os.WriteFile(defaultMarkerPath(root), []byte(strings.TrimSpace(id)+"\n"), 0o644)
}
