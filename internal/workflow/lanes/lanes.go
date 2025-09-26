package lanes

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

var (
	errLaneNotFound      = errors.New("lane not found")
	errInvalidLaneConfig = errors.New("invalid lane configuration")
)

type Commands struct {
	Setup []string `toml:"setup"`
	Build []string `toml:"build"`
	Test  []string `toml:"test"`
}

type Spec struct {
	Name           string   `toml:"name"`
	Description    string   `toml:"description"`
	RuntimeFamily  string   `toml:"runtime_family"`
	CacheNamespace string   `toml:"cache_namespace"`
	Commands       Commands `toml:"commands"`
}

type Registry struct {
	lanes map[string]Spec
}

type DescribeOptions struct {
	CommitSHA           string
	SnapshotFingerprint string
	ManifestVersion     string
	AsterToggles        []string
}

type Description struct {
	Lane       Spec
	CacheKey   string
	Parameters DescribeOptions
}

type CacheKeyRequest struct {
	Lane Spec
	DescribeOptions
}

// LoadDirectory reads lane specifications from TOML files in dir.
// It requires each spec to define name, runtime family, cache namespace,
// and build/test commands. Duplicate names are rejected.
func LoadDirectory(dir string) (*Registry, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read lane directory: %w", err)
	}

	lanesMap := make(map[string]Spec)

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
			return nil, fmt.Errorf("read lane file %s: %w", entry.Name(), err)
		}

		var spec Spec
		if err := toml.Unmarshal(data, &spec); err != nil {
			return nil, fmt.Errorf("decode lane %s: %w", entry.Name(), err)
		}

		if err := validateSpec(spec); err != nil {
			return nil, fmt.Errorf("%w: %v", errInvalidLaneConfig, err)
		}

		if _, exists := lanesMap[spec.Name]; exists {
			return nil, fmt.Errorf("%w: duplicate lane %q", errInvalidLaneConfig, spec.Name)
		}
		lanesMap[spec.Name] = spec
	}

	return &Registry{lanes: lanesMap}, nil
}

func validateSpec(spec Spec) error {
	if strings.TrimSpace(spec.Name) == "" {
		return errors.New("name is required")
	}
	if strings.TrimSpace(spec.RuntimeFamily) == "" {
		return errors.New("runtime_family is required")
	}
	if strings.TrimSpace(spec.CacheNamespace) == "" {
		return errors.New("cache_namespace is required")
	}
	if len(spec.Commands.Build) == 0 {
		return errors.New("commands.build is required")
	}
	if len(spec.Commands.Test) == 0 {
		return errors.New("commands.test is required")
	}
	return nil
}

func (r *Registry) Describe(lane string, opts DescribeOptions) (Description, error) {
	if r == nil {
		return Description{}, fmt.Errorf("%w: registry is nil", errLaneNotFound)
	}
	trimmed := strings.TrimSpace(lane)
	if trimmed == "" {
		return Description{}, fmt.Errorf("%w: name is empty", errLaneNotFound)
	}
	laneSpec, ok := r.lanes[trimmed]
	if !ok {
		return Description{}, fmt.Errorf("%w: %s", errLaneNotFound, trimmed)
	}

	cacheKey, err := ComposeCacheKey(CacheKeyRequest{
		Lane:            laneSpec,
		DescribeOptions: opts,
	})
	if err != nil {
		return Description{}, err
	}

	return Description{
		Lane:       laneSpec,
		CacheKey:   cacheKey,
		Parameters: opts,
	}, nil
}

func ComposeCacheKey(req CacheKeyRequest) (string, error) {
	if strings.TrimSpace(req.Lane.Name) == "" {
		return "", errors.New("lane name is required")
	}
	if strings.TrimSpace(req.Lane.CacheNamespace) == "" {
		return "", errors.New("lane cache namespace is required")
	}

	commit := sanitizeComponent(req.CommitSHA)
	snapshot := sanitizeComponent(req.SnapshotFingerprint)
	manifest := sanitizeComponent(req.ManifestVersion)
	aster := joinToggles(req.AsterToggles)

	key := fmt.Sprintf("%s/%s@commit=%s@snapshot=%s@manifest=%s@aster=%s",
		req.Lane.CacheNamespace,
		req.Lane.Name,
		commit,
		snapshot,
		manifest,
		aster,
	)
	return key, nil
}

func sanitizeComponent(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "none"
	}
	return trimmed
}

func joinToggles(values []string) string {
	if len(values) == 0 {
		return "none"
	}
	clean := make([]string, 0, len(values))
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			clean = append(clean, trimmed)
		}
	}
	if len(clean) == 0 {
		return "none"
	}
	sort.Strings(clean)
	return strings.Join(clean, "+")
}
