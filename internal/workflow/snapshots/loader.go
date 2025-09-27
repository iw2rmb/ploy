package snapshots

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

// LoadDirectory builds a Registry from the snapshot specs found in the provided directory.
func LoadDirectory(dir string, opts LoadOptions) (*Registry, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read snapshot directory: %w", err)
	}

	specs := make(map[string]Spec)

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
			return nil, fmt.Errorf("read snapshot file %s: %w", entry.Name(), err)
		}
		var spec Spec
		if err := toml.Unmarshal(data, &spec); err != nil {
			return nil, fmt.Errorf("decode snapshot %s: %w", entry.Name(), err)
		}
		if err := validateSpec(spec); err != nil {
			return nil, fmt.Errorf("%w: %v", ErrInvalidSpec, err)
		}
		if !filepath.IsAbs(spec.Source.Fixture) {
			spec.Source.Fixture = filepath.Join(dir, spec.Source.Fixture)
		}
		if _, exists := specs[spec.Name]; exists {
			return nil, fmt.Errorf("%w: duplicate snapshot %q", ErrInvalidSpec, spec.Name)
		}
		specs[spec.Name] = spec
	}

	if opts.ArtifactPublisher == nil {
		opts.ArtifactPublisher = NewInMemoryArtifactPublisher()
	}
	if opts.MetadataPublisher == nil {
		opts.MetadataPublisher = NewNoopMetadataPublisher()
	}

	return &Registry{
		specs:    specs,
		artifact: opts.ArtifactPublisher,
		metadata: opts.MetadataPublisher,
	}, nil
}

// validateSpec ensures a spec includes the minimum required snapshot metadata.
func validateSpec(spec Spec) error {
	if strings.TrimSpace(spec.Name) == "" {
		return errors.New("name is required")
	}
	if strings.TrimSpace(spec.Source.Engine) == "" {
		return errors.New("source.engine is required")
	}
	if strings.TrimSpace(spec.Source.Fixture) == "" {
		return errors.New("source.fixture is required")
	}
	return nil
}
