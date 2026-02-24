package manifests

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

type compileOptions struct {
	Name    string
	Version string
}

type registry struct {
	entries map[string]rawEntry
}

type rawEntry struct {
	manifest rawManifest
	path     string
}

// loadDirectory ingests manifest definitions from the provided directory.
func loadDirectory(dir string) (*registry, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read manifest directory: %w", err)
	}

	registry := &registry{entries: make(map[string]rawEntry)}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if filepath.Ext(entry.Name()) != ".toml" {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read manifest %s: %w", entry.Name(), err)
		}

		var manifest rawManifest
		if err := toml.Unmarshal(data, &manifest); err != nil {
			return nil, fmt.Errorf("decode manifest %s: %w", entry.Name(), err)
		}

		if err := validateRawManifest(manifest); err != nil {
			return nil, fmt.Errorf("%w (%s): %v", errInvalidManifest, entry.Name(), err)
		}

		key := strings.TrimSpace(manifest.Name)
		if _, exists := registry.entries[key]; exists {
			return nil, fmt.Errorf("%w: duplicate manifest %q", errInvalidManifest, key)
		}
		registry.entries[key] = rawEntry{manifest: manifest, path: path}
	}

	if len(registry.entries) == 0 {
		return nil, fmt.Errorf("%w: no manifest definitions found in %s", errRegistryUnavailable, dir)
	}

	return registry, nil
}

// compileManifest materialises a normalized compilation for the requested manifest.
func (r *registry) compileManifest(opts compileOptions) (Compilation, error) {
	if r == nil {
		return Compilation{}, fmt.Errorf("%w: registry missing", errRegistryUnavailable)
	}
	key := strings.TrimSpace(opts.Name)
	if key == "" {
		return Compilation{}, fmt.Errorf("%w: name is required", errManifestNotFound)
	}

	entry, ok := r.entries[key]
	if !ok {
		return Compilation{}, fmt.Errorf("%w: %s", errManifestNotFound, key)
	}

	requestedVersion := strings.TrimSpace(opts.Version)
	actualVersion := strings.TrimSpace(entry.manifest.Version)
	if requestedVersion != "" && requestedVersion != actualVersion {
		return Compilation{}, fmt.Errorf("version mismatch: requested %s, manifest %s", requestedVersion, actualVersion)
	}

	return compile(entry.manifest), nil
}
