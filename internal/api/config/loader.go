package config

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Load reads the configuration from the provided path.
func Load(path string) (Config, error) {
	f, err := os.Open(path)
	if err != nil {
		return Config{}, fmt.Errorf("config: open %s: %w", path, err)
	}
	defer func() { _ = f.Close() }()
	cfg, err := loadFromReader(f)
	if err != nil {
		return Config{}, err
	}
	cfg.FilePath = path
	applyDefaults(&cfg)
	if err := validate(&cfg); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

// loadFromReader unmarshals configuration from the reader.
func loadFromReader(r io.Reader) (Config, error) {
	var buf bytes.Buffer
	if _, err := io.Copy(&buf, r); err != nil {
		return Config{}, fmt.Errorf("config: read: %w", err)
	}
	if buf.Len() == 0 {
		cfg := defaultConfig()
		return cfg, nil
	}
	dec := yaml.NewDecoder(&buf)
	dec.KnownFields(true)
	var cfg Config
	if err := dec.Decode(&cfg); err != nil {
		return Config{}, fmt.Errorf("config: decode: %w", err)
	}
	return cfg, nil
}

// ResolveRelative resolves the provided path relative to the configuration location when the path is relative.
func (cfg Config) ResolveRelative(p string) string {
	if strings.TrimSpace(p) == "" {
		return ""
	}
	if filepath.IsAbs(p) || strings.HasPrefix(p, "${") {
		return p
	}
	base := filepath.Dir(cfg.FilePath)
	if base == "" || base == "." {
		return p
	}
	return filepath.Join(base, p)
}
