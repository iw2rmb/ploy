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

// Descriptor represents a local cluster descriptor written by bootstrap.
type Descriptor struct {
    ClusterID       string `json:"cluster_id"`
    Address         string `json:"address"`
    Scheme          string `json:"scheme,omitempty"`
    SSHIdentityPath string `json:"ssh_identity_path,omitempty"`
    Default         bool   `json:"default,omitempty"`
}

// SaveDescriptor persists the descriptor under the clusters directory.
func SaveDescriptor(desc Descriptor) (Descriptor, error) {
    dir, err := clustersDir()
    if err != nil {
        return Descriptor{}, err
    }
    if strings.TrimSpace(desc.ClusterID) == "" {
        return Descriptor{}, errors.New("descriptor: cluster id required")
    }
    if err := os.MkdirAll(dir, 0o755); err != nil {
        return Descriptor{}, fmt.Errorf("descriptor: ensure dir: %w", err)
    }
    path := filepath.Join(dir, sanitizeFilename(desc.ClusterID)+".json")
    data, _ := json.MarshalIndent(desc, "", "  ")
    if err := os.WriteFile(path, data, 0o644); err != nil {
        return Descriptor{}, fmt.Errorf("descriptor: write %s: %w", path, err)
    }
    return desc, nil
}

// SetDefault records the default cluster id.
func SetDefault(clusterID string) error {
    dir, err := clustersDir()
    if err != nil {
        return err
    }
    if err := os.MkdirAll(dir, 0o755); err != nil {
        return fmt.Errorf("descriptor: ensure dir: %w", err)
    }
    marker := filepath.Join(dir, "default")
    return os.WriteFile(marker, []byte(strings.TrimSpace(clusterID)), 0o644)
}

// ListDescriptors returns descriptors with Default marked when matching the marker file.
func ListDescriptors() ([]Descriptor, error) {
    dir, err := clustersDir()
    if err != nil {
        return nil, err
    }
    marker := filepath.Join(dir, "default")
    var def string
    if data, err := os.ReadFile(marker); err == nil {
        def = strings.TrimSpace(string(data))
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
        if strings.TrimSpace(d.ClusterID) == strings.TrimSpace(def) {
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

