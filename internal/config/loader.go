package config

import (
    "os"
    "sort"

    "github.com/mitchellh/mapstructure"
    yamlv3 "gopkg.in/yaml.v3"
)

// Source represents a configuration source.
type Source interface {
    Name() string
    Load() (map[string]interface{}, error)
    Priority() int // Higher number overrides lower
}

// CompositeLoader merges multiple sources by ascending priority, last-wins.
type CompositeLoader struct {
    sources []Source
}

func (l *CompositeLoader) AddSource(s Source) {
    l.sources = append(l.sources, s)
}

func (l *CompositeLoader) Load() (*Config, error) {
    // Sort: lower priority first, so higher overrides later.
    sort.Slice(l.sources, func(i, j int) bool { return l.sources[i].Priority() < l.sources[j].Priority() })

    merged := map[string]interface{}{}
    for _, s := range l.sources {
        data, err := s.Load()
        if err != nil {
            return nil, err
        }
        mergeMaps(merged, data)
    }

    var cfg Config
    dec, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
        Result:           &cfg,
        WeaklyTypedInput: true,
        TagName:          "yaml",
    })
    if err != nil {
        return nil, err
    }
    if err := dec.Decode(merged); err != nil {
        return nil, err
    }
    return &cfg, nil
}

// defaultsSource provides static defaults.
type defaultsSource struct {
    defaults *Config
    priority int
}

func (d *defaultsSource) Name() string     { return "defaults" }
func (d *defaultsSource) Priority() int    { return d.priority }
func (d *defaultsSource) Load() (map[string]interface{}, error) {
    if d.defaults == nil {
        return map[string]interface{}{}, nil
    }
    return map[string]interface{}{
        "app": map[string]interface{}{
            "name":    d.defaults.App.Name,
            "version": d.defaults.App.Version,
        },
    }, nil
}

// envSource maps environment variables with a prefix to config fields.
// Minimal support: PREFIX_APP_NAME -> app.name
type envSource struct {
    prefix   string
    priority int
}

func (e *envSource) Name() string  { return "env" }
func (e *envSource) Priority() int { return e.priority }
func (e *envSource) Load() (map[string]interface{}, error) {
    out := map[string]interface{}{}
    if v := os.Getenv(e.prefix + "APP_NAME"); v != "" {
        ensurePath(out, "app")["name"] = v
    }
    if v := os.Getenv(e.prefix + "APP_VERSION"); v != "" {
        ensurePath(out, "app")["version"] = v
    }
    // Storage overrides
    if v := os.Getenv(e.prefix + "STORAGE_PROVIDER"); v != "" {
        ensurePath(out, "storage")["provider"] = v
    }
    if v := os.Getenv(e.prefix + "STORAGE_ENDPOINT"); v != "" {
        ensurePath(out, "storage")["endpoint"] = v
    }
    return out, nil
}

// fileSource loads configuration from a YAML file into a generic map.
type fileSource struct {
    path     string
    priority int
}

func (f *fileSource) Name() string  { return "file" }
func (f *fileSource) Priority() int { return f.priority }
func (f *fileSource) Load() (map[string]interface{}, error) {
    b, err := os.ReadFile(f.path)
    if err != nil {
        return nil, err
    }
    var out map[string]interface{}
    if err := yamlv3.Unmarshal(b, &out); err != nil {
        return nil, err
    }
    if out == nil {
        out = map[string]interface{}{}
    }
    return out, nil
}

// mergeMaps recursively merges src into dst (last write wins).
func mergeMaps(dst, src map[string]interface{}) {
    for k, v := range src {
        if vmap, ok := v.(map[string]interface{}); ok {
            if _, exists := dst[k]; !exists {
                dst[k] = map[string]interface{}{}
            }
            if dmap, ok := dst[k].(map[string]interface{}); ok {
                mergeMaps(dmap, vmap)
            } else {
                dst[k] = v
            }
        } else {
            dst[k] = v
        }
    }
}

// ensurePath ensures dst[path] exists as a map and returns it.
func ensurePath(dst map[string]interface{}, path string) map[string]interface{} {
    if m, ok := dst[path].(map[string]interface{}); ok {
        return m
    }
    m := map[string]interface{}{}
    dst[path] = m
    return m
}
